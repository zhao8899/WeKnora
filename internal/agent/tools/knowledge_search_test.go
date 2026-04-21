package tools

import (
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

func TestKnowledgeSearchCompositeScore_UsesKnowledgeSourceWeight(t *testing.T) {
	tool := &KnowledgeSearchTool{}

	lowWeight := &searchResultWithMeta{
		SearchResult: &types.SearchResult{Metadata: map[string]string{"source_weight": "0.50"}},
	}
	highWeight := &searchResultWithMeta{
		SearchResult: &types.SearchResult{Metadata: map[string]string{"source_weight": "1.80"}},
	}

	lowScore := tool.compositeScore(lowWeight, 0.8, 0.8)
	highScore := tool.compositeScore(highWeight, 0.8, 0.8)

	if highScore <= lowScore {
		t.Fatalf("expected higher source_weight to raise composite score, got high=%f low=%f", highScore, lowScore)
	}
}
