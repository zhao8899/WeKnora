package chatpipeline

import (
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

func TestCompositeScore_UsesKnowledgeSourceWeight(t *testing.T) {
	lowWeight := &types.SearchResult{
		Metadata: map[string]string{"source_weight": "0.50"},
	}
	highWeight := &types.SearchResult{
		Metadata: map[string]string{"source_weight": "1.80"},
	}

	lowScore := compositeScore(lowWeight, 0.8, 0.8)
	highScore := compositeScore(highWeight, 0.8, 0.8)

	if highScore <= lowScore {
		t.Fatalf("expected higher source_weight to raise composite score, got high=%f low=%f", highScore, lowScore)
	}
}
