package chunker

import (
	"context"
	"math"
	"strings"
	"unicode/utf8"
)

// SemanticConfig configures the semantic chunker.
type SemanticConfig struct {
	// MaxChunkSize is the upper bound on chunk size in runes. Semantic chunks
	// that exceed this are force-split using the recursive splitter as fallback.
	MaxChunkSize int
	// MinChunkSize is the minimum size below which adjacent chunks are merged.
	MinChunkSize int
	// SimilarityThreshold is the cosine-similarity threshold below which a
	// semantic boundary is placed. Lower values produce larger chunks.
	// Typical range: 0.3–0.7. Default: 0.5.
	SimilarityThreshold float64
	// WindowSize is the number of sentences combined into a "group" before
	// computing its embedding. Grouping smooths noisy single-sentence
	// embeddings. Default: 1 (no grouping).
	WindowSize int
}

// DefaultSemanticConfig returns production-sane defaults.
func DefaultSemanticConfig() SemanticConfig {
	return SemanticConfig{
		MaxChunkSize:        512,
		MinChunkSize:        50,
		SimilarityThreshold: 0.5,
		WindowSize:          1,
	}
}

// EmbedFunc is the callback the semantic chunker uses to obtain embeddings.
// It maps to embedding.Embedder.BatchEmbed but avoids importing the models
// package so the chunker stays infrastructure-only.
type EmbedFunc func(ctx context.Context, texts []string) ([][]float32, error)

// SplitTextSemantic splits text into chunks at semantic boundaries detected
// by embedding cosine similarity between consecutive sentence groups.
//
// Algorithm:
//  1. Split text into sentences.
//  2. Group sentences into windows of size WindowSize.
//  3. Compute embeddings for each group via embedFn.
//  4. Calculate cosine similarity between consecutive groups.
//  5. Place boundaries where similarity drops below SimilarityThreshold.
//  6. Merge adjacent chunks that are too small (< MinChunkSize).
//  7. Force-split chunks that exceed MaxChunkSize.
//
// Falls back to the recursive splitter if embedFn is nil or returns an error.
func SplitTextSemantic(ctx context.Context, text string, cfg SemanticConfig, embedFn EmbedFunc) []Chunk {
	if text == "" {
		return nil
	}
	if cfg.MaxChunkSize <= 0 {
		cfg.MaxChunkSize = DefaultSemanticConfig().MaxChunkSize
	}
	if cfg.MinChunkSize <= 0 {
		cfg.MinChunkSize = DefaultSemanticConfig().MinChunkSize
	}
	if cfg.SimilarityThreshold <= 0 {
		cfg.SimilarityThreshold = DefaultSemanticConfig().SimilarityThreshold
	}
	if cfg.WindowSize <= 0 {
		cfg.WindowSize = 1
	}

	// Fallback to recursive splitter when no embedding function is available.
	if embedFn == nil {
		return SplitText(text, SplitterConfig{
			ChunkSize:    cfg.MaxChunkSize,
			ChunkOverlap: cfg.MinChunkSize,
			Separators:   []string{"\n\n", "\n", "。"},
		})
	}

	sentences := splitSentences(text)
	if len(sentences) <= 1 {
		return []Chunk{{Content: text, Seq: 0, Start: 0, End: utf8.RuneCountInString(text)}}
	}

	// Build sentence groups for embedding.
	groups := buildGroups(sentences, cfg.WindowSize)
	if len(groups) == 0 {
		return SplitText(text, SplitterConfig{ChunkSize: cfg.MaxChunkSize, ChunkOverlap: cfg.MinChunkSize})
	}

	// Compute embeddings.
	groupTexts := make([]string, len(groups))
	for i, g := range groups {
		groupTexts[i] = g.text
	}
	embeddings, err := embedFn(ctx, groupTexts)
	if err != nil || len(embeddings) != len(groups) {
		// Fallback on embedding error.
		return SplitText(text, SplitterConfig{ChunkSize: cfg.MaxChunkSize, ChunkOverlap: cfg.MinChunkSize})
	}

	// Find semantic boundaries: where similarity between consecutive groups
	// drops below the threshold.
	boundaries := []int{0} // always start at sentence 0
	for i := 0; i < len(embeddings)-1; i++ {
		sim := cosineSimilarity(embeddings[i], embeddings[i+1])
		if sim < cfg.SimilarityThreshold {
			// Boundary after groups[i] → start of groups[i+1]
			boundaries = append(boundaries, groups[i+1].startSentence)
		}
	}

	// Build raw chunks from boundaries.
	rawChunks := buildChunksFromBoundaries(text, sentences, boundaries, cfg)

	// Merge small chunks with their neighbours.
	merged := mergeSmallChunks(rawChunks, cfg.MinChunkSize)

	// Force-split oversized chunks.
	var result []Chunk
	seq := 0
	for _, c := range merged {
		if utf8.RuneCountInString(c.Content) > cfg.MaxChunkSize {
			subs := SplitText(c.Content, SplitterConfig{
				ChunkSize:    cfg.MaxChunkSize,
				ChunkOverlap: cfg.MinChunkSize / 2,
				Separators:   []string{"\n\n", "\n", "。", " "},
			})
			for _, sub := range subs {
				sub.Seq = seq
				sub.Start += c.Start
				sub.End = sub.Start + utf8.RuneCountInString(sub.Content)
				result = append(result, sub)
				seq++
			}
		} else {
			c.Seq = seq
			result = append(result, c)
			seq++
		}
	}
	return result
}

// sentenceInfo tracks a sentence's position in the original text.
type sentenceInfo struct {
	text       string
	startRune  int
	endRune    int
}

// groupInfo tracks a group of sentences for embedding.
type groupInfo struct {
	text          string
	startSentence int
}

// splitSentences splits text into sentences. Uses a simple heuristic:
// split on sentence-ending punctuation followed by whitespace or newline.
func splitSentences(text string) []sentenceInfo {
	var sentences []sentenceInfo
	var cur strings.Builder
	startRune := 0
	runeIdx := 0

	runes := []rune(text)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		cur.WriteRune(r)
		runeIdx = i + 1

		isSentenceEnd := false
		switch r {
		case '.', '!', '?', '。', '！', '？', '；':
			// Check if next char is whitespace or newline or end of text.
			if i+1 >= len(runes) {
				isSentenceEnd = true
			} else {
				next := runes[i+1]
				if next == ' ' || next == '\n' || next == '\r' || next == '\t' {
					isSentenceEnd = true
				}
			}
		case '\n':
			// Double newline is always a sentence boundary.
			if i+1 < len(runes) && runes[i+1] == '\n' {
				isSentenceEnd = true
			}
		}

		if isSentenceEnd {
			s := strings.TrimSpace(cur.String())
			if s != "" {
				sentences = append(sentences, sentenceInfo{
					text:      s,
					startRune: startRune,
					endRune:   runeIdx,
				})
			}
			cur.Reset()
			startRune = runeIdx
		}
	}

	// Flush remaining text.
	if s := strings.TrimSpace(cur.String()); s != "" {
		sentences = append(sentences, sentenceInfo{
			text:      s,
			startRune: startRune,
			endRune:   runeIdx,
		})
	}
	return sentences
}

// buildGroups creates sentence groups of the given window size.
func buildGroups(sentences []sentenceInfo, windowSize int) []groupInfo {
	if windowSize < 1 {
		windowSize = 1
	}
	var groups []groupInfo
	for i := 0; i <= len(sentences)-windowSize; i++ {
		var parts []string
		for j := i; j < i+windowSize && j < len(sentences); j++ {
			parts = append(parts, sentences[j].text)
		}
		groups = append(groups, groupInfo{
			text:          strings.Join(parts, " "),
			startSentence: i,
		})
	}
	return groups
}

// buildChunksFromBoundaries assembles chunks from sentence boundaries.
func buildChunksFromBoundaries(text string, sentences []sentenceInfo, boundaries []int, cfg SemanticConfig) []Chunk {
	var chunks []Chunk
	runes := []rune(text)

	for bi := 0; bi < len(boundaries); bi++ {
		startSent := boundaries[bi]
		var endSent int
		if bi+1 < len(boundaries) {
			endSent = boundaries[bi+1]
		} else {
			endSent = len(sentences)
		}
		if startSent >= endSent || startSent >= len(sentences) {
			continue
		}

		startRune := sentences[startSent].startRune
		endRune := sentences[endSent-1].endRune
		if endRune > len(runes) {
			endRune = len(runes)
		}

		content := string(runes[startRune:endRune])
		content = strings.TrimSpace(content)
		if content == "" {
			continue
		}
		chunks = append(chunks, Chunk{
			Content: content,
			Start:   startRune,
			End:     endRune,
		})
	}
	return chunks
}

// mergeSmallChunks merges chunks that are below minSize with their nearest neighbour.
func mergeSmallChunks(chunks []Chunk, minSize int) []Chunk {
	if len(chunks) <= 1 {
		return chunks
	}
	var merged []Chunk
	for i := 0; i < len(chunks); i++ {
		if utf8.RuneCountInString(chunks[i].Content) < minSize && i+1 < len(chunks) {
			// Merge with next chunk.
			next := chunks[i+1]
			next.Content = chunks[i].Content + "\n" + next.Content
			next.Start = chunks[i].Start
			chunks[i+1] = next
			continue
		}
		merged = append(merged, chunks[i])
	}
	return merged
}

// cosineSimilarity computes the cosine similarity between two float32 vectors.
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 0
	}
	return dot / denom
}
