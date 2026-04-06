package repository

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/gorm"
)

type auditLogRepository struct {
	db *gorm.DB
}

func NewAuditLogRepository(db *gorm.DB) *auditLogRepository {
	return &auditLogRepository{db: db}
}

func (r *auditLogRepository) Create(ctx context.Context, log *types.AuditLog) error {
	return r.db.WithContext(ctx).Create(log).Error
}

func (r *auditLogRepository) List(ctx context.Context, q types.AuditLogQuery) ([]*types.AuditLog, int64, error) {
	db := r.db.WithContext(ctx).Where("tenant_id = ?", q.TenantID)

	if q.UserID != "" {
		db = db.Where("user_id = ?", q.UserID)
	}
	if q.Action != "" {
		db = db.Where("action = ?", q.Action)
	}
	if q.ResourceType != "" {
		db = db.Where("resource_type = ?", q.ResourceType)
	}
	if q.ResourceID != "" {
		db = db.Where("resource_id = ?", q.ResourceID)
	}
	if q.StartTime != nil {
		db = db.Where("created_at >= ?", q.StartTime)
	}
	if q.EndTime != nil {
		db = db.Where("created_at <= ?", q.EndTime)
	}

	var total int64
	if err := db.Model(&types.AuditLog{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	page := q.Page
	if page < 1 {
		page = 1
	}
	pageSize := q.PageSize
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}

	var logs []*types.AuditLog
	err := db.Order("created_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&logs).Error
	return logs, total, err
}
