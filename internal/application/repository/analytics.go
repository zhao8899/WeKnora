package repository

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/gorm"
)

type AnalyticsRepository struct {
	db *gorm.DB
}

func NewAnalyticsRepository(db *gorm.DB) *AnalyticsRepository {
	return &AnalyticsRepository{db: db}
}

func (r *AnalyticsRepository) HotQuestions(
	ctx context.Context, tenantID uint64, limit int,
) ([]*types.HotQuestion, error) {
	var rows []*types.HotQuestion
	err := r.db.WithContext(ctx).Raw(`
		SELECT
			answer.id AS message_id,
			answer.session_id,
			COALESCE(question.content, answer.content) AS question,
			COUNT(*) FILTER (WHERE dal.access_type = 'retrieved') AS retrieved_count,
			COUNT(*) FILTER (WHERE dal.access_type = 'reranked') AS reranked_count,
			COUNT(*) FILTER (WHERE dal.access_type = 'cited') AS cited_count,
			MAX(dal.created_at) AS last_access_at
		FROM document_access_logs dal
		INNER JOIN messages answer ON answer.id = dal.message_id
		INNER JOIN sessions s ON s.id = answer.session_id AND s.tenant_id = ?
		LEFT JOIN LATERAL (
			SELECT m2.content
			FROM messages m2
			WHERE m2.session_id = answer.session_id
			  AND m2.role = 'user'
			  AND m2.deleted_at IS NULL
			  AND m2.created_at <= answer.created_at
			ORDER BY m2.created_at DESC
			LIMIT 1
		) AS question ON TRUE
		WHERE dal.tenant_id = ?
		  AND dal.created_at >= NOW() - INTERVAL '30 days'
		  AND answer.role = 'assistant'
		  AND answer.deleted_at IS NULL
		GROUP BY answer.id, answer.session_id, question.content, answer.content
		ORDER BY retrieved_count DESC, reranked_count DESC, cited_count DESC, last_access_at DESC
		LIMIT ?
	`, tenantID, tenantID, limit).Scan(&rows).Error
	return rows, err
}

func (r *AnalyticsRepository) CoverageGaps(
	ctx context.Context, tenantID uint64, limit int,
) ([]*types.CoverageGap, error) {
	var rows []*types.CoverageGap
	err := r.db.WithContext(ctx).Raw(`
		WITH answer_stats AS (
			SELECT
				answer.id AS message_id,
				answer.session_id,
				answer.created_at AS answer_created_at,
				COALESCE(question.content, answer.content) AS question,
				COUNT(ae.id) AS source_count,
				COALESCE(AVG(CASE WHEN ae.rerank_score > 0 THEN ae.rerank_score ELSE ae.retrieval_score END), 0) AS confidence_score
			FROM messages answer
			INNER JOIN sessions s ON s.id = answer.session_id AND s.tenant_id = ?
			LEFT JOIN answer_evidence ae ON ae.answer_message_id = answer.id AND ae.tenant_id = ?
			LEFT JOIN LATERAL (
				SELECT m2.content
				FROM messages m2
				WHERE m2.session_id = answer.session_id
				  AND m2.role = 'user'
				  AND m2.deleted_at IS NULL
				  AND m2.created_at <= answer.created_at
				ORDER BY m2.created_at DESC
				LIMIT 1
			) AS question ON TRUE
			WHERE answer.role = 'assistant'
			  AND answer.deleted_at IS NULL
			  AND answer.created_at >= NOW() - INTERVAL '30 days'
			GROUP BY answer.id, answer.session_id, answer.created_at, question.content, answer.content
		)
		SELECT
			message_id,
			session_id,
			question,
			confidence_score,
			CASE
				WHEN confidence_score >= 0.75 THEN 'high'
				WHEN confidence_score >= 0.45 THEN 'medium'
				ELSE 'low'
			END AS confidence_label,
			source_count,
			answer_created_at
		FROM answer_stats
		WHERE confidence_score < 0.4 OR source_count = 0
		ORDER BY confidence_score ASC, source_count ASC, answer_created_at DESC
		LIMIT ?
	`, tenantID, tenantID, limit).Scan(&rows).Error
	return rows, err
}

func (r *AnalyticsRepository) StaleDocuments(
	ctx context.Context, tenantID uint64, limit int,
) ([]*types.StaleDocument, error) {
	var rows []*types.StaleDocument
	err := r.db.WithContext(ctx).Raw(`
		SELECT
			k.id AS knowledge_id,
			k.title,
			k.source_weight,
			k.freshness_flag,
			COALESCE(COUNT(sf.id), 0) AS down_feedback_count,
			MAX(sf.created_at) AS last_feedback_at
		FROM knowledges k
		LEFT JOIN answer_evidence ae ON ae.source_knowledge_id = k.id AND ae.tenant_id = k.tenant_id
		LEFT JOIN source_feedback sf ON sf.answer_evidence_id = ae.id AND sf.feedback = 'down'
		WHERE k.tenant_id = ?
		  AND k.deleted_at IS NULL
		  AND (k.freshness_flag = TRUE OR k.source_weight < 1.0)
		GROUP BY k.id, k.title, k.source_weight, k.freshness_flag
		ORDER BY k.freshness_flag DESC, k.source_weight ASC, last_feedback_at DESC NULLS LAST
		LIMIT ?
	`, tenantID, limit).Scan(&rows).Error
	return rows, err
}

func (r *AnalyticsRepository) CitationHeatmap(
	ctx context.Context, tenantID uint64, limit int,
) ([]*types.CitationHeat, error) {
	var rows []*types.CitationHeat
	err := r.db.WithContext(ctx).Raw(`
		SELECT
			k.id AS knowledge_id,
			k.title,
			COUNT(*) FILTER (WHERE dal.access_type = 'cited') AS cited_count,
			COUNT(*) FILTER (WHERE dal.access_type = 'reranked') AS reranked_count,
			COUNT(*) FILTER (WHERE dal.access_type = 'retrieved') AS retrieved_count,
			k.source_weight,
			k.freshness_flag
		FROM document_access_logs dal
		INNER JOIN knowledges k ON k.id = dal.knowledge_id AND k.tenant_id = dal.tenant_id
		WHERE dal.tenant_id = ?
		  AND dal.created_at >= NOW() - INTERVAL '30 days'
		  AND k.deleted_at IS NULL
		GROUP BY k.id, k.title, k.source_weight, k.freshness_flag
		ORDER BY cited_count DESC, reranked_count DESC, retrieved_count DESC, k.title ASC
		LIMIT ?
	`, tenantID, limit).Scan(&rows).Error
	return rows, err
}
