package service

import (
	"context"
	"fmt"
	"math"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

type confidenceService struct {
	repo interfaces.AnswerEvidenceRepository
}

func NewConfidenceService(repo interfaces.AnswerEvidenceRepository) interfaces.ConfidenceService {
	return &confidenceService{repo: repo}
}

func (s *confidenceService) GetAnswerConfidence(
	ctx context.Context, messageID string,
) (*types.AnswerConfidenceResponse, error) {
	tenantID := types.MustTenantIDFromContext(ctx)
	exists, err := s.repo.AnswerMessageExists(ctx, tenantID, messageID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("answer message not found")
	}

	evidences, err := s.repo.ListAnswerEvidence(ctx, tenantID, messageID)
	if err != nil {
		return nil, err
	}

	knowledgeIDs := make([]string, 0, len(evidences))
	for _, e := range evidences {
		if e.SourceKnowledgeID != "" {
			knowledgeIDs = append(knowledgeIDs, e.SourceKnowledgeID)
		}
	}
	sourceWeights, err := s.repo.FetchSourceWeights(ctx, knowledgeIDs)
	if err != nil {
		return nil, err
	}

	userID, _ := types.UserIDFromContext(ctx)
	feedbackRows, err := s.repo.ListSourceFeedbackByMessageAndUser(ctx, tenantID, messageID, userID)
	if err != nil {
		return nil, err
	}
	feedbackByEvidenceID := make(map[string]string, len(feedbackRows))
	for _, row := range feedbackRows {
		feedbackByEvidenceID[row.AnswerEvidenceID] = row.Feedback
	}

	resp := &types.AnswerConfidenceResponse{
		MessageID:        messageID,
		SourceCount:      len(evidences),
		SourceTypeCounts: make(map[string]int),
		Evidences:        make([]*types.AnswerConfidenceEvidenceItem, 0, len(evidences)),
	}

	for _, evidence := range evidences {
		resp.SourceTypeCounts[evidence.SourceType]++
		resp.Evidences = append(resp.Evidences, &types.AnswerConfidenceEvidenceItem{
			ID:              evidence.ID,
			KnowledgeID:     evidence.SourceKnowledgeID,
			KnowledgeBaseID: evidence.SourceKnowledgeBaseID,
			ChunkID:         evidence.SourceChunkID,
			Title:           evidence.SourceTitle,
			SourceType:      evidence.SourceType,
			SourceChannel:   evidence.SourceChannel,
			MatchType:       evidence.MatchType,
			RetrievalScore:  evidence.RetrievalScore,
			RerankScore:     evidence.RerankScore,
			Position:        evidence.Position,
			CurrentFeedback: feedbackByEvidenceID[evidence.ID],
		})
	}

	resp.ConfidenceScore = computeConfidenceScore(evidences, sourceWeights)
	resp.ConfidenceLabel = confidenceLabel(resp.ConfidenceScore)
	return resp, nil
}

func (s *confidenceService) SubmitSourceFeedback(
	ctx context.Context, messageID, evidenceID, feedback, comment string,
) error {
	if feedback != types.SourceFeedbackUp && feedback != types.SourceFeedbackDown && feedback != types.SourceFeedbackExpired {
		return fmt.Errorf("invalid feedback value")
	}

	tenantID := types.MustTenantIDFromContext(ctx)
	exists, err := s.repo.AnswerMessageExists(ctx, tenantID, messageID)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("answer message not found")
	}

	evidence, err := s.repo.GetAnswerEvidence(ctx, tenantID, messageID, evidenceID)
	if err != nil {
		return err
	}
	if evidence == nil {
		return fmt.Errorf("answer evidence not found")
	}

	userID, _ := types.UserIDFromContext(ctx)
	return s.repo.UpsertSourceFeedback(ctx, &types.SourceFeedback{
		TenantID:         tenantID,
		AnswerMessageID:  messageID,
		AnswerEvidenceID: evidenceID,
		UserID:           userID,
		Feedback:         feedback,
		Comment:          comment,
	})
}

func computeConfidenceScore(evidences []*types.AnswerEvidence, sourceWeights map[string]float64) float64 {
	if len(evidences) == 0 {
		return 0
	}
	var weightedScore float64
	var totalWeight float64
	sourceTypes := make(map[string]struct{})
	for i, evidence := range evidences {
		score := evidence.RerankScore
		if score <= 0 {
			score = evidence.RetrievalScore
		}
		if score < 0 {
			score = 0
		}
		if score > 1 {
			score = 1
		}
		sw := sourceWeights[evidence.SourceKnowledgeID]
		if sw <= 0 {
			sw = 1.0
		}
		weight := 1 / float64(i+1)
		weightedScore += score * sw * weight
		totalWeight += weight
		sourceTypes[evidence.SourceType] = struct{}{}
	}
	base := weightedScore / totalWeight
	if len(evidences) >= 2 {
		base += 0.05
	}
	if len(sourceTypes) >= 2 {
		base += 0.05
	}
	return math.Min(1, math.Round(base*100)/100)
}

func confidenceLabel(score float64) string {
	switch {
	case score >= 0.85:
		return "high"
	case score >= 0.60:
		return "medium"
	case score >= 0.40:
		return "low"
	default:
		return "insufficient"
	}
}
