package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/gorm"
)

type AnalyticsRepository struct {
	db *gorm.DB
}

type analyticsFilterColumns struct {
	KnowledgeID string
	SessionID   string
	MessageID   string
	TenantID    string
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

func buildAnalyticsFilterClause(
	filter *types.AnalyticsFilter,
	columns analyticsFilterColumns,
) (string, []interface{}) {
	if filter == nil {
		return "", nil
	}

	clauses := make([]string, 0, 3)
	args := make([]interface{}, 0, 4)

	if filter.KnowledgeBaseID != nil && columns.KnowledgeID != "" {
		existsClause := fmt.Sprintf(
			`EXISTS (
				SELECT 1
				FROM knowledges k_filter
				WHERE k_filter.id = %s
				  AND k_filter.deleted_at IS NULL
				  AND k_filter.knowledge_base_id = ?
			)`,
			columns.KnowledgeID,
		)
		if columns.TenantID != "" {
			existsClause = strings.Replace(
				existsClause,
				"AND k_filter.deleted_at IS NULL",
				fmt.Sprintf("AND k_filter.tenant_id = %s\n\t\t\t\t  AND k_filter.deleted_at IS NULL", columns.TenantID),
				1,
			)
		}
		clauses = append(clauses, existsClause)
		args = append(args, *filter.KnowledgeBaseID)
	}

	if filter.SessionID != "" && columns.SessionID != "" {
		clauses = append(clauses, columns.SessionID+" = ?")
		args = append(args, filter.SessionID)
	}

	if filter.MessageID != "" && columns.MessageID != "" {
		clauses = append(clauses, columns.MessageID+" = ?")
		args = append(args, filter.MessageID)
	}

	if len(clauses) == 0 {
		return "", nil
	}
	return " AND " + strings.Join(clauses, " AND "), args
}

func coverageGapsQuery(filterSQL string) string {
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
			  %s
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
	`, filterSQL, evidenceStrengthLabelCase("evidence_strength_score"), evidenceStrengthLabelCase("evidence_strength_score"), sourceHealthLabelCase("source_health_score"))
}

func staleDocumentsQuery(filterSQL string) string {
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
			  %s
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
	`, scoreExpr, filterSQL, sourceHealthLabelCase("source_health_score"), sourceHealthStatusCase("source_health_score", "freshness_flag", "down_feedback_count", "expired_feedback_count"))
}

func citationHeatmapQuery(filterSQL string) string {
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
		  %s
		GROUP BY k.id, k.title, k.source_weight, k.freshness_flag, fs.down_feedback_count, fs.expired_feedback_count
		ORDER BY cited_count DESC, source_health_score ASC, reranked_count DESC, retrieved_count DESC, k.title ASC
		LIMIT ?
	`, scoreExpr, sourceHealthLabelCase(scoreExpr), sourceHealthStatusCase(scoreExpr, "k.freshness_flag", "fs.down_feedback_count", "fs.expired_feedback_count"), filterSQL)
}

func unansweredQuestionsQuery(filterSQL string) string {
	return fmt.Sprintf(`
		WITH user_questions AS (
			SELECT
				user_msg.id AS message_id,
				user_msg.session_id,
				user_msg.content AS question,
				user_msg.created_at AS question_created_at,
				(
					SELECT answer.id
					FROM messages answer
					WHERE answer.session_id = user_msg.session_id
					  AND answer.role = 'assistant'
					  AND answer.deleted_at IS NULL
					  AND answer.created_at >= user_msg.created_at
					ORDER BY answer.created_at ASC
					LIMIT 1
				) AS answer_message_id,
				(
					SELECT answer.created_at
					FROM messages answer
					WHERE answer.session_id = user_msg.session_id
					  AND answer.role = 'assistant'
					  AND answer.deleted_at IS NULL
					  AND answer.created_at >= user_msg.created_at
					ORDER BY answer.created_at ASC
					LIMIT 1
				) AS answer_created_at
			FROM messages user_msg
			INNER JOIN sessions s ON s.id = user_msg.session_id AND s.tenant_id = ?
			WHERE user_msg.role = 'user'
			  AND user_msg.deleted_at IS NULL
			  AND user_msg.created_at >= NOW() - INTERVAL '30 days'
		),
		question_with_sources AS (
			SELECT
				uq.message_id,
				uq.session_id,
				uq.question,
				uq.answer_message_id,
				uq.answer_created_at,
				uq.question_created_at,
				COUNT(ae.id) AS source_count
			FROM user_questions uq
			LEFT JOIN answer_evidence ae
			  ON ae.answer_message_id = uq.answer_message_id
			 AND ae.tenant_id = ?
			WHERE 1 = 1
			  %s
			GROUP BY uq.message_id, uq.session_id, uq.question, uq.answer_message_id, uq.answer_created_at, uq.question_created_at
		)
		SELECT
			qws.message_id,
			qws.session_id,
			qws.question,
			qws.answer_created_at,
			qws.source_count,
			freq.question_freq,
			freq.last_question_at
		FROM question_with_sources qws
		INNER JOIN (
			SELECT
				question,
				COUNT(*) AS question_freq,
				MAX(question_created_at) AS last_question_at
			FROM question_with_sources
			GROUP BY question
		) freq ON freq.question = qws.question
		WHERE qws.answer_message_id IS NULL OR qws.source_count = 0
		ORDER BY freq.question_freq DESC, freq.last_question_at DESC, qws.question_created_at DESC
		LIMIT ?
	`, filterSQL)
}

func (r *AnalyticsRepository) HotQuestions(
	ctx context.Context, tenantID uint64, limit int, filter *types.AnalyticsFilter,
) ([]*types.HotQuestion, error) {
	filterSQL, filterArgs := buildAnalyticsFilterClause(filter, analyticsFilterColumns{
		KnowledgeID: "dal.knowledge_id",
		SessionID:   "answer.session_id",
		MessageID:   "answer.id",
		TenantID:    "dal.tenant_id",
	})
	var rows []*types.HotQuestion
	query := fmt.Sprintf(`
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
		  %s
		GROUP BY answer.id, answer.session_id, question.content, answer.content
		ORDER BY retrieved_count DESC, reranked_count DESC, cited_count DESC, last_access_at DESC
		LIMIT ?
	`, filterSQL)
	args := []interface{}{tenantID, tenantID}
	args = append(args, filterArgs...)
	args = append(args, limit)
	err := r.db.WithContext(ctx).Raw(query, args...).Scan(&rows).Error
	return rows, err
}

func (r *AnalyticsRepository) CoverageGaps(
	ctx context.Context, tenantID uint64, limit int, filter *types.AnalyticsFilter,
) ([]*types.CoverageGap, error) {
	filterSQL, filterArgs := buildAnalyticsFilterClause(filter, analyticsFilterColumns{
		KnowledgeID: "ae.source_knowledge_id",
		SessionID:   "answer.session_id",
		MessageID:   "answer.id",
		TenantID:    "s.tenant_id",
	})
	var rows []*types.CoverageGap
	args := []interface{}{tenantID, tenantID}
	args = append(args, filterArgs...)
	args = append(args, limit)
	err := r.db.WithContext(ctx).Raw(coverageGapsQuery(filterSQL), args...).Scan(&rows).Error
	return rows, err
}

func (r *AnalyticsRepository) StaleDocuments(
	ctx context.Context, tenantID uint64, limit int, filter *types.AnalyticsFilter,
) ([]*types.StaleDocument, error) {
	filterSQL, filterArgs := buildAnalyticsFilterClause(filter, analyticsFilterColumns{
		KnowledgeID: "k.id",
		TenantID:    "k.tenant_id",
	})
	var rows []*types.StaleDocument
	args := []interface{}{tenantID, tenantID}
	args = append(args, filterArgs...)
	args = append(args, limit)
	err := r.db.WithContext(ctx).Raw(staleDocumentsQuery(filterSQL), args...).Scan(&rows).Error
	return rows, err
}

func (r *AnalyticsRepository) CitationHeatmap(
	ctx context.Context, tenantID uint64, limit int, filter *types.AnalyticsFilter,
) ([]*types.CitationHeat, error) {
	filterSQL, filterArgs := buildAnalyticsFilterClause(filter, analyticsFilterColumns{
		KnowledgeID: "k.id",
		SessionID:   "dal.session_id",
		MessageID:   "dal.message_id",
		TenantID:    "dal.tenant_id",
	})
	var rows []*types.CitationHeat
	args := []interface{}{tenantID, tenantID}
	args = append(args, filterArgs...)
	args = append(args, limit)
	err := r.db.WithContext(ctx).Raw(citationHeatmapQuery(filterSQL), args...).Scan(&rows).Error
	return rows, err
}

func (r *AnalyticsRepository) UnansweredQuestions(
	ctx context.Context, tenantID uint64, limit int, filter *types.AnalyticsFilter,
) ([]*types.UnansweredQuestion, error) {
	filterSQL, filterArgs := buildAnalyticsFilterClause(filter, analyticsFilterColumns{
		KnowledgeID: "ae.source_knowledge_id",
		SessionID:   "uq.session_id",
		MessageID:   "uq.message_id",
		TenantID:    "",
	})
	var rows []*types.UnansweredQuestion
	args := []interface{}{tenantID, tenantID}
	args = append(args, filterArgs...)
	args = append(args, limit)
	err := r.db.WithContext(ctx).Raw(unansweredQuestionsQuery(filterSQL), args...).Scan(&rows).Error
	return rows, err
}
