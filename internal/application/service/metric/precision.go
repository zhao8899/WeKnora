package metric

import (
	"github.com/Tencent/WeKnora/internal/types"
)

// PrecisionMetric calculates precision for retrieval evaluation
type PrecisionMetric struct{}

// NewPrecisionMetric creates a new PrecisionMetric instance
func NewPrecisionMetric() *PrecisionMetric {
	return &PrecisionMetric{}
}

// Compute calculates the precision score
func (r *PrecisionMetric) Compute(metricInput *types.MetricInput) float64 {
	gts := metricInput.RetrievalGT
	ids := metricInput.RetrievalIDs

	if len(gts) == 0 || len(ids) == 0 {
		return 0.0
	}

	gtSets := SliceMap(gts, ToSet)
	total := 0.0
	for _, gtSet := range gtSets {
		total += float64(Hit(ids, gtSet)) / float64(len(ids))
	}

	return total / float64(len(gtSets))
}
