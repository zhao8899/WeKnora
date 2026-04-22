package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

type AnswerEvidenceRepository interface {
	ReplaceAnswerEvidence(ctx context.Context, tenantID uint64, sessionID, messageID string, evidences []*types.AnswerEvidence) error
	AnswerMessageExists(ctx context.Context, tenantID uint64, messageID string) (bool, error)
	GetAnswerMessage(ctx context.Context, tenantID uint64, messageID string) (*types.Message, error)
	ListAnswerEvidence(ctx context.Context, tenantID uint64, messageID string) ([]*types.AnswerEvidence, error)
	GetAnswerEvidence(ctx context.Context, tenantID uint64, messageID, evidenceID string) (*types.AnswerEvidence, error)
	UpsertSourceFeedback(ctx context.Context, feedback *types.SourceFeedback) error
	ListSourceFeedbackByMessageAndUser(ctx context.Context, tenantID uint64, messageID, userID string) ([]*types.SourceFeedback, error)
	// FetchSourceWeights returns source_weight for each knowledge ID; missing IDs default to 1.0.
	FetchSourceWeights(ctx context.Context, knowledgeIDs []string) (map[string]float64, error)
}

type ConfidenceService interface {
	GetAnswerConfidence(ctx context.Context, messageID string) (*types.AnswerConfidenceResponse, error)
	SubmitSourceFeedback(ctx context.Context, messageID, evidenceID, feedback, comment string) error
}
