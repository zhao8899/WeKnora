package longterm

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisStore is a persistent Store implementation backed by Redis. Entries are
// stored as JSON hashes keyed by "ltm:{tenantID}:{id}" with a secondary set
// "ltm:idx:{tenantID}:{userID}" that tracks all entry IDs for a user,
// enabling efficient per-user scans. A configurable TTL auto-expires entries
// that haven't been accessed; production default is 30 days.
type RedisStore struct {
	client *redis.Client
	ttl    time.Duration
	prefix string
}

// NewRedisStore creates a Redis-backed longterm store. ttl controls how long
// entries survive without being accessed (0 = 30 days default). prefix is
// prepended to all Redis keys (default "ltm:").
func NewRedisStore(client *redis.Client, ttl time.Duration, prefix string) *RedisStore {
	if ttl == 0 {
		ttl = 30 * 24 * time.Hour
	}
	if prefix == "" {
		prefix = "ltm:"
	}
	return &RedisStore{client: client, ttl: ttl, prefix: prefix}
}

func (s *RedisStore) entryKey(tenantID, id string) string {
	return fmt.Sprintf("%s%s:%s", s.prefix, tenantID, id)
}

func (s *RedisStore) indexKey(tenantID, userID string) string {
	return fmt.Sprintf("%sidx:%s:%s", s.prefix, tenantID, userID)
}

// Save upserts an entry in Redis. If entry.ID is empty, a timestamp-based ID
// is assigned. The entry is stored as a JSON blob and its ID is added to the
// per-user index set.
func (s *RedisStore) Save(ctx context.Context, e *Entry) error {
	if e == nil {
		return fmt.Errorf("longterm: nil entry")
	}
	if e.TenantID == "" || e.UserID == "" {
		return ErrInvalidScope
	}

	now := time.Now()
	if e.ID == "" {
		e.ID = fmt.Sprintf("r-%d", now.UnixNano())
	}

	// Preserve CreatedAt on overwrites.
	rkey := s.entryKey(e.TenantID, e.ID)
	existing, err := s.client.Get(ctx, rkey).Bytes()
	if err == nil && len(existing) > 0 {
		var prev Entry
		if json.Unmarshal(existing, &prev) == nil {
			if e.CreatedAt.IsZero() {
				e.CreatedAt = prev.CreatedAt
			}
			if e.AccessCount == 0 {
				e.AccessCount = prev.AccessCount
			}
		}
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = now
	}
	e.LastAccessedAt = now

	data, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("longterm: marshal entry: %w", err)
	}

	pipe := s.client.Pipeline()
	pipe.Set(ctx, rkey, data, s.ttl)
	pipe.SAdd(ctx, s.indexKey(e.TenantID, e.UserID), e.ID)
	_, err = pipe.Exec(ctx)
	return err
}

// Get retrieves a single entry. Returns (nil, nil) when the key doesn't exist.
func (s *RedisStore) Get(ctx context.Context, tenantID, id string) (*Entry, error) {
	if tenantID == "" {
		return nil, ErrInvalidScope
	}
	data, err := s.client.Get(ctx, s.entryKey(tenantID, id)).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var e Entry
	if err := json.Unmarshal(data, &e); err != nil {
		return nil, fmt.Errorf("longterm: unmarshal entry: %w", err)
	}
	return &e, nil
}

// Delete removes an entry. Idempotent — missing keys are not an error.
func (s *RedisStore) Delete(ctx context.Context, tenantID, id string) error {
	if tenantID == "" {
		return ErrInvalidScope
	}
	// We need UserID to clean the index set. Fetch the entry first.
	data, err := s.client.Get(ctx, s.entryKey(tenantID, id)).Bytes()
	if err == redis.Nil {
		return nil
	}
	if err != nil {
		return err
	}
	var e Entry
	if json.Unmarshal(data, &e) == nil && e.UserID != "" {
		s.client.SRem(ctx, s.indexKey(tenantID, e.UserID), id)
	}
	return s.client.Del(ctx, s.entryKey(tenantID, id)).Err()
}

// Search returns the top-K scored entries for the given tenant + user.
// Scoring reuses the same ScoreEntry function as MemoryStore for consistency.
func (s *RedisStore) Search(ctx context.Context, q SearchQuery) ([]*Entry, error) {
	if q.TenantID == "" || q.UserID == "" {
		return nil, ErrInvalidScope
	}
	if q.TopK <= 0 {
		q.TopK = 10
	}

	// Fetch all entry IDs for this user from the index set.
	ids, err := s.client.SMembers(ctx, s.indexKey(q.TenantID, q.UserID)).Result()
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}

	// Batch fetch all entries.
	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = s.entryKey(q.TenantID, id)
	}
	results, err := s.client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, err
	}

	kindSet := make(map[Kind]bool, len(q.Kinds))
	for _, k := range q.Kinds {
		kindSet[k] = true
	}
	tagSet := make(map[string]bool, len(q.Tags))
	for _, t := range q.Tags {
		tagSet[strings.ToLower(t)] = true
	}

	type scored struct {
		entry *Entry
		score float64
	}
	candidates := make([]scored, 0, len(results))
	for _, raw := range results {
		str, ok := raw.(string)
		if !ok || str == "" {
			continue
		}
		var e Entry
		if json.Unmarshal([]byte(str), &e) != nil {
			continue
		}
		if e.UserID != q.UserID {
			continue
		}
		if len(kindSet) > 0 && !kindSet[e.Kind] {
			continue
		}
		if len(tagSet) > 0 {
			hit := false
			for _, t := range e.Tags {
				if tagSet[strings.ToLower(t)] {
					hit = true
					break
				}
			}
			if !hit {
				continue
			}
		}
		sc := ScoreEntry(&e, q.Query)
		if q.Query != "" && sc == 0 {
			continue
		}
		candidates = append(candidates, scored{entry: &e, score: sc})
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		if !candidates[i].entry.LastAccessedAt.Equal(candidates[j].entry.LastAccessedAt) {
			return candidates[i].entry.LastAccessedAt.After(candidates[j].entry.LastAccessedAt)
		}
		return candidates[i].entry.CreatedAt.After(candidates[j].entry.CreatedAt)
	})

	if len(candidates) > q.TopK {
		candidates = candidates[:q.TopK]
	}

	// Touch returned entries: bump AccessCount and refresh TTL.
	now := time.Now()
	out := make([]*Entry, 0, len(candidates))
	for _, c := range candidates {
		c.entry.AccessCount++
		c.entry.LastAccessedAt = now
		data, err := json.Marshal(c.entry)
		if err == nil {
			s.client.Set(ctx, s.entryKey(c.entry.TenantID, c.entry.ID), data, s.ttl)
		}
		out = append(out, c.entry)
	}
	return out, nil
}
