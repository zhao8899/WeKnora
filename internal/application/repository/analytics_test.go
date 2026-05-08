package repository

import (
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

func TestCoverageGapsQuery_UsesDualDimensionAliases(t *testing.T) {
	query := coverageGapsQuery("")

	required := []string{
		"evidence_strength_score AS confidence_score",
		"AS evidence_strength_label",
		"source_health_score",
		"AS source_health_label",
		"WHERE evidence_strength_score < 0.4 OR source_health_score < 0.4 OR source_count = 0",
	}

	for _, fragment := range required {
		if !strings.Contains(query, fragment) {
			t.Fatalf("coverageGapsQuery missing fragment %q\nquery:\n%s", fragment, query)
		}
	}
}

func TestStaleDocumentsQuery_UsesUnifiedSourceHealthStatus(t *testing.T) {
	query := staleDocumentsQuery("")

	required := []string{
		"expired_feedback_count",
		"AS source_health_score",
		"AS source_health_label",
		"AS health_status",
		types.SourceHealthStatusStale,
		types.SourceHealthStatusAtRisk,
		types.SourceHealthStatusHealthy,
	}

	for _, fragment := range required {
		if !strings.Contains(query, fragment) {
			t.Fatalf("staleDocumentsQuery missing fragment %q\nquery:\n%s", fragment, query)
		}
	}
}

func TestCitationHeatmapQuery_UsesSharedHealthExpressions(t *testing.T) {
	query := citationHeatmapQuery("")

	required := []string{
		"COUNT(*) FILTER (WHERE dal.access_type = 'cited') AS cited_count",
		"AS source_health_score",
		"AS source_health_label",
		"AS health_status",
		"ORDER BY cited_count DESC, source_health_score ASC",
		types.SourceHealthStatusStale,
		types.SourceHealthStatusAtRisk,
		types.SourceHealthStatusHealthy,
	}

	for _, fragment := range required {
		if !strings.Contains(query, fragment) {
			t.Fatalf("citationHeatmapQuery missing fragment %q\nquery:\n%s", fragment, query)
		}
	}
}

func TestUnansweredQuestionsQuery_IncludesOperationalSortAndFields(t *testing.T) {
	query := unansweredQuestionsQuery("")

	required := []string{
		"qws.message_id",
		"qws.session_id",
		"qws.question",
		"qws.answer_created_at",
		"qws.source_count",
		"freq.question_freq",
		"freq.last_question_at",
		"WHERE qws.answer_message_id IS NULL OR qws.source_count = 0",
		"ORDER BY freq.question_freq DESC, freq.last_question_at DESC",
	}

	for _, fragment := range required {
		if !strings.Contains(query, fragment) {
			t.Fatalf("unansweredQuestionsQuery missing fragment %q\nquery:\n%s", fragment, query)
		}
	}
}

func TestUnansweredQuestionsQuery_AcceptsSessionAndMessageFilters(t *testing.T) {
	filter := &types.AnalyticsFilter{
		SessionID: "session-1",
		MessageID: "message-1",
	}
	filterSQL, args := buildAnalyticsFilterClause(filter, analyticsFilterColumns{
		SessionID: "uq.session_id",
		MessageID: "uq.message_id",
	})
	query := unansweredQuestionsQuery(filterSQL)

	required := []string{
		"uq.session_id = ?",
		"uq.message_id = ?",
		"LIMIT ?",
	}
	for _, fragment := range required {
		if !strings.Contains(query, fragment) {
			t.Fatalf("unansweredQuestionsQuery missing filter fragment %q\nquery:\n%s", fragment, query)
		}
	}
	if len(args) != 2 {
		t.Fatalf("expected 2 filter args, got %d (%v)", len(args), args)
	}
	if args[0] != "session-1" || args[1] != "message-1" {
		t.Fatalf("unexpected filter args order/content: %#v", args)
	}
}

func TestSourceHealthSQLHelpers_StayAlignedWithSharedThresholds(t *testing.T) {
	labelCase := sourceHealthLabelCase("score_expr")
	if !strings.Contains(labelCase, "score_expr >= 0.75 THEN 'high'") {
		t.Fatalf("sourceHealthLabelCase should use high threshold, got:\n%s", labelCase)
	}
	if !strings.Contains(labelCase, "score_expr >= 0.45 THEN 'medium'") {
		t.Fatalf("sourceHealthLabelCase should use medium threshold, got:\n%s", labelCase)
	}

	statusCase := sourceHealthStatusCase("score_expr", "freshness_flag", "down_count", "expired_count")
	required := []string{
		"freshness_flag = TRUE OR COALESCE(expired_count, 0) > 0",
		"score_expr < 0.45 OR COALESCE(down_count, 0) > 0",
		types.SourceHealthStatusStale,
		types.SourceHealthStatusAtRisk,
		types.SourceHealthStatusHealthy,
	}

	for _, fragment := range required {
		if !strings.Contains(statusCase, fragment) {
			t.Fatalf("sourceHealthStatusCase missing fragment %q\ncase:\n%s", fragment, statusCase)
		}
	}
}

func TestBuildAnalyticsFilterClause_DefaultIsNoop(t *testing.T) {
	clause, args := buildAnalyticsFilterClause(nil, analyticsFilterColumns{})
	if clause != "" {
		t.Fatalf("expected empty clause for nil filter, got %q", clause)
	}
	if len(args) != 0 {
		t.Fatalf("expected no args for nil filter, got %d", len(args))
	}
}

func TestBuildAnalyticsFilterClause_UsesPlaceholdersForAllSupportedFilters(t *testing.T) {
	kbID := "kb-123"
	filter := &types.AnalyticsFilter{
		KnowledgeBaseID: &kbID,
		SessionID:       "session-1",
		MessageID:       "message-1",
	}
	clause, args := buildAnalyticsFilterClause(filter, analyticsFilterColumns{
		KnowledgeID: "dal.knowledge_id",
		SessionID:   "answer.session_id",
		MessageID:   "answer.id",
		TenantID:    "dal.tenant_id",
	})

	required := []string{
		"k_filter.knowledge_base_id = ?",
		"answer.session_id = ?",
		"answer.id = ?",
	}
	for _, fragment := range required {
		if !strings.Contains(clause, fragment) {
			t.Fatalf("missing filter fragment %q in clause:\n%s", fragment, clause)
		}
	}
	if strings.Contains(clause, "session-1") || strings.Contains(clause, "message-1") || strings.Contains(clause, "kb-123") {
		t.Fatalf("clause should not interpolate user input directly:\n%s", clause)
	}
	if len(args) != 3 {
		t.Fatalf("expected 3 args, got %d (%v)", len(args), args)
	}
	if args[0] != kbID || args[1] != "session-1" || args[2] != "message-1" {
		t.Fatalf("unexpected args order/content: %#v", args)
	}
}

func TestBuildAnalyticsFilterClause_SkipsUnsupportedColumns(t *testing.T) {
	kbID := "kb-5"
	filter := &types.AnalyticsFilter{
		KnowledgeBaseID: &kbID,
		SessionID:       "session-2",
		MessageID:       "message-2",
	}
	clause, args := buildAnalyticsFilterClause(filter, analyticsFilterColumns{
		KnowledgeID: "k.id",
		TenantID:    "k.tenant_id",
	})

	if strings.Contains(clause, "session_id") || strings.Contains(clause, "message_id") {
		t.Fatalf("session/message conditions should be skipped when columns are not provided:\n%s", clause)
	}
	if !strings.Contains(clause, "k_filter.knowledge_base_id = ?") {
		t.Fatalf("knowledge base filter should still exist:\n%s", clause)
	}
	if len(args) != 1 || args[0] != kbID {
		t.Fatalf("unexpected args: %#v", args)
	}
}

func TestBuildAnalyticsFilterClause_AllowsKnowledgeFilterWithoutTenantColumn(t *testing.T) {
	kbID := "kb-9"
	filter := &types.AnalyticsFilter{
		KnowledgeBaseID: &kbID,
	}

	clause, args := buildAnalyticsFilterClause(filter, analyticsFilterColumns{
		KnowledgeID: "ae.source_knowledge_id",
	})

	if !strings.Contains(clause, "k_filter.id = ae.source_knowledge_id") {
		t.Fatalf("knowledge filter should target provided knowledge column:\n%s", clause)
	}
	if strings.Contains(clause, "k_filter.tenant_id") {
		t.Fatalf("tenant predicate should be omitted when tenant column is unavailable:\n%s", clause)
	}
	if len(args) != 1 || args[0] != kbID {
		t.Fatalf("unexpected args: %#v", args)
	}
}
