package repository

import (
	"context"
	"fmt"

	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/gorm"
)

type AnalyticsRepository struct {
	db *gorm.DB
}

func NewAnalyticsRepository(db *gorm.DB) *AnalyticsRepository {
	return &AnalyticsRepository{db: db}
}

func evidenceStrengthLabelCase(scoreExpr string) string {
	return fmt.Sprintf(`CASE
				WHEN %s >= 0.75 THEN 'high'
				WHEN %s >= 0.45 THEN 'medium'
				ELSE 'low'
			END`, scoreExpr, scoreExpr)
}

func sourceHealthScoreExpr(
	sourceWeightExpr string,
	freshnessFlagExpr string,
	downFeedbackCountExpr string,
	expiredFeedbackCountExpr string,
) string {
	return fmt.Sprintf(`LEAST(
				1.0,
				GREATEST(
					0.0,
					%s
					- CASE WHEN %s = TRUE THEN 0.20 ELSE 0.0 END
					- COALESCE(%s, 0) * 0.08
					- COALESCE(%s, 0) * 0.12
				)
			)`, sourceWeightExpr, freshnessFlagExpr, downFeedbackCountExpr, expiredFeedbackCountExpr)
}

func sourceHealthLabelCase(scoreExpr string) string {
	return fmt.Sprintf(`CASE
				WHEN %s >= 0.75 THEN 'high'
				WHEN %s >= 0.45 THEN 'medium'
				ELSE 'low'
			END`, scoreExpr, scoreExpr)
}

func sourceHealthStatusCase(
	scoreExpr string,
	freshnessFlagExpr string,
	downFeedbackCountExpr string,
	expiredFeedbackCountExpr string,
) string {
	return fmt.Sprintf(`CASE
				WHEN %s = TRUE OR COALESCE(%s, 0) > 0 THEN '%s'
				WHEN %s < 0.45 OR COALESCE(%s, 0) > 0 THEN '%s'
				ELSE '%s'
			END`,
		freshnessFlagExpr,
		expiredFeedbackCountExpr,
		types.SourceHealthStatusStale,
		scoreExpr,
		downFeedbackCountExpr,
		types.SourceHealthStatusAtRisk,
		types.SourceHealthStatusHealthy,
	)
}

func coverageGapsQuery() string {
	return fmt.Sprintf(`
		WITH answer_stats AS (
			SELECT
				answer.id AS message_id,
				answer.session_id,
				answer.created_at AS answer_created_at,
				COALESCE(question.content, answer.content) AS question,
				COUNT(ae.id) AS source_count,
				COALESCE(AVG(CASE WHEN ae.rerank_score > 0 THEN ae.rerank_score ELSE ae.retrieval_score END), 0) AS evidence_strength_score,
				COALESCE(AVG(
					LEAST(
						1.0,
						GREATEST(
							0.0,
							COALESCE(k.source_weight, 1.0) +
							CASE
								WHEN sf.feedback = 'up' THEN 0.05
								WHEN sf.feedback = 'down' THEN -0.20
								WHEN sf.feedback = 'expired' THEN -0.30
								ELSE 0.0
							END
						)
					)
				), 0) AS source_health_score
			FROM messages answer
			INNER JOIN sessions s ON s.id = answer.session_id AND s.tenant_id = ?
			LEFT JOIN answer_evidence ae ON ae.answer_message_id = answer.id AND ae.tenant_id = ?
			LEFT JOIN knowledges k ON k.id = ae.source_knowledge_id AND k.deleted_at IS NULL
			LEFT JOIN LATERAL (
				SELECT sf.feedback
				FROM source_feedback sf
				WHERE sf.answer_evidence_id = ae.id
				ORDER BY sf.updated_at DESC
				LIMIT 1
			) sf ON TRUE
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
			evidence_strength_score AS confidence_score,
			%s AS confidence_label,
			evidence_strength_score,
			%s AS evidence_strength_label,
			source_health_score,
			%s AS source_health_label,
			source_count,
			answer_created_at
		FROM answer_stats
		WHERE evidence_strength_score < 0.4 OR source_health_score < 0.4 OR source_count = 0
		ORDER BY evidence_strength_score ASC, source_health_score ASC, source_count ASC, answer_created_at DESC
		LIMIT ?
	`, evidenceStrengthLabelCase("evidence_strength_score"), evidenceStrengthLabelCase("evidence_strength_score"), sourceHealthLabelCase("source_health_score"))
}

func staleDocumentsQuery() string {
	scoreExpr := sourceHealthScoreExpr("k.source_weight", "k.freshness_flag", "fs.down_feedback_count", "fs.expired_feedback_count")
	return fmt.Sprintf(`
		WITH feedback_stats AS (
			SELECT
				ae.source_knowledge_id AS knowledge_id,
				COUNT(*) FILTER (WHERE sf.feedback = 'down') AS down_feedback_count,
				COUNT(*) FILTER (WHERE sf.feedback = 'expired') AS expired_feedback_count,
				MAX(sf.created_at) AS last_feedback_at
			FROM answer_evidence ae
			INNER JOIN source_feedback sf ON sf.answer_evidence_id = ae.id
			WHERE ae.tenant_id = ?
			GROUP BY ae.source_knowledge_id
		),
		health_stats AS (
			SELECT
				k.id AS knowledge_id,
				k.title,
				k.source_weight,
				k.freshness_flag,
				COALESCE(fs.down_feedback_count, 0) AS down_feedback_count,
				COALESCE(fs.expired_feedback_count, 0) AS expired_feedback_count,
				fs.last_feedback_at,
				%s AS source_health_score
			FROM knowledges k
			LEFT JOIN feedback_stats fs ON fs.knowledge_id = k.id
			WHERE k.tenant_id = ?
			  AND k.deleted_at IS NULL
		)
		SELECT
			knowledge_id,
			title,
			source_weight,
			freshness_flag,
			down_feedback_count,
			expired_feedback_count,
			source_health_score,
			%s AS source_health_label,
			%s AS health_status,
			last_feedback_at
		FROM health_stats
		WHERE freshness_flag = TRUE
		   OR source_weight < 1.0
		   OR down_feedback_count > 0
		   OR expired_feedback_count > 0
		ORDER BY source_health_score ASC, freshness_flag DESC, expired_feedback_count DESC, down_feedback_count DESC, last_feedback_at DESC NULLS LAST
		LIMIT ?
	`, scoreExpr, sourceHealthLabelCase("source_health_score"), sourceHealthStatusCase("source_health_score", "freshness_flag", "down_feedback_count", "expired_feedback_count"))
}

func citationHeatmapQuery() string {
	scoreExpr := sourceHealthScoreExpr("k.source_weight", "k.freshness_flag", "fs.down_feedback_count", "fs.expired_feedback_count")
	return fmt.Sprintf(`
		WITH feedback_stats AS (
			SELECT
				ae.source_knowledge_id AS knowledge_id,
				COUNT(*) FILTER (WHERE sf.feedback = 'down') AS down_feedback_count,
				COUNT(*) FILTER (WHERE sf.feedback = 'expired') AS expired_feedback_count
			FROM answer_evidence ae
			INNER JOIN source_feedback sf ON sf.answer_evidence_id = ae.id
			WHERE ae.tenant_id = ?
			GROUP BY ae.source_knowledge_id
		)
		SELECT
			k.id AS knowledge_id,
			k.title,
			COUNT(*) FILTER (WHERE dal.access_type = 'cited') AS cited_count,
			COUNT(*) FILTER (WHERE dal.access_type = 'reranked') AS reranked_count,
			COUNT(*) FILTER (WHERE dal.access_type = 'retrieved') AS retrieved_count,
			k.source_weight,
			k.freshness_flag,
			%s AS source_health_score,
			%s AS source_health_label,
			%s AS health_status
		FROM document_access_logs dal
		INNER JOIN knowledges k ON k.id = dal.knowledge_id AND k.tenant_id = dal.tenant_id
		LEFT JOIN feedback_stats fs ON fs.knowledge_id = k.id
		WHERE dal.tenant_id = ?
		  AND dal.created_at >= NOW() - INTERVAL '30 days'
		  AND k.deleted_at IS NULL
		GROUP BY k.id, k.title, k.source_weight, k.freshness_flag, fs.down_feedback_count, fs.expired_feedback_count
		ORDER BY cited_count DESC, source_health_score ASC, reranked_count DESC, retrieved_count DESC, k.title ASC
		LIMIT ?
	`, scoreExpr, sourceHealthLabelCase(scoreExpr), sourceHealthStatusCase(scoreExpr, "k.freshness_flag", "fs.down_feedback_count", "fs.expired_feedback_count"))
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
	err := r.db.WithContext(ctx).Raw(coverageGapsQuery(), tenantID, tenantID, limit).Scan(&rows).Error
	return rows, err
}

func (r *AnalyticsRepository) StaleDocuments(
	ctx context.Context, tenantID uint64, limit int,
) ([]*types.StaleDocument, error) {
	var rows []*types.StaleDocument
	err := r.db.WithContext(ctx).Raw(staleDocumentsQuery(), tenantID, tenantID, limit).Scan(&rows).Error
	return rows, err
}

func (r *AnalyticsRepository) CitationHeatmap(
	ctx context.Context, tenantID uint64, limit int,
) ([]*types.CitationHeat, error) {
	var rows []*types.CitationHeat
	err := r.db.WithContext(ctx).Raw(citationHeatmapQuery(), tenantID, tenantID, limit).Scan(&rows).Error
	return rows, err
}
