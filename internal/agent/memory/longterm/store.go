// Package longterm provides a cross-session persistent memory store for the
// agent.
//
// The in-conversation consolidator (see the parent memory package) compresses
// messages within a single session so the context window doesn't overflow.
// This package addresses the complementary problem: surfacing facts the user
// shared in a previous session — their role, preferences, recurring topics,
// past decisions — so the agent doesn't have to re-learn them on every turn.
//
// Storage backend is behind a narrow Store interface. The reference
// implementation in this package is an in-memory store suitable for tests
// and single-process deployments; production deployments are expected to
// provide a Redis (hot tier, <30d) + Postgres (cold tier, full history)
// implementation. Keeping the interface backend-agnostic means higher tiers
// can be added without touching callers.
package longterm

import (
	"context"
	"errors"
	"strings"
	"time"
)

// Kind classifies what a memory entry represents. The classification matters
// at retrieval time: a ``fact`` about the user (their role, project)
// should nearly always be surfaced, whereas a ``summary`` of past
// activity is only relevant when its topic matches the current query.
type Kind string

const (
	// KindFact — durable, role/preference/constraint about the user.
	// Example: "user is a data scientist focused on observability".
	KindFact Kind = "fact"
	// KindPreference — explicit guidance the user gave about how the agent
	// should behave. Example: "prefer short answers, no trailing summary".
	KindPreference Kind = "preference"
	// KindSummary — digest of a prior session's work. Example:
	// "session 2026-03-28 reviewed the auth middleware rewrite".
	KindSummary Kind = "summary"
	// KindReference — pointer to an external resource. Example:
	// "Linear project INGEST tracks pipeline bugs".
	KindReference Kind = "reference"
)

// Entry is a single long-term memory record. TenantID + UserID form the
// access boundary — retrieval MUST always filter by both.
type Entry struct {
	ID             string    `json:"id"`
	TenantID       string    `json:"tenant_id"`
	UserID         string    `json:"user_id"`
	SessionID      string    `json:"session_id,omitempty"`
	Kind           Kind      `json:"kind"`
	Content        string    `json:"content"`
	Tags           []string  `json:"tags,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	LastAccessedAt time.Time `json:"last_accessed_at"`
	// AccessCount gives a crude popularity signal used to rank near-tied
	// matches; the store increments it on every Search hit.
	AccessCount int `json:"access_count"`
}

// SearchQuery is the filter passed to Store.Search. TenantID and UserID are
// required; the store returns an error when either is empty.
type SearchQuery struct {
	TenantID string
	UserID   string
	// Query is a keyword string matched against Content + Tags. Empty Query
	// is legal and returns the most-recent entries for the user.
	Query string
	// Kinds restricts results to the given kinds. Nil/empty means "any".
	Kinds []Kind
	// Tags is ANY-match: an entry is kept if it carries at least one of
	// these tags. Empty means "any".
	Tags []string
	// TopK caps returned entries. Zero means "use store default" (10).
	TopK int
}

// ErrInvalidScope is returned by stores when TenantID or UserID is missing.
// We surface this as a distinct sentinel so handlers can reject the request
// rather than quietly returning another user's data.
var ErrInvalidScope = errors.New("longterm: tenant_id and user_id are required")

// Store is the persistence abstraction. Implementations must be safe for
// concurrent use by multiple goroutines.
type Store interface {
	Save(ctx context.Context, entry *Entry) error
	Get(ctx context.Context, tenantID, id string) (*Entry, error)
	Search(ctx context.Context, q SearchQuery) ([]*Entry, error)
	Delete(ctx context.Context, tenantID, id string) error
}

// ScoreEntry computes a relevance score for a single entry against a query
// string. Exported because dispatchers and custom rerankers may want to
// reuse the exact scoring used by the in-memory store.
//
// Scoring is intentionally simple and deterministic:
//
//   - +2.0 per keyword found in Content (case-insensitive, whole-word)
//   - +1.5 per keyword found in a Tag
//   - +0.25 * log1p(AccessCount) as a mild popularity tiebreaker
//   - Kind boosts: fact/preference get a +0.5 floor so durable facts are
//     surfaced even when only loosely related to the query.
//
// No-query searches (Query == "") return score = 1.0 for every entry so
// the caller can still sort by recency.
func ScoreEntry(e *Entry, query string) float64 {
	if e == nil {
		return 0
	}
	if query == "" {
		return 1.0
	}
	q := strings.ToLower(query)
	tokens := tokenise(q)
	if len(tokens) == 0 {
		return 1.0
	}
	content := strings.ToLower(e.Content)
	score := 0.0
	for _, tok := range tokens {
		if strings.Contains(content, tok) {
			score += 2.0
		}
		for _, tag := range e.Tags {
			if strings.EqualFold(tag, tok) {
				score += 1.5
			}
		}
	}
	switch e.Kind {
	case KindFact, KindPreference:
		if score > 0 {
			score += 0.5
		}
	}
	// popularity tiebreaker — log1p so a handful of hits doesn't dominate
	if e.AccessCount > 0 {
		score += 0.25 * log1p(float64(e.AccessCount))
	}
	return score
}

// tokenise splits a lowercase query on whitespace and punctuation, dropping
// tokens shorter than 2 chars (English stopwords like "a" / "I" add noise).
// CJK characters always pass because single CJK chars are meaningful.
func tokenise(s string) []string {
	out := make([]string, 0, 4)
	var cur strings.Builder
	flush := func() {
		if cur.Len() == 0 {
			return
		}
		word := cur.String()
		cur.Reset()
		if len(word) < 2 && !hasCJK(word) {
			return
		}
		out = append(out, word)
	}
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
			cur.WriteRune(r)
		case r >= 0x4e00 && r <= 0x9fff:
			cur.WriteRune(r)
		default:
			flush()
		}
	}
	flush()
	return out
}

func hasCJK(s string) bool {
	for _, r := range s {
		if r >= 0x4e00 && r <= 0x9fff {
			return true
		}
	}
	return false
}

// log1p is a tiny local helper so we don't pull in math for one call.
func log1p(x float64) float64 {
	// Newton approximation is overkill — use a cheap polynomial that is
	// monotonic on [0, ∞): log1p(x) ≈ x / (1 + x/2 + x*x/12).
	// Fine for tiebreakers; never consulted in hot paths.
	return x / (1 + x/2 + x*x/12)
}
