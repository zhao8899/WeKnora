package repository

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestModelRepositoryListIncludesPlatformModels(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil && strings.Contains(err.Error(), "go-sqlite3 requires cgo") {
		t.Skip("sqlite test skipped because go-sqlite3 is unavailable with CGO_ENABLED=0")
	}
	require.NoError(t, err)
	err = db.AutoMigrate(&types.Model{})
	if err != nil && strings.Contains(err.Error(), "go-sqlite3 requires cgo") {
		t.Skip("sqlite test skipped because go-sqlite3 is unavailable with CGO_ENABLED=0")
	}
	require.NoError(t, err)

	repo := NewModelRepository(db)
	ctx := context.Background()
	now := time.Now()
	models := []*types.Model{
		{
			ID:        "tenant-model",
			TenantID:  2,
			Name:      "tenant chat",
			Type:      types.ModelTypeKnowledgeQA,
			Source:    types.ModelSourceRemote,
			Status:    types.ModelStatusActive,
			CreatedAt: now.Add(2 * time.Minute),
		},
		{
			ID:        "platform-model",
			TenantID:  1,
			Name:      "platform chat",
			Type:      types.ModelTypeKnowledgeQA,
			Source:    types.ModelSourceRemote,
			IsBuiltin: true,
			IsDefault: true,
			Status:    types.ModelStatusActive,
			CreatedAt: now,
		},
		{
			ID:        "other-tenant-model",
			TenantID:  3,
			Name:      "other tenant chat",
			Type:      types.ModelTypeKnowledgeQA,
			Source:    types.ModelSourceRemote,
			Status:    types.ModelStatusActive,
			CreatedAt: now.Add(time.Minute),
		},
	}
	for _, model := range models {
		require.NoError(t, repo.Create(ctx, model))
	}

	got, err := repo.List(ctx, 2, types.ModelTypeKnowledgeQA, "")
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, "platform-model", got[0].ID)
	require.True(t, got[0].IsBuiltin)
	require.True(t, got[0].IsDefault)
	require.Equal(t, "tenant-model", got[1].ID)
}
