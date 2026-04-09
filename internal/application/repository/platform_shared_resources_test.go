package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupSharedResourceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&types.WebSearchProviderEntity{}, &types.MCPService{}))
	return db
}

func TestWebSearchProviderRepository_ListAndDefaultIncludePlatform(t *testing.T) {
	db := setupSharedResourceTestDB(t)
	repo := NewWebSearchProviderRepository(db)
	ctx := context.Background()

	baseTime := time.Now().UTC()
	tenantID := uint64(42)

	platformProvider := &types.WebSearchProviderEntity{
		ID:         "platform-provider",
		TenantID:   10000,
		Name:       "Platform Brave",
		Provider:   types.WebSearchProviderTypeBrave,
		IsPlatform: true,
		IsDefault:  true,
		CreatedAt:  baseTime,
		UpdatedAt:  baseTime,
	}
	tenantProvider := &types.WebSearchProviderEntity{
		ID:        "tenant-provider",
		TenantID:  tenantID,
		Name:      "Tenant Bing",
		Provider:  types.WebSearchProviderTypeBing,
		IsDefault: true,
		CreatedAt: baseTime.Add(time.Minute),
		UpdatedAt: baseTime.Add(time.Minute),
	}
	require.NoError(t, db.Create(platformProvider).Error)
	require.NoError(t, db.Create(tenantProvider).Error)

	list, err := repo.List(ctx, tenantID)
	require.NoError(t, err)
	require.Len(t, list, 2)
	require.Equal(t, "tenant-provider", list[0].ID)
	require.Equal(t, "platform-provider", list[1].ID)

	def, err := repo.GetDefault(ctx, tenantID)
	require.NoError(t, err)
	require.NotNil(t, def)
	require.Equal(t, "tenant-provider", def.ID)
}

func TestMCPServiceRepository_ListIncludesPlatformAndBuiltin(t *testing.T) {
	db := setupSharedResourceTestDB(t)
	repo := NewMCPServiceRepository(db)
	ctx := context.Background()

	tenantID := uint64(42)
	now := time.Now().UTC()

	tenantService := &types.MCPService{
		ID:            "tenant-svc",
		TenantID:      tenantID,
		Name:          "Tenant MCP",
		Enabled:       true,
		TransportType: types.MCPTransportSSE,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	platformService := &types.MCPService{
		ID:            "platform-svc",
		TenantID:      10000,
		Name:          "Platform MCP",
		Enabled:       true,
		TransportType: types.MCPTransportSSE,
		IsPlatform:    true,
		CreatedAt:     now.Add(time.Minute),
		UpdatedAt:     now.Add(time.Minute),
	}
	builtinService := &types.MCPService{
		ID:            "builtin-svc",
		TenantID:      0,
		Name:          "Builtin MCP",
		Enabled:       true,
		TransportType: types.MCPTransportSSE,
		IsBuiltin:     true,
		CreatedAt:     now.Add(2 * time.Minute),
		UpdatedAt:     now.Add(2 * time.Minute),
	}
	require.NoError(t, db.Create(tenantService).Error)
	require.NoError(t, db.Create(platformService).Error)
	require.NoError(t, db.Create(builtinService).Error)

	list, err := repo.List(ctx, tenantID)
	require.NoError(t, err)
	require.Len(t, list, 3)

	ids := map[string]bool{}
	for _, item := range list {
		ids[item.ID] = true
	}
	require.True(t, ids["tenant-svc"])
	require.True(t, ids["platform-svc"])
	require.True(t, ids["builtin-svc"])
}
