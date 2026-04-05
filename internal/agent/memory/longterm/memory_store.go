package longterm

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// MemoryStore is an in-process Store implementation backed by a map. It is
// suitable for tests, single-node deployments, and as a cache tier in front
// of a persistent backend. Not durable across restarts.
type MemoryStore struct {
	mu      sync.RWMutex
	entries map[string]*Entry // key: tenantID + "/" + id
	now     func() time.Time
	nextID  int64
}

// NewMemoryStore returns an empty in-process store. Pass a fake clock for
// deterministic tests; nil uses time.Now.
func NewMemoryStore(clock func() time.Time) *MemoryStore {
	if clock == nil {
		clock = time.Now
	}
	return &MemoryStore{
		entries: make(map[string]*Entry),
		now:     clock,
	}
}

func key(tenantID, id string) string { return tenantID + "/" + id }

// Save upserts an entry. If entry.ID is empty, the store assigns one.
// CreatedAt is set on first insert; LastAccessedAt is touched on every save.
func (s *MemoryStore) Save(_ context.Context, e *Entry) error {
	if e == nil {
		return fmt.Errorf("longterm: nil entry")
	}
	if e.TenantID == "" || e.UserID == "" {
		return ErrInvalidScope
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if e.ID == "" {
		s.nextID++
		e.ID = fmt.Sprintf("mem-%d", s.nextID)
	}
	now := s.now()
	if existing, ok := s.entries[key(e.TenantID, e.ID)]; ok {
		// preserve CreatedAt and AccessCount across overwrites
		e.CreatedAt = existing.CreatedAt
		if e.AccessCount == 0 {
			e.AccessCount = existing.AccessCount
		}
	} else {
		if e.CreatedAt.IsZero() {
			e.CreatedAt = now
		}
	}
	e.LastAccessedAt = now
	// Take a shallow copy so callers mutating the struct after Save don't
	// corrupt our state.
	cp := *e
	if e.Tags != nil {
		cp.Tags = append([]string(nil), e.Tags...)
	}
	s.entries[key(e.TenantID, e.ID)] = &cp
	return nil
}

// Get fetches a single entry by (tenant, id). Returns nil, nil when the
// entry doesn't exist (not an error — callers routinely probe for optional
// keys).
func (s *MemoryStore) Get(_ context.Context, tenantID, id string) (*Entry, error) {
	if tenantID == "" {
		return nil, ErrInvalidScope
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.entries[key(tenantID, id)]
	if !ok {
		return nil, nil
	}
	cp := *e
	cp.Tags = append([]string(nil), e.Tags...)
	return &cp, nil
}

// Delete removes an entry. Missing keys are not an error (idempotent).
func (s *MemoryStore) Delete(_ context.Context, tenantID, id string) error {
	if tenantID == "" {
		return ErrInvalidScope
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, key(tenantID, id))
	return nil
}

// Search returns the TopK highest-scoring entries for (TenantID, UserID).
// Ranking: ScoreEntry desc, LastAccessedAt desc, CreatedAt desc.
// Side effect: increments AccessCount on returned entries so popularity
// tiebreaking works over time.
func (s *MemoryStore) Search(_ context.Context, q SearchQuery) ([]*Entry, error) {
	if q.TenantID == "" || q.UserID == "" {
		return nil, ErrInvalidScope
	}
	if q.TopK <= 0 {
		q.TopK = 10
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
	s.mu.RLock()
	candidates := make([]scored, 0, 32)
	for _, e := range s.entries {
		if e.TenantID != q.TenantID || e.UserID != q.UserID {
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
		sc := ScoreEntry(e, q.Query)
		if q.Query != "" && sc == 0 {
			continue
		}
		candidates = append(candidates, scored{entry: e, score: sc})
	}
	s.mu.RUnlock()

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

	now := s.now()
	out := make([]*Entry, 0, len(candidates))
	// touch returned entries — upgrade to write lock briefly
	s.mu.Lock()
	for _, c := range candidates {
		if live, ok := s.entries[key(c.entry.TenantID, c.entry.ID)]; ok {
			live.AccessCount++
			live.LastAccessedAt = now
			cp := *live
			cp.Tags = append([]string(nil), live.Tags...)
			out = append(out, &cp)
		}
	}
	s.mu.Unlock()
	return out, nil
}
