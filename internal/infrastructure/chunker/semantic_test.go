package chunker

import (
	"context"
	"math"
	"testing"
)

func TestSplitSentences(t *testing.T) {
	text := "Hello world. This is a test. Another sentence!"
	got := splitSentences(text)
	if len(got) != 3 {
		t.Fatalf("expected 3 sentences, got %d: %+v", len(got), got)
	}
	if got[0].text != "Hello world." {
		t.Fatalf("sentence[0] = %q, want %q", got[0].text, "Hello world.")
	}
}

func TestSplitSentencesCJK(t *testing.T) {
	text := "这是第一句。这是第二句。这是第三句。"
	got := splitSentences(text)
	if len(got) != 3 {
		t.Fatalf("expected 3 sentences, got %d: %+v", len(got), got)
	}
}

func TestCosineSimilarity(t *testing.T) {
	a := []float32{1, 0, 0}
	b := []float32{1, 0, 0}
	if sim := cosineSimilarity(a, b); math.Abs(sim-1.0) > 1e-6 {
		t.Fatalf("identical vectors: sim = %f, want 1.0", sim)
	}

	c := []float32{0, 1, 0}
	if sim := cosineSimilarity(a, c); math.Abs(sim) > 1e-6 {
		t.Fatalf("orthogonal vectors: sim = %f, want 0.0", sim)
	}
}

func TestSplitTextSemantic_NilEmbedFallback(t *testing.T) {
	text := "First paragraph about knowledge graphs.\n\nSecond paragraph about retrieval.\n\nThird paragraph about generation."
	chunks := SplitTextSemantic(context.Background(), text, SemanticConfig{MaxChunkSize: 100, MinChunkSize: 10}, nil)
	if len(chunks) == 0 {
		t.Fatal("expected non-empty chunks from fallback")
	}
	for _, c := range chunks {
		if c.Content == "" {
			t.Fatal("empty chunk content")
		}
	}
}

func TestSplitTextSemantic_WithEmbeddings(t *testing.T) {
	// Mock embedder: sentences about similar topics get similar embeddings,
	// different topics get orthogonal embeddings.
	mockEmbed := func(_ context.Context, texts []string) ([][]float32, error) {
		vecs := make([][]float32, len(texts))
		for i, text := range texts {
			if len(text) > 10 && (text[0] == 'A' || text[0] == 'B') {
				// "A*" and "B*" sentences are similar (topic 1)
				vecs[i] = []float32{0.9, 0.1, 0.0}
			} else {
				// Other sentences are different (topic 2)
				vecs[i] = []float32{0.1, 0.9, 0.0}
			}
		}
		return vecs, nil
	}

	text := "Alpha is about graphs. Beta builds on alpha. Charlie discusses retrieval. Delta covers search."
	chunks := SplitTextSemantic(context.Background(), text, SemanticConfig{
		MaxChunkSize:        200,
		MinChunkSize:        10,
		SimilarityThreshold: 0.5,
		WindowSize:          1,
	}, mockEmbed)

	if len(chunks) == 0 {
		t.Fatal("expected non-empty chunks")
	}
	// Verify no empty chunks.
	for i, c := range chunks {
		if c.Content == "" {
			t.Fatalf("chunk[%d] is empty", i)
		}
	}
}

func TestSplitTextSemantic_EmptyText(t *testing.T) {
	chunks := SplitTextSemantic(context.Background(), "", DefaultSemanticConfig(), nil)
	if chunks != nil {
		t.Fatalf("expected nil for empty text, got %d chunks", len(chunks))
	}
}

func TestBuildGroups(t *testing.T) {
	sentences := []sentenceInfo{
		{text: "A", startRune: 0, endRune: 1},
		{text: "B", startRune: 2, endRune: 3},
		{text: "C", startRune: 4, endRune: 5},
	}

	groups := buildGroups(sentences, 2)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups with window=2, got %d", len(groups))
	}
	if groups[0].text != "A B" {
		t.Fatalf("group[0] = %q, want %q", groups[0].text, "A B")
	}
	if groups[1].text != "B C" {
		t.Fatalf("group[1] = %q, want %q", groups[1].text, "B C")
	}
}
