package repository

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

type documentAccessLogRepository struct {
	db *gorm.DB
}

func NewDocumentAccessLogRepository(db *gorm.DB) interfaces.DocumentAccessLogRepository {
	return &documentAccessLogRepository{db: db}
}

func (r *documentAccessLogRepository) BulkCreate(ctx context.Context, logs []*types.DocumentAccessLog) error {
	if len(logs) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).CreateInBatches(logs, 200).Error
}
