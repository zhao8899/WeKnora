package service

import (
	"context"
	"testing"

	infra_web_search "github.com/Tencent/WeKnora/internal/infrastructure/web_search"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

type stubWebSearchProviderRepo struct {
	created       *types.WebSearchProviderEntity
	defaultEntity *types.WebSearchProviderEntity
	clearedTenant uint64
	clearedScope  bool
	gotDefaultFor uint64
}

func (s *stubWebSearchProviderRepo) Create(_ context.Context, provider *types.WebSearchProviderEntity) error {
	s.created = provider
	return nil
}

func (s *stubWebSearchProviderRepo) GetByID(_ context.Context, _ uint64, _ string) (*types.WebSearchProviderEntity, error) {
	return nil, nil
}

func (s *stubWebSearchProviderRepo) GetDefault(_ context.Context, tenantID uint64) (*types.WebSearchProviderEntity, error) {
	s.gotDefaultFor = tenantID
	return s.defaultEntity, nil
}

func (s *stubWebSearchProviderRepo) GetPlatformDefault(_ context.Context) (*types.WebSearchProviderEntity, error) {
	return s.defaultEntity, nil
}

func (s *stubWebSearchProviderRepo) List(_ context.Context, _ uint64) ([]*types.WebSearchProviderEntity, error) {
	return nil, nil
}

func (s *stubWebSearchProviderRepo) Update(_ context.Context, _ *types.WebSearchProviderEntity) error {
	return nil
}

func (s *stubWebSearchProviderRepo) Delete(_ context.Context, _ uint64, _ string) error {
	return nil
}

func (s *stubWebSearchProviderRepo) ClearDefault(_ context.Context, tenantID uint64, isPlatform bool, _ string) error {
	s.clearedTenant = tenantID
	s.clearedScope = isPlatform
	return nil
}

type stubSearchProvider struct{}

func (s *stubSearchProvider) Name() string { return "stub" }

func (s *stubSearchProvider) Search(_ context.Context, query string, _ int, _ bool) ([]*types.WebSearchResult, error) {
	return []*types.WebSearchResult{{Title: query, URL: "https://example.com"}}, nil
}

var _ interfaces.WebSearchProviderRepository = (*stubWebSearchProviderRepo)(nil)

func TestWebSearchProviderServiceCreateProviderAllowsSerpAPI(t *testing.T) {
	repo := &stubWebSearchProviderRepo{}
	svc := NewWebSearchProviderService(repo)

	provider := &types.WebSearchProviderEntity{
		TenantID:   1,
		Name:       "SerpAPI Default",
		Provider:   types.WebSearchProviderTypeSerpAPI,
		Parameters: types.WebSearchProviderParameters{APIKey: "test-key"},
		IsDefault:  true,
	}

	if err := svc.CreateProvider(context.Background(), provider); err != nil {
		t.Fatalf("CreateProvider returned error: %v", err)
	}
	if repo.created == nil {
		t.Fatal("expected provider to be persisted")
	}
	if repo.clearedTenant != 1 || repo.clearedScope {
		t.Fatalf("expected tenant default scope to be cleared, got tenant=%d platform=%t", repo.clearedTenant, repo.clearedScope)
	}
}

func TestWebSearchProviderServiceUpdateProviderNormalizesLegacySerpAPI(t *testing.T) {
	repo := &stubWebSearchProviderRepo{}
	svc := NewWebSearchProviderService(repo)

	provider := &types.WebSearchProviderEntity{
		ID:         "legacy-serpapi",
		TenantID:   1,
		Name:       "Legacy SerpAPI",
		Provider:   types.WebSearchProviderType("SerpAPI"),
		Parameters: types.WebSearchProviderParameters{APIKey: "test-key"},
	}

	if err := svc.UpdateProvider(context.Background(), provider); err != nil {
		t.Fatalf("UpdateProvider returned error: %v", err)
	}
	if provider.Provider != types.WebSearchProviderTypeSerpAPI {
		t.Fatalf("expected provider to be normalized to %q, got %q", types.WebSearchProviderTypeSerpAPI, provider.Provider)
	}
}

func TestWebSearchServiceSearchFallsBackToPlatformDefault(t *testing.T) {
	registry := infra_web_search.NewRegistry()
	registry.Register(string(types.WebSearchProviderTypeDuckDuckGo), func(params types.WebSearchProviderParameters) (interfaces.WebSearchProvider, error) {
		return &stubSearchProvider{}, nil
	})

	repo := &stubWebSearchProviderRepo{
		defaultEntity: &types.WebSearchProviderEntity{
			ID:         "platform-default",
			TenantID:   100,
			Name:       "Platform DuckDuckGo",
			Provider:   types.WebSearchProviderTypeDuckDuckGo,
			IsDefault:  true,
			IsPlatform: true,
		},
	}

	svc, err := NewWebSearchService(nil, registry, repo)
	if err != nil {
		t.Fatalf("NewWebSearchService returned error: %v", err)
	}

	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(2))
	results, err := svc.Search(ctx, "", &types.WebSearchConfig{MaxResults: 1}, "fallback-query")
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if repo.gotDefaultFor != 2 {
		t.Fatalf("expected default lookup for tenant 2, got %d", repo.gotDefaultFor)
	}
	if len(results) != 1 || results[0].Title != "fallback-query" {
		t.Fatalf("unexpected search results: %#v", results)
	}
}
