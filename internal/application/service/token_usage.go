package service

import (
	"context"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// TokenUsageService handles token consumption tracking and quota enforcement.
type TokenUsageService struct {
	tenantRepo interfaces.TenantRepository
}

// NewTokenUsageService creates a new TokenUsageService.
func NewTokenUsageService(tenantRepo interfaces.TenantRepository) *TokenUsageService {
	return &TokenUsageService{tenantRepo: tenantRepo}
}

// CheckQuota returns an error if the tenant has exceeded their token quota.
// A quota of 0 means unlimited.
func (s *TokenUsageService) CheckQuota(ctx context.Context, tenant *types.Tenant) error {
	if tenant.TokenQuota <= 0 {
		return nil
	}
	if tenant.QuotaResetAt != nil && tenant.QuotaResetAt.Before(timeNow()) {
		return nil
	}
	if tenant.TokenUsed >= tenant.TokenQuota {
		return types.NewTokenQuotaExceededError()
	}
	return nil
}

// RecordUsage records token consumption for a tenant.
func (s *TokenUsageService) RecordUsage(ctx context.Context, tenantID uint64, tokensUsed int64) {
	if tokensUsed <= 0 {
		return
	}
	if err := s.tenantRepo.AdjustTokenUsed(ctx, tenantID, tokensUsed); err != nil {
		logger.Errorf(ctx, "Failed to record token usage for tenant %d: %v", tenantID, err)
	}
}

var timeNow = func() time.Time { return time.Now() }
