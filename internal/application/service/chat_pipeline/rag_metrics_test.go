package chatpipeline

import (
	"testing"
)

func TestContextPrecision(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		contexts string
		want     float64
	}{
		{
			name:     "all keywords found",
			query:    "knowledge graph",
			contexts: "A knowledge graph is a structured representation...",
			want:     1.0,
		},
		{
			name:     "half keywords found",
			query:    "knowledge quantum",
			contexts: "A knowledge base stores documents...",
			want:     0.5,
		},
		{
			name:     "no keywords found",
			query:    "quantum physics",
			contexts: "A knowledge base stores documents...",
			want:     0.0,
		},
		{
			name:     "empty query",
			query:    "",
			contexts: "some contexts",
			want:     0.0,
		},
		{
			name:     "empty contexts",
			query:    "knowledge",
			contexts: "",
			want:     0.0,
		},
		{
			name:     "CJK keywords",
			query:    "知识图谱 检索",
			contexts: "知识图谱是一种结构化的检索方式",
			want:     1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := contextPrecision(tt.query, tt.contexts)
			if got != tt.want {
				t.Fatalf("contextPrecision(%q, %q) = %f, want %f", tt.query, tt.contexts, got, tt.want)
			}
		})
	}
}
