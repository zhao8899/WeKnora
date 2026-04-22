package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

type stubAnswerEvidenceRepo struct {
	message            *types.Message
	evidences          []*types.AnswerEvidence
	feedbackRows       []*types.SourceFeedback
	sourceWeights      map[string]float64
	replaceCalled      bool
	replaceErr         error
	gotReplaceTenantID uint64
	gotReplaceSession  string
	gotReplaceMessage  string
}

func (s *stubAnswerEvidenceRepo) ReplaceAnswerEvidence(
	ctx context.Context, tenantID uint64, sessionID, messageID string, evidences []*types.AnswerEvidence,
) error {
	s.replaceCalled = true
	s.gotReplaceTenantID = tenantID
	s.gotReplaceSession = sessionID
	s.gotReplaceMessage = messageID
	if s.replaceErr != nil {
		return s.replaceErr
	}
	s.evidences = evidences
	return nil
}

func (s *stubAnswerEvidenceRepo) AnswerMessageExists(ctx context.Context, tenantID uint64, messageID string) (bool, error) {
	return s.message != nil && s.message.ID == messageID, nil
}

func (s *stubAnswerEvidenceRepo) GetAnswerMessage(
	ctx context.Context, tenantID uint64, messageID string,
) (*types.Message, error) {
	if s.message == nil || s.message.ID != messageID {
		return nil, nil
	}
	return s.message, nil
}

func (s *stubAnswerEvidenceRepo) ListAnswerEvidence(
	ctx context.Context, tenantID uint64, messageID string,
) ([]*types.AnswerEvidence, error) {
	return s.evidences, nil
}

func (s *stubAnswerEvidenceRepo) GetAnswerEvidence(
	ctx context.Context, tenantID uint64, messageID, evidenceID string,
) (*types.AnswerEvidence, error) {
	for _, evidence := range s.evidences {
		if evidence.ID == evidenceID {
			return evidence, nil
		}
	}
	return nil, nil
}

func (s *stubAnswerEvidenceRepo) UpsertSourceFeedback(ctx context.Context, feedback *types.SourceFeedback) error {
	return nil
}

func (s *stubAnswerEvidenceRepo) ListSourceFeedbackByMessageAndUser(
	ctx context.Context, tenantID uint64, messageID, userID string,
) ([]*types.SourceFeedback, error) {
	return s.feedbackRows, nil
}

func (s *stubAnswerEvidenceRepo) FetchSourceWeights(ctx context.Context, knowledgeIDs []string) (map[string]float64, error) {
	return s.sourceWeights, nil
}

func tenantContext() context.Context {
	ctx := context.Background()
	ctx = context.WithValue(ctx, types.TenantIDContextKey, uint64(10000))
	ctx = context.WithValue(ctx, types.UserIDContextKey, "u1")
	return ctx
}

func TestComputeEvidenceStrengthScore_Empty(t *testing.T) {
	score := computeEvidenceStrengthScore(nil, nil)
	if score != 0 {
		t.Fatalf("expected 0 for empty evidences, got %f", score)
	}
}

func TestComputeEvidenceStrengthScore_SingleSource_NoWeight(t *testing.T) {
	evidences := []*types.AnswerEvidence{
		{SourceKnowledgeID: "k1", RerankScore: 0.8},
	}
	score := computeEvidenceStrengthScore(evidences, map[string]float64{})
	if score < 0.79 || score > 0.81 {
		t.Fatalf("expected ~0.80, got %f", score)
	}
}

func TestComputeEvidenceStrengthScore_SourceWeightBoostsScore(t *testing.T) {
	evidences := []*types.AnswerEvidence{
		{SourceKnowledgeID: "k1", RerankScore: 0.6},
	}
	weights := map[string]float64{"k1": 1.5}
	score := computeEvidenceStrengthScore(evidences, weights)
	// 0.6 * 1.5 = 0.9, capped at 1.0; no multi-source bonus
	if score < 0.89 || score > 1.0 {
		t.Fatalf("expected ~0.90 with weight 1.5, got %f", score)
	}
}

func TestComputeEvidenceStrengthScore_SourceWeightReducesScore(t *testing.T) {
	evidences := []*types.AnswerEvidence{
		{SourceKnowledgeID: "k1", RerankScore: 0.8},
	}
	weights := map[string]float64{"k1": 0.5}
	score := computeEvidenceStrengthScore(evidences, weights)
	// 0.8 * 0.5 = 0.40
	if score < 0.39 || score > 0.41 {
		t.Fatalf("expected ~0.40 with weight 0.5, got %f", score)
	}
}

func TestComputeEvidenceStrengthScore_MultiSourceBonus(t *testing.T) {
	evidences := []*types.AnswerEvidence{
		{SourceKnowledgeID: "k1", SourceType: "document", RerankScore: 0.7},
		{SourceKnowledgeID: "k2", SourceType: "faq", RerankScore: 0.6},
	}
	weights := map[string]float64{"k1": 1.0, "k2": 1.0}
	score := computeEvidenceStrengthScore(evidences, weights)
	// position-weighted: (0.7*1 + 0.6*0.5) / 1.5 = 0.667; +0.05 (>=2 sources) +0.05 (2 types) = 0.767 -> 0.77
	if score < 0.76 || score > 0.78 {
		t.Fatalf("expected ~0.77 with multi-source bonus, got %f", score)
	}
}

func TestComputeEvidenceStrengthScore_FallsBackToRetrievalScore(t *testing.T) {
	evidences := []*types.AnswerEvidence{
		{SourceKnowledgeID: "k1", RetrievalScore: 0.5, RerankScore: 0},
	}
	score := computeEvidenceStrengthScore(evidences, map[string]float64{"k1": 1.0})
	if score < 0.49 || score > 0.51 {
		t.Fatalf("expected ~0.50 using retrieval fallback, got %f", score)
	}
}

func TestComputeEvidenceStrengthScore_ZeroWeightDefaultsToOne(t *testing.T) {
	evidences := []*types.AnswerEvidence{
		{SourceKnowledgeID: "k1", RerankScore: 0.7},
	}
	scoreZeroWeight := computeEvidenceStrengthScore(evidences, map[string]float64{"k1": 0})
	scoreDefaultWeight := computeEvidenceStrengthScore(evidences, map[string]float64{"k1": 1.0})
	if scoreZeroWeight != scoreDefaultWeight {
		t.Fatalf("zero weight should default to 1.0: got %f vs %f", scoreZeroWeight, scoreDefaultWeight)
	}
}

func TestComputeSourceHealthScore_UsesSourceWeightAndFeedback(t *testing.T) {
	evidences := []*types.AnswerEvidence{
		{ID: "e1", SourceKnowledgeID: "k1"},
		{ID: "e2", SourceKnowledgeID: "k2"},
	}
	score := computeSourceHealthScore(evidences, map[string]float64{
		"k1": 0.9,
		"k2": 0.8,
	}, map[string]string{
		"e1": types.SourceFeedbackUp,
		"e2": types.SourceFeedbackExpired,
	})
	if score < 0.82 || score > 0.84 {
		t.Fatalf("expected ~0.83 source health score, got %f", score)
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

func TestGetAnswerConfidence_UsesPersistedEvidence(t *testing.T) {
	repo := &stubAnswerEvidenceRepo{
		message: &types.Message{
			ID:                  "m1",
			SessionID:           "s1",
			KnowledgeReferences: types.References{{KnowledgeID: "k1"}},
		},
		evidences: []*types.AnswerEvidence{
			{ID: "e1", SourceKnowledgeID: "k1", SourceType: types.SourceTypeDocument, RerankScore: 0.82, Position: 1},
		},
		sourceWeights: map[string]float64{"k1": 1.0},
	}

	svc := NewConfidenceService(repo)
	resp, err := svc.GetAnswerConfidence(tenantContext(), "m1")
	if err != nil {
		t.Fatalf("GetAnswerConfidence returned error: %v", err)
	}
	if resp.EvidenceStatus != "ready" {
		t.Fatalf("expected evidence status ready, got %q", resp.EvidenceStatus)
	}
	if resp.EvidenceStrengthScore == 0 || resp.SourceHealthScore == 0 {
		t.Fatalf("expected both evidence strength and source health to be populated, got %#v", resp)
	}
	if resp.ConfidenceScore != resp.EvidenceStrengthScore {
		t.Fatalf("expected legacy confidence_score to mirror evidence strength, got %f vs %f", resp.ConfidenceScore, resp.EvidenceStrengthScore)
	}
	if resp.SourceCount != 1 || resp.ReferenceCount != 1 {
		t.Fatalf("expected 1 source and 1 reference, got %d and %d", resp.SourceCount, resp.ReferenceCount)
	}
	if repo.replaceCalled {
		t.Fatal("did not expect ReplaceAnswerEvidence to be called when evidence already exists")
	}
}

func TestGetAnswerConfidence_RecoversEvidenceFromReferences(t *testing.T) {
	repo := &stubAnswerEvidenceRepo{
		message: &types.Message{
			ID:        "m1",
			SessionID: "s1",
			KnowledgeReferences: types.References{
				{
					ID:               "chunk-1",
					KnowledgeID:      "k1",
					KnowledgeTitle:   "Doc 1",
					KnowledgeBaseID:  "kb1",
					KnowledgeChannel: "web",
					MatchType:        types.MatchTypeEmbedding,
					Score:            0.76,
					Metadata:         map[string]string{"final_score": "0.81"},
				},
			},
		},
		sourceWeights: map[string]float64{"k1": 1.0},
	}

	svc := NewConfidenceService(repo)
	resp, err := svc.GetAnswerConfidence(tenantContext(), "m1")
	if err != nil {
		t.Fatalf("GetAnswerConfidence returned error: %v", err)
	}
	if !repo.replaceCalled {
		t.Fatal("expected ReplaceAnswerEvidence to be called for self-healing recovery")
	}
	if resp.EvidenceStatus != "recovered" {
		t.Fatalf("expected recovered evidence status, got %q", resp.EvidenceStatus)
	}
	if resp.SourceCount != 1 || len(resp.Evidences) != 1 {
		t.Fatalf("expected 1 recovered evidence, got %d", resp.SourceCount)
	}
	if resp.EvidenceStrengthLabel == "" || resp.SourceHealthLabel == "" {
		t.Fatalf("expected labels for both dimensions, got %#v", resp)
	}
	if resp.Evidences[0].Title != "Doc 1" {
		t.Fatalf("expected recovered title Doc 1, got %q", resp.Evidences[0].Title)
	}
}

func TestGetAnswerConfidence_DegradesWhenRecoveryPersistenceFails(t *testing.T) {
	repo := &stubAnswerEvidenceRepo{
		message: &types.Message{
			ID:        "m1",
			SessionID: "s1",
			KnowledgeReferences: types.References{
				{ID: "chunk-1", KnowledgeID: "k1", KnowledgeTitle: "Doc 1", MatchType: types.MatchTypeEmbedding, Score: 0.61},
			},
		},
		sourceWeights: map[string]float64{"k1": 1.0},
		replaceErr:    errors.New("db unavailable"),
	}

	svc := NewConfidenceService(repo)
	resp, err := svc.GetAnswerConfidence(tenantContext(), "m1")
	if err != nil {
		t.Fatalf("GetAnswerConfidence returned error: %v", err)
	}
	if resp.EvidenceStatus != "degraded" {
		t.Fatalf("expected degraded evidence status, got %q", resp.EvidenceStatus)
	}
	if resp.SourceCount != 1 {
		t.Fatalf("expected transient evidence in response, got %d", resp.SourceCount)
	}
}

func TestGetAnswerConfidence_MissingWhenNoReferences(t *testing.T) {
	repo := &stubAnswerEvidenceRepo{
		message: &types.Message{
			ID:        "m1",
			SessionID: "s1",
		},
		sourceWeights: map[string]float64{},
	}

	svc := NewConfidenceService(repo)
	resp, err := svc.GetAnswerConfidence(tenantContext(), "m1")
	if err != nil {
		t.Fatalf("GetAnswerConfidence returned error: %v", err)
	}
	if resp.EvidenceStatus != "missing" {
		t.Fatalf("expected missing evidence status, got %q", resp.EvidenceStatus)
	}
	if resp.SourceCount != 0 || resp.ReferenceCount != 0 {
		t.Fatalf("expected no evidence and no references, got %d and %d", resp.SourceCount, resp.ReferenceCount)
	}
}
