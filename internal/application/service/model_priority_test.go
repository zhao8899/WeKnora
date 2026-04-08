package service

import (
	"context"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

type fakeModelRepo struct {
	models       []*types.Model
	updatedModel *types.Model
	deletedID    string
}

func (r *fakeModelRepo) Create(ctx context.Context, model *types.Model) error {
	return nil
}

func (r *fakeModelRepo) GetByID(ctx context.Context, tenantID uint64, id string) (*types.Model, error) {
	for _, model := range r.models {
		if model != nil && model.ID == id {
			return model, nil
		}
	}
	return nil, nil
}

func (r *fakeModelRepo) List(
	ctx context.Context,
	tenantID uint64,
	modelType types.ModelType,
	source types.ModelSource,
) ([]*types.Model, error) {
	return r.models, nil
}

func (r *fakeModelRepo) Update(ctx context.Context, model *types.Model) error {
	r.updatedModel = model
	return nil
}

func (r *fakeModelRepo) Delete(ctx context.Context, tenantID uint64, id string) error {
	r.deletedID = id
	return nil
}

func (r *fakeModelRepo) ClearDefaultByType(ctx context.Context, tenantID uint64, modelType types.ModelType, excludeID string) error {
	return nil
}

func (r *fakeModelRepo) ListPlatformDefaults(ctx context.Context, modelType types.ModelType) ([]*types.Model, error) {
	return nil, nil
}

func TestResolvePreferredModel(t *testing.T) {
	tenantID := uint64(42)
	baseTime := time.Date(2026, 4, 7, 10, 0, 0, 0, time.UTC)
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, tenantID)

	tests := []struct {
		name   string
		models []*types.Model
		wantID string
	}{
		{
			name: "tenant model wins over platform model",
			models: []*types.Model{
				{ID: "platform", Type: types.ModelTypeKnowledgeQA, IsPlatform: true, Status: types.ModelStatusActive, CreatedAt: baseTime.Add(time.Minute)},
				{ID: "tenant", TenantID: tenantID, Type: types.ModelTypeKnowledgeQA, Status: types.ModelStatusActive, CreatedAt: baseTime.Add(2 * time.Minute)},
			},
			wantID: "tenant",
		},
		{
			name: "platform wins when tenant model is not active",
			models: []*types.Model{
				{ID: "tenant-downloading", TenantID: tenantID, Type: types.ModelTypeKnowledgeQA, Status: types.ModelStatusDownloading, CreatedAt: baseTime},
				{ID: "platform", Type: types.ModelTypeKnowledgeQA, IsPlatform: true, Status: types.ModelStatusActive, CreatedAt: baseTime.Add(time.Minute)},
			},
			wantID: "platform",
		},
		{
			name: "platform is fallback when tenant model is absent",
			models: []*types.Model{
				{ID: "platform", Type: types.ModelTypeKnowledgeQA, IsPlatform: true, Status: types.ModelStatusActive, CreatedAt: baseTime},
			},
			wantID: "platform",
		},
		{
			name: "prefer default within same governance layer",
			models: []*types.Model{
				{ID: "tenant-older", TenantID: tenantID, Type: types.ModelTypeKnowledgeQA, Status: types.ModelStatusActive, CreatedAt: baseTime},
				{ID: "tenant-default", TenantID: tenantID, Type: types.ModelTypeKnowledgeQA, Status: types.ModelStatusActive, IsDefault: true, CreatedAt: baseTime.Add(time.Minute)},
				{ID: "platform", Type: types.ModelTypeKnowledgeQA, IsPlatform: true, Status: types.ModelStatusActive, CreatedAt: baseTime.Add(2 * time.Minute)},
			},
			wantID: "tenant-default",
		},
		{
			name: "returns nil when no active model exists",
			models: []*types.Model{
				{ID: "platform-failed", Type: types.ModelTypeKnowledgeQA, IsPlatform: true, Status: types.ModelStatusDownloadFailed, CreatedAt: baseTime},
			},
			wantID: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &modelService{repo: &fakeModelRepo{models: tt.models}}
			got, err := svc.ResolvePreferredModel(ctx, types.ModelTypeKnowledgeQA)
			if err != nil {
				t.Fatalf("ResolvePreferredModel() error = %v", err)
			}
			if tt.wantID == "" {
				if got != nil {
					t.Fatalf("ResolvePreferredModel() = %s, want nil", got.ID)
				}
				return
			}
			if got == nil {
				t.Fatalf("ResolvePreferredModel() = nil, want %s", tt.wantID)
			}
			if got.ID != tt.wantID {
				t.Fatalf("ResolvePreferredModel() = %s, want %s", got.ID, tt.wantID)
			}
		})
	}
}

func TestSharedModelMutationRequiresSuperAdmin(t *testing.T) {
	tenantID := uint64(42)
	sharedModel := &types.Model{
		ID:         "platform-model",
		TenantID:   tenantID,
		Type:       types.ModelTypeKnowledgeQA,
		IsPlatform: true,
		Status:     types.ModelStatusActive,
	}

	t.Run("update shared model denied for tenant admin", func(t *testing.T) {
		repo := &fakeModelRepo{models: []*types.Model{sharedModel}}
		svc := &modelService{repo: repo}
		ctx := context.WithValue(context.Background(), types.TenantIDContextKey, tenantID)

		err := svc.UpdateModel(ctx, &types.Model{ID: sharedModel.ID, TenantID: tenantID})
		if err == nil {
			t.Fatal("UpdateModel() expected error for non-super-admin")
		}
		if repo.updatedModel != nil {
			t.Fatal("UpdateModel() should not call repo.Update for non-super-admin")
		}
	})

	t.Run("update shared model allowed for super admin", func(t *testing.T) {
		repo := &fakeModelRepo{models: []*types.Model{sharedModel}}
		svc := &modelService{repo: repo}
		ctx := context.WithValue(context.Background(), types.TenantIDContextKey, tenantID)
		ctx = context.WithValue(ctx, types.UserContextKey, &types.User{ID: "sa", CanAccessAllTenants: true})

		err := svc.UpdateModel(ctx, &types.Model{ID: sharedModel.ID, TenantID: tenantID})
		if err != nil {
			t.Fatalf("UpdateModel() error = %v", err)
		}
		if repo.updatedModel == nil || repo.updatedModel.ID != sharedModel.ID {
			t.Fatal("UpdateModel() should call repo.Update for super-admin")
		}
	})

	t.Run("delete shared model denied for tenant admin", func(t *testing.T) {
		repo := &fakeModelRepo{models: []*types.Model{sharedModel}}
		svc := &modelService{repo: repo}
		ctx := context.WithValue(context.Background(), types.TenantIDContextKey, tenantID)

		err := svc.DeleteModel(ctx, sharedModel.ID)
		if err == nil {
			t.Fatal("DeleteModel() expected error for non-super-admin")
		}
		if repo.deletedID != "" {
			t.Fatal("DeleteModel() should not call repo.Delete for non-super-admin")
		}
	})

	t.Run("delete shared model allowed for super admin", func(t *testing.T) {
		repo := &fakeModelRepo{models: []*types.Model{sharedModel}}
		svc := &modelService{repo: repo}
		ctx := context.WithValue(context.Background(), types.TenantIDContextKey, tenantID)
		ctx = context.WithValue(ctx, types.UserContextKey, &types.User{ID: "sa", CanAccessAllTenants: true})

		err := svc.DeleteModel(ctx, sharedModel.ID)
		if err != nil {
			t.Fatalf("DeleteModel() error = %v", err)
		}
		if repo.deletedID != sharedModel.ID {
			t.Fatal("DeleteModel() should call repo.Delete for super-admin")
		}
	})
}
