package repository

import (
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

func TestCoverageGapsQuery_UsesDualDimensionAliases(t *testing.T) {
	query := coverageGapsQuery()

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
	query := staleDocumentsQuery()

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
	query := citationHeatmapQuery()

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
