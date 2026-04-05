// Package chunker - contextual.go implements Anthropic-style "Contextual
// Retrieval" chunking. For each chunk, we prepend a short breadcrumb of
// enclosing markdown headings before sending the text to the embedding model.
//
// This closes the context gap that plagues naive chunkers: a sentence like
// "Revenue grew 12% year-over-year" is ambiguous on its own, but becomes
// unambiguous when embedded alongside "Q3 2024 Report > Financials > Revenue".
//
// The heuristic is deterministic and LLM-free — it scans ATX-style markdown
// headings (# ... ######) once per document, then maps each chunk's start
// offset to the active heading stack at that point.
package chunker

import (
	"regexp"
	"strings"
)

// atxHeading matches ATX-style markdown headings at the start of a line:
//
//	#   Title
//	### Subsection
//
// The leading whitespace is captured but not required. Trailing "#" markers
// (permitted by CommonMark) are stripped when building the breadcrumb.
var atxHeading = regexp.MustCompile(`(?m)^[ \t]{0,3}(#{1,6})[ \t]+([^\n]+?)[ \t]*#*[ \t]*$`)

// HeadingIndex maps byte offsets in a markdown document to the active
// heading stack at that position.
type HeadingIndex struct {
	entries []headingEntry
}

type headingEntry struct {
	offset int
	level  int
	text   string
}

// NewHeadingIndex scans markdown for ATX headings and returns an index
// usable to resolve the heading breadcrumb at any byte offset.
// Returns nil when the input has no headings, so callers can quickly skip.
func NewHeadingIndex(markdown string) *HeadingIndex {
	locs := atxHeading.FindAllStringSubmatchIndex(markdown, -1)
	if len(locs) == 0 {
		return nil
	}
	entries := make([]headingEntry, 0, len(locs))
	for _, loc := range locs {
		// loc layout: [matchStart, matchEnd, g1Start, g1End, g2Start, g2End]
		if len(loc) < 6 {
			continue
		}
		level := loc[3] - loc[2]
		text := strings.TrimSpace(markdown[loc[4]:loc[5]])
		if text == "" {
			continue
		}
		entries = append(entries, headingEntry{
			offset: loc[0],
			level:  level,
			text:   text,
		})
	}
	if len(entries) == 0 {
		return nil
	}
	return &HeadingIndex{entries: entries}
}

// PathAt returns a " > "-joined breadcrumb of enclosing headings for the
// given byte offset. Lower-level headings (# is level 1) nest higher-level
// ones. Returns "" when no headings precede the offset.
//
// Example: for an offset inside "### Revenue" under "## Financials" under
// "# Q3 Report", returns "Q3 Report > Financials > Revenue".
func (h *HeadingIndex) PathAt(offset int) string {
	if h == nil || len(h.entries) == 0 {
		return ""
	}
	// Walk entries in order, maintaining the active heading stack.
	// When a heading at level L appears, it replaces any active heading
	// at level >= L (standard markdown outline semantics).
	var stack []headingEntry
	for _, e := range h.entries {
		if e.offset > offset {
			break
		}
		// Pop entries of equal or deeper level.
		for len(stack) > 0 && stack[len(stack)-1].level >= e.level {
			stack = stack[:len(stack)-1]
		}
		stack = append(stack, e)
	}
	if len(stack) == 0 {
		return ""
	}
	parts := make([]string, len(stack))
	for i, e := range stack {
		parts[i] = e.text
	}
	return strings.Join(parts, " > ")
}
