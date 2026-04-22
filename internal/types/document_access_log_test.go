package types

import "testing"

func TestSourceHealthLabel(t *testing.T) {
	cases := []struct {
		score float64
		want  string
	}{
		{0.90, "high"},
		{0.75, "high"},
		{0.60, "medium"},
		{0.45, "medium"},
		{0.44, "low"},
		{0.10, "low"},
	}

	for _, tc := range cases {
		if got := SourceHealthLabel(tc.score); got != tc.want {
			t.Fatalf("SourceHealthLabel(%v) = %q, want %q", tc.score, got, tc.want)
		}
	}
}

func TestSourceHealthStatus(t *testing.T) {
	cases := []struct {
		name                 string
		score                float64
		freshnessFlag        bool
		downFeedbackCount    int64
		expiredFeedbackCount int64
		want                 string
	}{
		{"healthy", 0.80, false, 0, 0, SourceHealthStatusHealthy},
		{"at risk by score", 0.30, false, 0, 0, SourceHealthStatusAtRisk},
		{"at risk by down feedback", 0.80, false, 1, 0, SourceHealthStatusAtRisk},
		{"stale by freshness", 0.80, true, 0, 0, SourceHealthStatusStale},
		{"stale by expired feedback", 0.80, false, 0, 1, SourceHealthStatusStale},
	}

	for _, tc := range cases {
		if got := SourceHealthStatus(tc.score, tc.freshnessFlag, tc.downFeedbackCount, tc.expiredFeedbackCount); got != tc.want {
			t.Fatalf("%s: SourceHealthStatus(...) = %q, want %q", tc.name, got, tc.want)
		}
	}
}
