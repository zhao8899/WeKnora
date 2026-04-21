package chatpipeline

import (
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

func TestBuildAnswerEvidence_UsesBaseAndFinalScores(t *testing.T) {
	cm := &types.ChatManage{
		PipelineRequest: types.PipelineRequest{
			TenantID:  10007,
			SessionID: "session-1",
		},
		PipelineContext: types.PipelineContext{
			MessageID: "message-1",
		},
		PipelineState: types.PipelineState{
			MergeResult: []*types.SearchResult{
				{
					ID:               "chunk-1",
					KnowledgeID:      "knowledge-1",
					KnowledgeBaseID:  "kb-1",
					KnowledgeTitle:   "Doc 1",
					KnowledgeChannel: "web",
					Score:            0.82,
					Metadata: map[string]string{
						"base_score":  "0.61",
						"final_score": "0.82",
					},
				},
			},
		},
	}

	evidences := buildAnswerEvidence(cm)
	if len(evidences) != 1 {
		t.Fatalf("expected 1 evidence, got %d", len(evidences))
	}
	if evidences[0].RetrievalScore != 0.61 {
		t.Fatalf("expected retrieval_score=0.61, got %f", evidences[0].RetrievalScore)
	}
	if evidences[0].RerankScore != 0.82 {
		t.Fatalf("expected rerank_score=0.82, got %f", evidences[0].RerankScore)
	}
}
