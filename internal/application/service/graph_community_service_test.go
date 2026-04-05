package service

import (
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

func TestRenderCommunitySummary(t *testing.T) {
	g := &types.CommunityGroup{
		ID:   42,
		Size: 3,
		Nodes: []*types.GraphNode{
			{Name: "Alice"},
			{Name: "Bob"},
			{Name: "Carol"},
		},
		Relation: []*types.GraphRelation{
			{Node1: "Alice", Node2: "Bob", Type: "knows"},
			{Node1: "Bob", Node2: "Carol", Type: "works_with"},
		},
	}
	cs := renderCommunitySummary(g)
	if cs.CommunityID != 42 || cs.Size != 3 {
		t.Fatalf("unexpected header: id=%d size=%d", cs.CommunityID, cs.Size)
	}
	if len(cs.Entities) != 3 {
		t.Fatalf("want 3 entities, got %d", len(cs.Entities))
	}
	if len(cs.Edges) != 2 {
		t.Fatalf("want 2 edges, got %d", len(cs.Edges))
	}
	if !strings.Contains(cs.Text, "Alice, Bob, Carol") {
		t.Errorf("expected entity line, got:\n%s", cs.Text)
	}
	if !strings.Contains(cs.Text, "Alice -[knows]-> Bob") {
		t.Errorf("expected edge render, got:\n%s", cs.Text)
	}
}

func TestRenderCommunitySummary_SkipsEmptyNames(t *testing.T) {
	g := &types.CommunityGroup{
		ID:   1,
		Size: 2,
		Nodes: []*types.GraphNode{
			{Name: "X"},
			{Name: ""}, // must be skipped
			nil,        // must be skipped
		},
	}
	cs := renderCommunitySummary(g)
	if len(cs.Entities) != 1 || cs.Entities[0] != "X" {
		t.Fatalf("empty/nil names should be skipped, got %v", cs.Entities)
	}
}

func TestFormatForPrompt_Empty(t *testing.T) {
	if got := FormatForPrompt(nil); got != "" {
		t.Errorf("nil → want empty, got %q", got)
	}
	if got := FormatForPrompt([]*CommunitySummary{}); got != "" {
		t.Errorf("empty → want empty, got %q", got)
	}
}

func TestFormatForPrompt_ConcatenatesWithHeader(t *testing.T) {
	sums := []*CommunitySummary{
		{Text: "Community #1\n"},
		{Text: "Community #2\n"},
	}
	out := FormatForPrompt(sums)
	if !strings.HasPrefix(out, "### Knowledge Graph Communities\n") {
		t.Errorf("missing header, got:\n%s", out)
	}
	if !strings.Contains(out, "Community #1") || !strings.Contains(out, "Community #2") {
		t.Errorf("missing community bodies, got:\n%s", out)
	}
}
