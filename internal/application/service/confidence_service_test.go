package service

import (
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

func TestComputeConfidenceScore_Empty(t *testing.T) {
	score := computeConfidenceScore(nil, nil)
	if score != 0 {
		t.Fatalf("expected 0 for empty evidences, got %f", score)
	}
}

func TestComputeConfidenceScore_SingleSource_NoWeight(t *testing.T) {
	evidences := []*types.AnswerEvidence{
		{SourceKnowledgeID: "k1", RerankScore: 0.8},
	}
	score := computeConfidenceScore(evidences, map[string]float64{})
	if score < 0.79 || score > 0.81 {
		t.Fatalf("expected ~0.80, got %f", score)
	}
}

func TestComputeConfidenceScore_SourceWeightBoostsScore(t *testing.T) {
	evidences := []*types.AnswerEvidence{
		{SourceKnowledgeID: "k1", RerankScore: 0.6},
	}
	weights := map[string]float64{"k1": 1.5}
	score := computeConfidenceScore(evidences, weights)
	// 0.6 * 1.5 = 0.9, capped at 1.0; no multi-source bonus
	if score < 0.89 || score > 1.0 {
		t.Fatalf("expected ~0.90 with weight 1.5, got %f", score)
	}
}

func TestComputeConfidenceScore_SourceWeightReducesScore(t *testing.T) {
	evidences := []*types.AnswerEvidence{
		{SourceKnowledgeID: "k1", RerankScore: 0.8},
	}
	weights := map[string]float64{"k1": 0.5}
	score := computeConfidenceScore(evidences, weights)
	// 0.8 * 0.5 = 0.40
	if score < 0.39 || score > 0.41 {
		t.Fatalf("expected ~0.40 with weight 0.5, got %f", score)
	}
}

func TestComputeConfidenceScore_MultiSourceBonus(t *testing.T) {
	evidences := []*types.AnswerEvidence{
		{SourceKnowledgeID: "k1", SourceType: "document", RerankScore: 0.7},
		{SourceKnowledgeID: "k2", SourceType: "faq", RerankScore: 0.6},
	}
	weights := map[string]float64{"k1": 1.0, "k2": 1.0}
	score := computeConfidenceScore(evidences, weights)
	// position-weighted: (0.7*1 + 0.6*0.5) / 1.5 = 0.667; +0.05 (>=2 sources) +0.05 (2 types) = 0.767 -> 0.77
	if score < 0.76 || score > 0.78 {
		t.Fatalf("expected ~0.77 with multi-source bonus, got %f", score)
	}
}

func TestComputeConfidenceScore_FallsBackToRetrievalScore(t *testing.T) {
	evidences := []*types.AnswerEvidence{
		{SourceKnowledgeID: "k1", RetrievalScore: 0.5, RerankScore: 0},
	}
	score := computeConfidenceScore(evidences, map[string]float64{"k1": 1.0})
	if score < 0.49 || score > 0.51 {
		t.Fatalf("expected ~0.50 using retrieval fallback, got %f", score)
	}
}

func TestComputeConfidenceScore_ZeroWeightDefaultsToOne(t *testing.T) {
	evidences := []*types.AnswerEvidence{
		{SourceKnowledgeID: "k1", RerankScore: 0.7},
	}
	scoreZeroWeight := computeConfidenceScore(evidences, map[string]float64{"k1": 0})
	scoreDefaultWeight := computeConfidenceScore(evidences, map[string]float64{"k1": 1.0})
	if scoreZeroWeight != scoreDefaultWeight {
		t.Fatalf("zero weight should default to 1.0: got %f vs %f", scoreZeroWeight, scoreDefaultWeight)
	}
}

func TestConfidenceLabel_Thresholds(t *testing.T) {
	cases := []struct {
		score float64
		want  string
	}{
		{1.00, "high"},
		{0.85, "high"},
		{0.84, "medium"},
		{0.60, "medium"},
		{0.59, "low"},
		{0.40, "low"},
		{0.39, "insufficient"},
		{0.00, "insufficient"},
	}
	for _, c := range cases {
		got := confidenceLabel(c.score)
		if got != c.want {
			t.Errorf("confidenceLabel(%v) = %q, want %q", c.score, got, c.want)
		}
	}
}
