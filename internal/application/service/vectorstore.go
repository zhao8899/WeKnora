package service

import (
	"context"
	"fmt"
	"os"

	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	secutils "github.com/Tencent/WeKnora/internal/utils"
)

type vectorStoreService struct {
	repo interfaces.VectorStoreRepository
}

func NewVectorStoreService(repo interfaces.VectorStoreRepository) interfaces.VectorStoreService {
	return &vectorStoreService{repo: repo}
}

func (s *vectorStoreService) CreateStore(ctx context.Context, store *types.VectorStore) error {
	if err := store.Validate(); err != nil {
		return err
	}
	if err := validateConnectionConfig(store.EngineType, store.ConnectionConfig); err != nil {
		return err
	}
	if err := types.ValidateIndexConfig(store.IndexConfig); err != nil {
		return err
	}

	endpoint := store.ConnectionConfig.GetEndpoint()
	indexName := store.IndexConfig.GetIndexNameOrDefault(store.EngineType)
	exists, err := s.repo.ExistsByEndpointAndIndex(ctx, store.TenantID, store.EngineType, endpoint, indexName)
	if err != nil {
		return errors.NewInternalServerError("failed to check for duplicate vector stores")
	}
	if exists {
		return errors.NewConflictError("a vector store with the same endpoint and index already exists")
	}

	for _, envStore := range types.BuildEnvVectorStores(os.Getenv("RETRIEVE_DRIVER"), os.Getenv) {
		if envStore.EngineType == store.EngineType &&
			envStore.ConnectionConfig.GetEndpoint() == endpoint &&
			envStore.IndexConfig.GetIndexNameOrDefault(store.EngineType) == indexName {
			return errors.NewConflictError(
				"a vector store with the same endpoint and index is already configured via environment variables")
		}
	}

	version, err := s.TestConnection(ctx, store.EngineType, store.ConnectionConfig)
	if err != nil {
		return errors.NewBadRequestError(
			fmt.Sprintf("connection test failed: %s. Ensure the server is reachable before saving.", err.Error()))
	}
	if version != "" {
		store.ConnectionConfig.Version = version
	}

	logger.Infof(ctx, "Creating vector store: tenant=%d, name=%s, engine=%s",
		store.TenantID, secutils.SanitizeForLog(store.Name), store.EngineType)
	return s.repo.Create(ctx, store)
}

func (s *vectorStoreService) UpdateStore(ctx context.Context, store *types.VectorStore) error {
	if store.TenantID == 0 {
		return errors.NewValidationError("tenant_id is required")
	}
	if store.Name == "" {
		return errors.NewValidationError("name is required")
	}
	logger.Infof(ctx, "Updating vector store: tenant=%d, id=%s", store.TenantID, store.ID)
	return s.repo.Update(ctx, store)
}

func (s *vectorStoreService) DeleteStore(ctx context.Context, tenantID uint64, id string) error {
	if err := s.repo.Delete(ctx, tenantID, id); err != nil {
		return err
	}
	logger.Infof(ctx, "Deleted vector store: tenant=%d, id=%s", tenantID, id)
	return nil
}

func (s *vectorStoreService) SaveDetectedVersion(ctx context.Context, store *types.VectorStore, version string) error {
	updated := *store
	updated.ConnectionConfig.Version = version
	return s.repo.UpdateConnectionConfig(ctx, &updated)
}

func validateConnectionConfig(engineType types.RetrieverEngineType, config types.ConnectionConfig) error {
	switch engineType {
	case types.ElasticsearchRetrieverEngineType:
		if config.Addr == "" {
			return errors.NewValidationError("addr is required for elasticsearch")
		}
	case types.PostgresRetrieverEngineType:
		if !config.UseDefaultConnection && config.Addr == "" {
			return errors.NewValidationError("addr or use_default_connection is required for postgres")
		}
	case types.QdrantRetrieverEngineType:
		if config.Host == "" {
			return errors.NewValidationError("host is required for qdrant")
		}
	case types.MilvusRetrieverEngineType:
		if config.Addr == "" {
			return errors.NewValidationError("addr is required for milvus")
		}
	case types.WeaviateRetrieverEngineType:
		if config.Host == "" {
			return errors.NewValidationError("host is required for weaviate")
		}
	case types.SQLiteRetrieverEngineType:
	default:
		return errors.NewValidationError(fmt.Sprintf("unsupported engine type: %s", engineType))
	}
	return nil
}
