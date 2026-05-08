package service

import (
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

func TestResolveAnalyticsLimit_Default(t *testing.T) {
	limit := resolveAnalyticsLimit(nil)
	if limit != defaultAnalyticsLimit {
		t.Fatalf("expected default limit %d, got %d", defaultAnalyticsLimit, limit)
	}
}

func TestResolveAnalyticsLimit_FromFilter(t *testing.T) {
	custom := 35
	limit := resolveAnalyticsLimit(&types.AnalyticsFilter{Limit: &custom})
	if limit != custom {
		t.Fatalf("expected custom limit %d, got %d", custom, limit)
	}
}

func TestResolveAnalyticsLimit_RejectsNonPositive(t *testing.T) {
	zero := 0
	limit := resolveAnalyticsLimit(&types.AnalyticsFilter{Limit: &zero})
	if limit != defaultAnalyticsLimit {
		t.Fatalf("expected fallback limit %d, got %d", defaultAnalyticsLimit, limit)
	}
}
