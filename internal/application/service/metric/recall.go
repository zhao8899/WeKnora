package metric

import (
	"github.com/Tencent/WeKnora/internal/types"
)

// RecallMetric calculates recall for retrieval evaluation
type RecallMetric struct{}

// NewRecallMetric creates a new RecallMetric instance
func NewRecallMetric() *RecallMetric {
	return &RecallMetric{}
}

// Compute calculates the recall score
func (r *RecallMetric) Compute(metricInput *types.MetricInput) float64 {
	gts := metricInput.RetrievalGT
	ids := metricInput.RetrievalIDs

	gtSets := SliceMap(gts, ToSet)
	if len(gtSets) == 0 {
		return 0.0
	}

	total := 0.0
	for _, gtSet := range gtSets {
		if len(gtSet) == 0 {
			continue
		}
		total += float64(Hit(ids, gtSet)) / float64(len(gtSet))
	}

	return total / float64(len(gtSets))
}
