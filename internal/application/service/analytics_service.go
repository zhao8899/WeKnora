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

func (s *AnalyticsService) HotQuestions(ctx context.Context) ([]*types.HotQuestion, error) {
	return s.repo.HotQuestions(ctx, types.MustTenantIDFromContext(ctx), defaultAnalyticsLimit)
}

func (s *AnalyticsService) CoverageGaps(ctx context.Context) ([]*types.CoverageGap, error) {
	return s.repo.CoverageGaps(ctx, types.MustTenantIDFromContext(ctx), defaultAnalyticsLimit)
}

func (s *AnalyticsService) StaleDocuments(ctx context.Context) ([]*types.StaleDocument, error) {
	return s.repo.StaleDocuments(ctx, types.MustTenantIDFromContext(ctx), defaultAnalyticsLimit)
}

func (s *AnalyticsService) CitationHeatmap(ctx context.Context) ([]*types.CitationHeat, error) {
	return s.repo.CitationHeatmap(ctx, types.MustTenantIDFromContext(ctx), defaultAnalyticsLimit)
}
