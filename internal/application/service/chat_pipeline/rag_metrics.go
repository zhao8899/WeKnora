package chatpipeline

import (
	"context"
	"strings"

	"go.opentelemetry.io/otel/attribute"

	"github.com/Tencent/WeKnora/internal/tracing"
	"github.com/Tencent/WeKnora/internal/types"
)

// RecordRAGMetrics computes lightweight online quality metrics for a completed
// RAG pipeline run and records them as OTLP span attributes. This enables
// quality monitoring in Jaeger / Langfuse / Grafana without requiring an LLM
// judge.
//
// Metrics recorded:
//   - rag.context_precision: fraction of query keywords found in retrieved contexts
//   - rag.retrieval_count:   number of chunks that reached the LLM context
//   - rag.retrieval_verdict: CRAG grader verdict (correct/ambiguous/incorrect)
//   - rag.query_length:      character length of the user query
//   - rag.context_length:    character length of the rendered contexts
//
// Call this after the pipeline completes (after KnowledgeQAByEvent returns).
func RecordRAGMetrics(ctx context.Context, chatManage *types.ChatManage) {
	if chatManage == nil {
		return
	}

	ctx, span := tracing.ContextWithSpan(ctx, "rag.quality_metrics")
	defer span.End()

	query := chatManage.Query
	if chatManage.RewriteQuery != "" {
		query = chatManage.RewriteQuery
	}

	// Context precision: fraction of query keywords found in contexts.
	precision := contextPrecision(query, chatManage.RenderedContexts)

	span.SetAttributes(
		attribute.String("rag.session_id", chatManage.SessionID),
		attribute.Float64("rag.context_precision", precision),
		attribute.Int("rag.retrieval_count", len(chatManage.MergeResult)),
		attribute.String("rag.retrieval_verdict", chatManage.RetrievalVerdict),
		attribute.Int("rag.query_length", len(query)),
		attribute.Int("rag.context_length", len(chatManage.RenderedContexts)),
		attribute.String("rag.intent", string(chatManage.Intent)),
	)
}

// contextPrecision computes the fraction of query keywords that appear in the
// retrieved context. This is the same metric as scripts/rag_metrics.py's
// context_precision, ported to Go for online use.
func contextPrecision(query, contexts string) float64 {
	if query == "" || contexts == "" {
		return 0
	}
	tokens := tokeniseQuery(query)
	if len(tokens) == 0 {
		return 0
	}
	lower := strings.ToLower(contexts)
	hits := 0
	for _, tok := range tokens {
		if strings.Contains(lower, tok) {
			hits++
		}
	}
	return float64(hits) / float64(len(tokens))
}

// tokeniseQuery splits a query into lowercase tokens for keyword matching.
// Filters out short tokens (<2 chars) except CJK characters.
func tokeniseQuery(s string) []string {
	out := make([]string, 0, 8)
	var cur strings.Builder
	flush := func() {
		if cur.Len() == 0 {
			return
		}
		word := strings.ToLower(cur.String())
		cur.Reset()
		if len(word) < 2 && !hasCJKChar(word) {
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

func hasCJKChar(s string) bool {
	for _, r := range s {
		if r >= 0x4e00 && r <= 0x9fff {
			return true
		}
	}
	return false
}
