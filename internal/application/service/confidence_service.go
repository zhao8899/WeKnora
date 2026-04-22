package service

import (
	"context"
	"fmt"
	"math"
	"strconv"

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
	message, err := s.repo.GetAnswerMessage(ctx, tenantID, messageID)
	if err != nil {
		return nil, err
	}
	if message == nil {
		return nil, fmt.Errorf("answer message not found")
	}

	evidences, err := s.repo.ListAnswerEvidence(ctx, tenantID, messageID)
	if err != nil {
		return nil, err
	}
	evidenceStatus := "ready"
	if len(evidences) == 0 {
		switch refs := message.KnowledgeReferences; {
		case len(refs) > 0:
			evidences = buildFallbackAnswerEvidence(tenantID, message)
			if len(evidences) > 0 {
				if err := s.repo.ReplaceAnswerEvidence(ctx, tenantID, message.SessionID, message.ID, evidences); err != nil {
					evidenceStatus = "degraded"
				} else {
					evidenceStatus = "recovered"
				}
			} else {
				evidenceStatus = "missing"
			}
		default:
			evidenceStatus = "missing"
		}
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
		ReferenceCount:   len(message.KnowledgeReferences),
		EvidenceStatus:   evidenceStatus,
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

	resp.EvidenceStrengthScore = computeEvidenceStrengthScore(evidences, sourceWeights)
	resp.EvidenceStrengthLabel = confidenceLabel(resp.EvidenceStrengthScore)
	resp.SourceHealthScore = computeSourceHealthScore(evidences, sourceWeights, feedbackByEvidenceID)
	resp.SourceHealthLabel = types.SourceHealthLabel(resp.SourceHealthScore)
	// Keep legacy fields for backward compatibility. These now mirror evidence strength.
	resp.ConfidenceScore = resp.EvidenceStrengthScore
	resp.ConfidenceLabel = resp.EvidenceStrengthLabel
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

func computeEvidenceStrengthScore(evidences []*types.AnswerEvidence, sourceWeights map[string]float64) float64 {
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

func computeSourceHealthScore(
	evidences []*types.AnswerEvidence, sourceWeights map[string]float64, feedbackByEvidenceID map[string]string,
) float64 {
	if len(evidences) == 0 {
		return 0
	}

	var weightedScore float64
	var totalWeight float64
	seenKnowledgeIDs := make(map[string]struct{})

	for i, evidence := range evidences {
		health := sourceWeights[evidence.SourceKnowledgeID]
		if health <= 0 {
			health = 1.0
		}
		if health > 1 {
			health = 1
		}

		switch feedbackByEvidenceID[evidence.ID] {
		case types.SourceFeedbackUp:
			health += 0.05
		case types.SourceFeedbackDown:
			health -= 0.20
		case types.SourceFeedbackExpired:
			health -= 0.30
		}

		if health < 0 {
			health = 0
		}
		if health > 1 {
			health = 1
		}

		weight := 1 / float64(i+1)
		weightedScore += health * weight
		totalWeight += weight
		if evidence.SourceKnowledgeID != "" {
			seenKnowledgeIDs[evidence.SourceKnowledgeID] = struct{}{}
		}
	}

	base := weightedScore / totalWeight
	if len(seenKnowledgeIDs) >= 2 {
		base += 0.03
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

func buildFallbackAnswerEvidence(tenantID uint64, message *types.Message) []*types.AnswerEvidence {
	if message == nil || len(message.KnowledgeReferences) == 0 {
		return nil
	}

	evidences := make([]*types.AnswerEvidence, 0, len(message.KnowledgeReferences))
	for i, result := range message.KnowledgeReferences {
		if result == nil {
			continue
		}
		evidences = append(evidences, &types.AnswerEvidence{
			TenantID:              tenantID,
			SessionID:             message.SessionID,
			AnswerMessageID:       message.ID,
			SourceKnowledgeID:     result.KnowledgeID,
			SourceKnowledgeBaseID: result.KnowledgeBaseID,
			SourceChunkID:         result.ID,
			SourceTitle:           chooseConfidenceSourceTitle(result),
			SourceType:            inferConfidenceSourceType(result),
			SourceChannel:         result.KnowledgeChannel,
			MatchType:             mapConfidenceMatchType(result.MatchType),
			RetrievalScore:        parseConfidenceScore(result, "base_score", result.Score),
			RerankScore:           parseConfidenceScore(result, "final_score", result.Score),
			Position:              i + 1,
		})
	}
	return evidences
}

func chooseConfidenceSourceTitle(result *types.SearchResult) string {
	if result.KnowledgeTitle != "" {
		return result.KnowledgeTitle
	}
	if result.KnowledgeFilename != "" {
		return result.KnowledgeFilename
	}
	return result.KnowledgeID
}

func inferConfidenceSourceType(result *types.SearchResult) string {
	if result.ChunkType == string(types.ChunkTypeFAQ) {
		return types.SourceTypeFAQ
	}
	if result.MatchType == types.MatchTypeWebSearch || result.KnowledgeSource == "url" {
		return types.SourceTypeWeb
	}
	return types.SourceTypeDocument
}

func mapConfidenceMatchType(matchType types.MatchType) string {
	switch matchType {
	case types.MatchTypeEmbedding:
		return "vector"
	case types.MatchTypeKeywords:
		return "keyword"
	case types.MatchTypeNearByChunk:
		return "nearby"
	case types.MatchTypeHistory:
		return "history"
	case types.MatchTypeParentChunk:
		return "parent_chunk"
	case types.MatchTypeRelationChunk:
		return "relation_chunk"
	case types.MatchTypeGraph:
		return "graph"
	case types.MatchTypeWebSearch:
		return "web_search"
	case types.MatchTypeDirectLoad:
		return "direct_load"
	case types.MatchTypeDataAnalysis:
		return "data_analysis"
	default:
		return "unknown"
	}
}

func parseConfidenceScore(result *types.SearchResult, key string, fallback float64) float64 {
	if result == nil || result.Metadata == nil {
		return fallback
	}
	raw := result.Metadata[key]
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return fallback
	}
	return parsed
}
