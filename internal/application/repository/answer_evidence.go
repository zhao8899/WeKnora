package repository

import (
	"context"
	"errors"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type answerEvidenceRepository struct {
	db *gorm.DB
}

func NewAnswerEvidenceRepository(db *gorm.DB) interfaces.AnswerEvidenceRepository {
	return &answerEvidenceRepository{db: db}
}

func (r *answerEvidenceRepository) ReplaceAnswerEvidence(
	ctx context.Context, tenantID uint64, sessionID, messageID string, evidences []*types.AnswerEvidence,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where(
			"tenant_id = ? AND session_id = ? AND answer_message_id = ?",
			tenantID, sessionID, messageID,
		).Delete(&types.AnswerEvidence{}).Error; err != nil {
			return err
		}
		if len(evidences) == 0 {
			return nil
		}
		return tx.Create(&evidences).Error
	})
}

func (r *answerEvidenceRepository) AnswerMessageExists(ctx context.Context, tenantID uint64, messageID string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&types.Message{}).
		Joins("JOIN sessions ON sessions.id = messages.session_id AND sessions.deleted_at IS NULL").
		Where("messages.id = ? AND messages.role = 'assistant' AND messages.deleted_at IS NULL AND sessions.tenant_id = ?", messageID, tenantID).
		Count(&count).Error
	return count > 0, err
}

func (r *answerEvidenceRepository) ListAnswerEvidence(
	ctx context.Context, tenantID uint64, messageID string,
) ([]*types.AnswerEvidence, error) {
	var evidences []*types.AnswerEvidence
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND answer_message_id = ?", tenantID, messageID).
		Order("position ASC, created_at ASC").
		Find(&evidences).Error
	return evidences, err
}

func (r *answerEvidenceRepository) GetAnswerEvidence(
	ctx context.Context, tenantID uint64, messageID, evidenceID string,
) (*types.AnswerEvidence, error) {
	var evidence types.AnswerEvidence
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND answer_message_id = ? AND id = ?", tenantID, messageID, evidenceID).
		First(&evidence).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &evidence, nil
}

func (r *answerEvidenceRepository) UpsertSourceFeedback(ctx context.Context, feedback *types.SourceFeedback) error {
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "answer_message_id"},
				{Name: "answer_evidence_id"},
				{Name: "user_id"},
			},
			DoUpdates: clause.AssignmentColumns([]string{"feedback", "comment", "updated_at"}),
		}).
		Create(feedback).Error
}

func (r *answerEvidenceRepository) ListSourceFeedbackByMessageAndUser(
	ctx context.Context, tenantID uint64, messageID, userID string,
) ([]*types.SourceFeedback, error) {
	var rows []*types.SourceFeedback
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND answer_message_id = ? AND user_id = ?", tenantID, messageID, userID).
		Find(&rows).Error
	return rows, err
}

func (r *answerEvidenceRepository) FetchSourceWeights(
	ctx context.Context, knowledgeIDs []string,
) (map[string]float64, error) {
	if len(knowledgeIDs) == 0 {
		return map[string]float64{}, nil
	}
	type row struct {
		ID           string
		SourceWeight float64
	}
	var rows []row
	err := r.db.WithContext(ctx).
		Raw("SELECT id, source_weight FROM knowledges WHERE id IN ? AND deleted_at IS NULL", knowledgeIDs).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	weights := make(map[string]float64, len(rows))
	for _, r := range rows {
		weights[r.ID] = r.SourceWeight
	}
	return weights, nil
}
