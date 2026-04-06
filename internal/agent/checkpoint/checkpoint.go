// Package checkpoint provides durable execution support for the ReACT agent.
//
// After each round completes, the agent engine can persist a Checkpoint
// containing the full execution state (AgentState + LLM messages). If the
// process crashes or the HTTP connection drops, a new engine can resume
// from the last saved checkpoint rather than re-executing from scratch.
//
// The checkpoint store is behind a narrow Store interface. The Redis
// implementation is the production default; an in-memory implementation
// is provided for tests.
package checkpoint

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/redis/go-redis/v9"
)

// Checkpoint captures the full agent execution state at the end of a round.
type Checkpoint struct {
	// Execution identity
	SessionID      string `json:"session_id"`
	MessageID      string `json:"message_id"`
	ExecutionID    string `json:"execution_id"`
	Query          string `json:"query"`

	// Agent state
	State *types.AgentState `json:"state"`

	// LLM context — the full message array so the loop can resume mid-conversation.
	Messages []chat.Message `json:"messages"`

	// Metadata
	Round     int       `json:"round"`
	SavedAt   time.Time `json:"saved_at"`
	Completed bool      `json:"completed"`
}

// Store is the persistence abstraction for agent checkpoints.
type Store interface {
	// Save persists a checkpoint. Overwrites any existing checkpoint for the
	// same (sessionID, executionID) pair.
	Save(ctx context.Context, cp *Checkpoint) error

	// Load retrieves the latest checkpoint for the given session. Returns
	// (nil, nil) when no checkpoint exists.
	Load(ctx context.Context, sessionID string) (*Checkpoint, error)

	// Delete removes a checkpoint (called after successful completion).
	Delete(ctx context.Context, sessionID string) error
}

// ---------------------------------------------------------------------------
// Redis implementation
// ---------------------------------------------------------------------------

// RedisStore persists checkpoints in Redis with a configurable TTL.
type RedisStore struct {
	client *redis.Client
	ttl    time.Duration
	prefix string
}

// NewRedisStore creates a Redis-backed checkpoint store.
// Default TTL is 1 hour (agent tasks should not take longer).
func NewRedisStore(client *redis.Client, ttl time.Duration) *RedisStore {
	if ttl == 0 {
		ttl = 1 * time.Hour
	}
	return &RedisStore{client: client, ttl: ttl, prefix: "agent:cp:"}
}

func (s *RedisStore) key(sessionID string) string {
	return fmt.Sprintf("%s%s", s.prefix, sessionID)
}

func (s *RedisStore) Save(ctx context.Context, cp *Checkpoint) error {
	data, err := json.Marshal(cp)
	if err != nil {
		return fmt.Errorf("checkpoint: marshal: %w", err)
	}
	return s.client.Set(ctx, s.key(cp.SessionID), data, s.ttl).Err()
}

func (s *RedisStore) Load(ctx context.Context, sessionID string) (*Checkpoint, error) {
	data, err := s.client.Get(ctx, s.key(sessionID)).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var cp Checkpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return nil, fmt.Errorf("checkpoint: unmarshal: %w", err)
	}
	return &cp, nil
}

func (s *RedisStore) Delete(ctx context.Context, sessionID string) error {
	return s.client.Del(ctx, s.key(sessionID)).Err()
}

// ---------------------------------------------------------------------------
// In-memory implementation (for tests)
// ---------------------------------------------------------------------------

// MemoryStore is a test-only in-memory checkpoint store.
type MemoryStore struct {
	data map[string]*Checkpoint
}

// NewMemoryStore returns an empty in-memory store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{data: make(map[string]*Checkpoint)}
}

func (s *MemoryStore) Save(_ context.Context, cp *Checkpoint) error {
	s.data[cp.SessionID] = cp
	return nil
}

func (s *MemoryStore) Load(_ context.Context, sessionID string) (*Checkpoint, error) {
	cp, ok := s.data[sessionID]
	if !ok {
		return nil, nil
	}
	return cp, nil
}

func (s *MemoryStore) Delete(_ context.Context, sessionID string) error {
	delete(s.data, sessionID)
	return nil
}
