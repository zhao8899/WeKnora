package service

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockVectorStoreRepo struct {
	stores           []*types.VectorStore
	createErr        error
	updateErr        error
	deleteErr        error
	existsErr        error
	existsDuplicate  bool
	updatedStoreName string
}

func (m *mockVectorStoreRepo) Create(_ context.Context, store *types.VectorStore) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.stores = append(m.stores, store)
	return nil
}

func (m *mockVectorStoreRepo) GetByID(_ context.Context, tenantID uint64, id string) (*types.VectorStore, error) {
	for _, s := range m.stores {
		if s.TenantID == tenantID && s.ID == id {
			return s, nil
		}
	}
	return nil, nil
}

func (m *mockVectorStoreRepo) List(_ context.Context, tenantID uint64) ([]*types.VectorStore, error) {
	var result []*types.VectorStore
	for _, s := range m.stores {
		if s.TenantID == tenantID {
			result = append(result, s)
		}
	}
	return result, nil
}

func (m *mockVectorStoreRepo) Update(_ context.Context, store *types.VectorStore) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	m.updatedStoreName = store.Name
	return nil
}

func (m *mockVectorStoreRepo) UpdateConnectionConfig(_ context.Context, _ *types.VectorStore) error {
	return m.updateErr
}

func (m *mockVectorStoreRepo) Delete(_ context.Context, _ uint64, _ string) error {
	return m.deleteErr
}

func (m *mockVectorStoreRepo) ExistsByEndpointAndIndex(
	_ context.Context,
	_ uint64,
	_ types.RetrieverEngineType,
	_ string,
	_ string,
) (bool, error) {
	if m.existsErr != nil {
		return false, m.existsErr
	}
	return m.existsDuplicate, nil
}

func TestCreateStore_SQLiteSuccess(t *testing.T) {
	repo := &mockVectorStoreRepo{}
	svc := NewVectorStoreService(repo)

	err := svc.CreateStore(context.Background(), &types.VectorStore{
		TenantID:   1,
		Name:       "local-sqlite",
		EngineType: types.SQLiteRetrieverEngineType,
	})

	require.NoError(t, err)
	require.Len(t, repo.stores, 1)
	assert.Equal(t, "local-sqlite", repo.stores[0].Name)
}

func TestCreateStore_ValidationAndDuplicateErrors(t *testing.T) {
	t.Run("missing connection", func(t *testing.T) {
		svc := NewVectorStoreService(&mockVectorStoreRepo{})
		err := svc.CreateStore(context.Background(), &types.VectorStore{
			TenantID:   1,
			Name:       "bad-es",
			EngineType: types.ElasticsearchRetrieverEngineType,
		})
		require.Error(t, err)
	})

	t.Run("duplicate endpoint index", func(t *testing.T) {
		svc := NewVectorStoreService(&mockVectorStoreRepo{existsDuplicate: true})
		err := svc.CreateStore(context.Background(), &types.VectorStore{
			TenantID:   1,
			Name:       "dup",
			EngineType: types.SQLiteRetrieverEngineType,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "same endpoint and index")
	})
}

func TestUpdateDeleteAndSaveVersion(t *testing.T) {
	repo := &mockVectorStoreRepo{}
	svc := NewVectorStoreService(repo)

	err := svc.UpdateStore(context.Background(), &types.VectorStore{ID: "s1", TenantID: 1, Name: "renamed"})
	require.NoError(t, err)
	assert.Equal(t, "renamed", repo.updatedStoreName)

	err = svc.SaveDetectedVersion(context.Background(), &types.VectorStore{
		ID:               "s1",
		TenantID:         1,
		ConnectionConfig: types.ConnectionConfig{Version: "old"},
	}, "new")
	require.NoError(t, err)

	err = svc.DeleteStore(context.Background(), 1, "s1")
	require.NoError(t, err)
}

func TestTestConnection_LocalCases(t *testing.T) {
	svc := NewVectorStoreService(&mockVectorStoreRepo{})

	version, err := svc.TestConnection(context.Background(), types.SQLiteRetrieverEngineType, types.ConnectionConfig{})
	require.NoError(t, err)
	assert.Empty(t, version)

	version, err = svc.TestConnection(
		context.Background(),
		types.PostgresRetrieverEngineType,
		types.ConnectionConfig{UseDefaultConnection: true},
	)
	require.NoError(t, err)
	assert.Empty(t, version)
}
