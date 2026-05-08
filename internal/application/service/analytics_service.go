package service

import (
	"context"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/types"
)

const (
	defaultAnalyticsLimit = 20
)

type AnalyticsService struct {
	repo *repository.AnalyticsRepository
}

func NewAnalyticsService(repo *repository.AnalyticsRepository) *AnalyticsService {
	return &AnalyticsService{repo: repo}
}

func (s *AnalyticsService) HotQuestions(ctx context.Context, filter *types.AnalyticsFilter) ([]*types.HotQuestion, error) {
	return s.repo.HotQuestions(ctx, types.MustTenantIDFromContext(ctx), resolveAnalyticsLimit(filter), filter)
}

func (s *AnalyticsService) CoverageGaps(ctx context.Context, filter *types.AnalyticsFilter) ([]*types.CoverageGap, error) {
	return s.repo.CoverageGaps(ctx, types.MustTenantIDFromContext(ctx), resolveAnalyticsLimit(filter), filter)
}

func (s *AnalyticsService) StaleDocuments(ctx context.Context, filter *types.AnalyticsFilter) ([]*types.StaleDocument, error) {
	return s.repo.StaleDocuments(ctx, types.MustTenantIDFromContext(ctx), resolveAnalyticsLimit(filter), filter)
}

func (s *AnalyticsService) CitationHeatmap(ctx context.Context, filter *types.AnalyticsFilter) ([]*types.CitationHeat, error) {
	return s.repo.CitationHeatmap(ctx, types.MustTenantIDFromContext(ctx), resolveAnalyticsLimit(filter), filter)
}

func (s *AnalyticsService) UnansweredQuestions(
	ctx context.Context,
	filter *types.AnalyticsFilter,
) ([]*types.UnansweredQuestion, error) {
	return s.repo.UnansweredQuestions(ctx, types.MustTenantIDFromContext(ctx), resolveAnalyticsLimit(filter), filter)
}

func resolveAnalyticsLimit(filter *types.AnalyticsFilter) int {
	if filter != nil && filter.Limit != nil && *filter.Limit > 0 {
		return *filter.Limit
	}
	return defaultAnalyticsLimit
}
