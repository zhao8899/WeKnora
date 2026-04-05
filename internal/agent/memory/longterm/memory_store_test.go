package longterm

import (
	"context"
	"errors"
	"testing"
	"time"
)

func fixedClock(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

func TestMemoryStore_SaveAndGet(t *testing.T) {
	s := NewMemoryStore(fixedClock(time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)))
	ctx := context.Background()

	e := &Entry{
		TenantID: "t1", UserID: "u1",
		Kind: KindFact, Content: "user is a data scientist", Tags: []string{"role"},
	}
	if err := s.Save(ctx, e); err != nil {
		t.Fatalf("save: %v", err)
	}
	if e.ID == "" {
		t.Fatal("ID not assigned")
	}

	got, err := s.Get(ctx, "t1", e.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil || got.Content != e.Content {
		t.Fatalf("got %+v", got)
	}
	// mutating the returned entry must not affect the store
	got.Content = "mutated"
	got2, _ := s.Get(ctx, "t1", e.ID)
	if got2.Content != "user is a data scientist" {
		t.Errorf("store was mutated via returned entry")
	}
}

func TestMemoryStore_ScopeEnforced(t *testing.T) {
	s := NewMemoryStore(nil)
	ctx := context.Background()
	if err := s.Save(ctx, &Entry{UserID: "u"}); !errors.Is(err, ErrInvalidScope) {
		t.Errorf("missing tenant → want ErrInvalidScope, got %v", err)
	}
	if _, err := s.Search(ctx, SearchQuery{TenantID: "t"}); !errors.Is(err, ErrInvalidScope) {
		t.Errorf("missing user → want ErrInvalidScope, got %v", err)
	}
}

func TestMemoryStore_SearchRanksByScore(t *testing.T) {
	s := NewMemoryStore(fixedClock(time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)))
	ctx := context.Background()
	entries := []*Entry{
		{TenantID: "t", UserID: "u", Kind: KindFact,
			Content: "user prefers Rust and hates Java", Tags: []string{"language"}},
		{TenantID: "t", UserID: "u", Kind: KindSummary,
			Content: "last session discussed Python packaging", Tags: []string{"session"}},
		{TenantID: "t", UserID: "u", Kind: KindFact,
			Content: "user works at Acme Corp", Tags: []string{"org"}},
	}
	for _, e := range entries {
		if err := s.Save(ctx, e); err != nil {
			t.Fatal(err)
		}
	}

	hits, err := s.Search(ctx, SearchQuery{TenantID: "t", UserID: "u", Query: "rust"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("expected at least one hit")
	}
	if hits[0].Content != entries[0].Content {
		t.Errorf("top hit mismatch: %q", hits[0].Content)
	}
}

func TestMemoryStore_SearchRespectsTenantAndUser(t *testing.T) {
	s := NewMemoryStore(nil)
	ctx := context.Background()
	_ = s.Save(ctx, &Entry{TenantID: "t1", UserID: "u1", Kind: KindFact, Content: "secret for t1/u1"})
	_ = s.Save(ctx, &Entry{TenantID: "t2", UserID: "u1", Kind: KindFact, Content: "secret for t2/u1"})
	_ = s.Save(ctx, &Entry{TenantID: "t1", UserID: "u2", Kind: KindFact, Content: "secret for t1/u2"})

	hits, _ := s.Search(ctx, SearchQuery{TenantID: "t1", UserID: "u1", Query: "secret"})
	if len(hits) != 1 || hits[0].Content != "secret for t1/u1" {
		t.Errorf("cross-tenant leak: %+v", hits)
	}
}

func TestMemoryStore_EmptyQueryReturnsByRecency(t *testing.T) {
	base := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	clk := base
	s := NewMemoryStore(func() time.Time { return clk })
	ctx := context.Background()

	_ = s.Save(ctx, &Entry{TenantID: "t", UserID: "u", Content: "first", Kind: KindFact})
	clk = base.Add(time.Hour)
	_ = s.Save(ctx, &Entry{TenantID: "t", UserID: "u", Content: "second", Kind: KindFact})
	clk = base.Add(2 * time.Hour)
	_ = s.Save(ctx, &Entry{TenantID: "t", UserID: "u", Content: "third", Kind: KindFact})

	hits, _ := s.Search(ctx, SearchQuery{TenantID: "t", UserID: "u", TopK: 2})
	if len(hits) != 2 {
		t.Fatalf("topK=2 → got %d", len(hits))
	}
	if hits[0].Content != "third" || hits[1].Content != "second" {
		t.Errorf("recency order wrong: %s, %s", hits[0].Content, hits[1].Content)
	}
}

func TestMemoryStore_SearchTouchesAccessCount(t *testing.T) {
	s := NewMemoryStore(nil)
	ctx := context.Background()
	_ = s.Save(ctx, &Entry{TenantID: "t", UserID: "u", Content: "alpha", Kind: KindFact})
	_, _ = s.Search(ctx, SearchQuery{TenantID: "t", UserID: "u", Query: "alpha"})
	_, _ = s.Search(ctx, SearchQuery{TenantID: "t", UserID: "u", Query: "alpha"})
	e, _ := s.Search(ctx, SearchQuery{TenantID: "t", UserID: "u", Query: "alpha"})
	if e[0].AccessCount != 3 {
		t.Errorf("AccessCount expected 3 got %d", e[0].AccessCount)
	}
}

func TestMemoryStore_Delete(t *testing.T) {
	s := NewMemoryStore(nil)
	ctx := context.Background()
	e := &Entry{TenantID: "t", UserID: "u", Content: "x", Kind: KindFact}
	_ = s.Save(ctx, e)
	_ = s.Delete(ctx, "t", e.ID)
	got, _ := s.Get(ctx, "t", e.ID)
	if got != nil {
		t.Errorf("delete didn't remove: %+v", got)
	}
	// idempotent
	if err := s.Delete(ctx, "t", "nope"); err != nil {
		t.Errorf("delete missing should not error: %v", err)
	}
}

func TestTokenise(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"hello world", []string{"hello", "world"}},
		{"I/O buffer, size=12", []string{"buffer", "size", "12"}}, // single-char I, O filtered
		{"a the on", []string{"the", "on"}}, // single-char "a" filtered
		{"中文 测试", []string{"中文", "测试"}},
		{"", nil},
	}
	for _, c := range cases {
		// tokenise expects lowercase
		got := tokenise(lower(c.in))
		if !sliceEq(got, c.want) {
			t.Errorf("tokenise(%q) = %v want %v", c.in, got, c.want)
		}
	}
}

func lower(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r >= 'A' && r <= 'Z' {
			r = r + ('a' - 'A')
		}
		out = append(out, r)
	}
	return string(out)
}

func sliceEq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
