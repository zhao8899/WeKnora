package service

import (
	"context"

	"github.com/Tencent/WeKnora/internal/logger"
	"gorm.io/gorm"
)

const sourceWeightUpdateSQL = `
UPDATE knowledges
SET source_weight = 1.0
WHERE deleted_at IS NULL;

WITH feedback_stats AS (
	SELECT
		ae.source_knowledge_id AS knowledge_id,
		SUM(CASE WHEN sf.feedback = 'up' THEN 1 ELSE 0 END) AS up_count,
		SUM(CASE WHEN sf.feedback = 'down' THEN 1 ELSE 0 END) AS down_count,
		SUM(CASE WHEN sf.feedback = 'expired' THEN 1 ELSE 0 END) AS expired_count
	FROM source_feedback sf
	INNER JOIN answer_evidence ae ON ae.id = sf.answer_evidence_id
	WHERE NULLIF(ae.source_knowledge_id, '') IS NOT NULL
		AND sf.created_at > NOW() - INTERVAL '30 days'
	GROUP BY ae.source_knowledge_id
)
UPDATE knowledges k
SET source_weight = GREATEST(
	0.1,
	LEAST(
		2.0,
		1.0 + (feedback_stats.up_count * 0.02) - (feedback_stats.down_count * 0.05) - (feedback_stats.expired_count * 0.02)
	)
)
FROM feedback_stats
WHERE k.id = feedback_stats.knowledge_id
	AND k.deleted_at IS NULL;
`

const freshnessFlagUpdateSQL = `
UPDATE knowledges
SET freshness_flag = FALSE
WHERE deleted_at IS NULL;

WITH stale_candidates AS (
	SELECT DISTINCT ae.source_knowledge_id AS knowledge_id
	FROM source_feedback sf
	INNER JOIN answer_evidence ae ON ae.id = sf.answer_evidence_id
	WHERE NULLIF(ae.source_knowledge_id, '') IS NOT NULL
		AND sf.feedback IN ('down', 'expired')
		AND sf.created_at > NOW() - INTERVAL '7 days'
)
UPDATE knowledges k
SET freshness_flag = TRUE
FROM stale_candidates
WHERE k.id = stale_candidates.knowledge_id
	AND k.deleted_at IS NULL;
`

type SourceWeightUpdater struct {
	db *gorm.DB
}

func NewSourceWeightUpdater(db *gorm.DB) *SourceWeightUpdater {
	return &SourceWeightUpdater{db: db}
}

func (u *SourceWeightUpdater) Run(ctx context.Context) error {
	if err := u.db.WithContext(ctx).Exec(sourceWeightUpdateSQL).Error; err != nil {
		return err
	}
	if err := u.db.WithContext(ctx).Exec(freshnessFlagUpdateSQL).Error; err != nil {
		return err
	}
	logger.Infof(ctx, "[SourceWeightUpdater] source weights and freshness flags updated")
	return nil
}
