package chunker

import (
	"strings"
	"testing"
)

func TestHeadingIndex_Basic(t *testing.T) {
	md := "# Top\n\nintro text\n\n## Section A\n\nalpha content\n\n### Sub A1\n\nleaf text\n\n## Section B\n\nbravo content\n"
	idx := NewHeadingIndex(md)
	if idx == nil {
		t.Fatal("expected non-nil index")
	}

	cases := []struct {
		needle string
		want   string
	}{
		{"intro text", "Top"},
		{"alpha content", "Top > Section A"},
		{"leaf text", "Top > Section A > Sub A1"},
		{"bravo content", "Top > Section B"},
	}
	for _, c := range cases {
		off := strings.Index(md, c.needle)
		if off < 0 {
			t.Fatalf("needle %q not found", c.needle)
		}
		got := idx.PathAt(off)
		if got != c.want {
			t.Errorf("PathAt(%q) = %q, want %q", c.needle, got, c.want)
		}
	}
}

func TestHeadingIndex_NoHeadings(t *testing.T) {
	if idx := NewHeadingIndex("plain text with no headings\nline two"); idx != nil {
		t.Error("expected nil index for heading-free input")
	}
}

func TestHeadingIndex_PathAtBeforeFirstHeading(t *testing.T) {
	md := "prelude\n\n# First\n\nbody"
	idx := NewHeadingIndex(md)
	if got := idx.PathAt(0); got != "" {
		t.Errorf("PathAt(0) = %q, want empty before first heading", got)
	}
}

func TestHeadingIndex_SkipLevels(t *testing.T) {
	// # A → ### C  (skip level 2) should still nest under A.
	md := "# A\n\n### C\n\nleaf"
	idx := NewHeadingIndex(md)
	off := strings.Index(md, "leaf")
	got := idx.PathAt(off)
	if got != "A > C" {
		t.Errorf("PathAt(leaf) = %q, want %q", got, "A > C")
	}
}
