package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

// VectorStoreHandler handles HTTP requests for vector store CRUD.
type VectorStoreHandler struct {
	repo    interfaces.VectorStoreRepository
	service interfaces.VectorStoreService
}

// NewVectorStoreHandler creates a new handler.
func NewVectorStoreHandler(
	repo interfaces.VectorStoreRepository,
	service interfaces.VectorStoreService,
) *VectorStoreHandler {
	return &VectorStoreHandler{repo: repo, service: service}
}

// --- request DTOs ---

type CreateStoreRequest struct {
	Name             string                    `json:"name" binding:"required"`
	EngineType       types.RetrieverEngineType `json:"engine_type" binding:"required"`
	ConnectionConfig types.ConnectionConfig    `json:"connection_config" binding:"required"`
	IndexConfig      types.IndexConfig         `json:"index_config"`
}

// UpdateStoreRequest only allows the store name to be changed.
type UpdateStoreRequest struct {
	Name string `json:"name" binding:"required"`
}

type TestStoreRequest struct {
	EngineType       types.RetrieverEngineType `json:"engine_type" binding:"required"`
	ConnectionConfig types.ConnectionConfig    `json:"connection_config" binding:"required"`
}

type knowledgeBaseListResponse struct {
	Success bool                  `json:"success"`
	Data    []types.KnowledgeBase `json:"data"`
}

// --- helpers ---

func (h *VectorStoreHandler) getTenantID(c *gin.Context) uint64 {
	return c.GetUint64(types.TenantIDContextKey.String())
}

func (h *VectorStoreHandler) getOwnedStore(
	ctx context.Context, tenantID uint64, id string,
) (*types.VectorStore, int, string) {
	store, err := h.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, http.StatusInternalServerError, "failed to query vector store"
	}
	if store == nil {
		return nil, http.StatusNotFound, "vector store not found"
	}
	return store, http.StatusOK, ""
}

func envStoreReadonlyError() gin.H {
	return gin.H{"success": false, "error": "environment-configured vector stores cannot be modified via API"}
}

func requestBaseURL(req *http.Request) string {
	scheme := strings.TrimSpace(req.Header.Get("X-Forwarded-Proto"))
	if scheme == "" {
		if req.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	host := strings.TrimSpace(req.Host)
	if host == "" {
		host = "localhost:8080"
	}
	return fmt.Sprintf("%s://%s", scheme, host)
}

func cloneRequestHeaders(src http.Header) http.Header {
	cloned := make(http.Header, len(src))
	for key, values := range src {
		cloned[key] = append([]string(nil), values...)
	}
	return cloned
}

func (h *VectorStoreHandler) fetchKnowledgeBases(c *gin.Context) ([]types.KnowledgeBase, error) {
	reqURL, err := url.Parse(requestBaseURL(c.Request))
	if err != nil {
		return nil, err
	}
	reqURL.Path = "/api/v1/knowledge-bases"

	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, reqURL.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header = cloneRequestHeaders(c.Request.Header)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected knowledge base list status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload knowledgeBaseListResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	for i := range payload.Data {
		payload.Data[i].EnsureDefaults()
	}
	return payload.Data, nil
}

func countKnowledgeBasesByVectorStore(knowledgeBases []types.KnowledgeBase, storeID string) int {
	if storeID == "" {
		return 0
	}
	count := 0
	for i := range knowledgeBases {
		if knowledgeBases[i].VectorStoreID != nil && *knowledgeBases[i].VectorStoreID == storeID {
			count++
		}
	}
	return count
}

func buildKnowledgeBaseBindings(
	knowledgeBases []types.KnowledgeBase, storeID string,
) []types.VectorStoreKnowledgeBaseBinding {
	if storeID == "" {
		return nil
	}
	bindings := make([]types.VectorStoreKnowledgeBaseBinding, 0)
	for i := range knowledgeBases {
		kb := knowledgeBases[i]
		if kb.VectorStoreID == nil || *kb.VectorStoreID != storeID {
			continue
		}
		bindings = append(bindings, types.VectorStoreKnowledgeBaseBinding{
			ID:               kb.ID,
			Name:             kb.Name,
			Type:             kb.Type,
			VectorStoreID:    storeID,
			KnowledgeCount:   kb.KnowledgeCount,
			ChunkCount:       kb.ChunkCount,
			UpdatedAt:        kb.UpdatedAt,
			IsTemporary:      kb.IsTemporary,
			IndexingStrategy: kb.IndexingStrategy,
		})
	}
	return bindings
}

// --- endpoints ---

// CreateStore creates a new vector store configuration for the current tenant.
func (h *VectorStoreHandler) CreateStore(c *gin.Context) {
	ctx := c.Request.Context()

	tenantID := h.getTenantID(c)
	if tenantID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "unauthorized: tenant context missing"})
		return
	}

	var req CreateStoreRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Warnf(ctx, "Invalid create vector store request: %v", err)
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}

	store := &types.VectorStore{
		TenantID:         tenantID,
		Name:             req.Name,
		EngineType:       req.EngineType,
		ConnectionConfig: req.ConnectionConfig,
		IndexConfig:      req.IndexConfig,
	}

	if err := h.service.CreateStore(ctx, store); err != nil {
		logger.Warnf(ctx, "Failed to create vector store: %v", err)
		c.Error(err)
		return
	}

	resp := types.NewVectorStoreResponse(store, "user", false)
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": resp})
}

// ListStores lists all vector stores for the current tenant.
func (h *VectorStoreHandler) ListStores(c *gin.Context) {
	ctx := c.Request.Context()

	tenantID := h.getTenantID(c)
	if tenantID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "unauthorized: tenant context missing"})
		return
	}

	knowledgeBases, err := h.fetchKnowledgeBases(c)
	if err != nil {
		logger.Warnf(ctx, "Failed to load knowledge base bindings for vector stores: %v", err)
	}

	dbStores, err := h.repo.List(ctx, tenantID)
	if err != nil {
		logger.Warnf(ctx, "Failed to list vector stores: %v", err)
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	maskedDBStores := make([]types.VectorStoreResponse, len(dbStores))
	for i, s := range dbStores {
		resp := types.NewVectorStoreResponse(s, "user", false)
		resp.KnowledgeBaseCount = countKnowledgeBasesByVectorStore(knowledgeBases, s.ID)
		maskedDBStores[i] = resp
	}

	envStores := types.BuildEnvVectorStores(os.Getenv("RETRIEVE_DRIVER"), os.Getenv)
	maskedEnvStores := make([]types.VectorStoreResponse, len(envStores))
	for i := range envStores {
		resp := types.NewVectorStoreResponse(&envStores[i], "env", true)
		resp.KnowledgeBaseCount = countKnowledgeBasesByVectorStore(knowledgeBases, envStores[i].ID)
		maskedEnvStores[i] = resp
	}

	allStores := append(maskedEnvStores, maskedDBStores...)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": allStores})
}

// GetStore retrieves a single vector store by ID.
func (h *VectorStoreHandler) GetStore(c *gin.Context) {
	ctx := c.Request.Context()

	tenantID := h.getTenantID(c)
	if tenantID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "unauthorized: tenant context missing"})
		return
	}

	knowledgeBases, err := h.fetchKnowledgeBases(c)
	if err != nil {
		logger.Warnf(ctx, "Failed to load knowledge base bindings for vector store detail: %v", err)
	}

	id := c.Param("id")

	if types.IsEnvStoreID(id) {
		envStore := types.FindEnvVectorStore(os.Getenv("RETRIEVE_DRIVER"), os.Getenv, id)
		if envStore == nil {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "vector store not found"})
			return
		}
		resp := types.NewVectorStoreResponse(envStore, "env", true)
		resp.KnowledgeBaseCount = countKnowledgeBasesByVectorStore(knowledgeBases, id)
		c.JSON(http.StatusOK, gin.H{"success": true, "data": resp})
		return
	}

	store, status, msg := h.getOwnedStore(ctx, tenantID, id)
	if status != http.StatusOK {
		c.JSON(status, gin.H{"success": false, "error": msg})
		return
	}

	resp := types.NewVectorStoreResponse(store, "user", false)
	resp.KnowledgeBaseCount = countKnowledgeBasesByVectorStore(knowledgeBases, id)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": resp})
}

// UpdateStore updates a vector store (name only).
func (h *VectorStoreHandler) UpdateStore(c *gin.Context) {
	ctx := c.Request.Context()

	tenantID := h.getTenantID(c)
	if tenantID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "unauthorized: tenant context missing"})
		return
	}

	id := c.Param("id")
	if types.IsEnvStoreID(id) {
		c.JSON(http.StatusBadRequest, envStoreReadonlyError())
		return
	}

	if _, status, msg := h.getOwnedStore(ctx, tenantID, id); status != http.StatusOK {
		c.JSON(status, gin.H{"success": false, "error": msg})
		return
	}

	knowledgeBases, err := h.fetchKnowledgeBases(c)
	if err != nil {
		logger.Warnf(ctx, "Failed to load knowledge base bindings for vector store update: %v", err)
	}

	var req UpdateStoreRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}

	updated := &types.VectorStore{
		ID:       id,
		TenantID: tenantID,
		Name:     req.Name,
	}

	if err := h.service.UpdateStore(ctx, updated); err != nil {
		logger.Warnf(ctx, "Failed to update vector store %s: %v", id, err)
		c.Error(err)
		return
	}

	result, err := h.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		logger.Warnf(ctx, "Failed to re-fetch vector store %s after update: %v", id, err)
	}
	if result != nil {
		resp := types.NewVectorStoreResponse(result, "user", false)
		resp.KnowledgeBaseCount = countKnowledgeBasesByVectorStore(knowledgeBases, id)
		c.JSON(http.StatusOK, gin.H{"success": true, "data": resp})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": nil})
}

// DeleteStore soft-deletes a vector store.
func (h *VectorStoreHandler) DeleteStore(c *gin.Context) {
	ctx := c.Request.Context()

	tenantID := h.getTenantID(c)
	if tenantID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "unauthorized: tenant context missing"})
		return
	}

	id := c.Param("id")
	if types.IsEnvStoreID(id) {
		c.JSON(http.StatusBadRequest, envStoreReadonlyError())
		return
	}

	if _, status, msg := h.getOwnedStore(ctx, tenantID, id); status != http.StatusOK {
		c.JSON(status, gin.H{"success": false, "error": msg})
		return
	}

	if err := h.service.DeleteStore(ctx, tenantID, id); err != nil {
		logger.Warnf(ctx, "Failed to delete vector store %s: %v", id, err)
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ListStoreTypes returns supported engine types with connection and index schemas.
func (h *VectorStoreHandler) ListStoreTypes(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    types.GetVectorStoreTypes(),
	})
}

// TestStoreByID tests an existing saved or env store connection.
func (h *VectorStoreHandler) TestStoreByID(c *gin.Context) {
	ctx := c.Request.Context()

	tenantID := h.getTenantID(c)
	if tenantID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "unauthorized: tenant context missing"})
		return
	}

	id := c.Param("id")
	if types.IsEnvStoreID(id) {
		envStore := types.FindEnvVectorStore(os.Getenv("RETRIEVE_DRIVER"), os.Getenv, id)
		if envStore == nil {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "vector store not found"})
			return
		}
		version, err := h.service.TestConnection(ctx, envStore.EngineType, envStore.ConnectionConfig)
		if err != nil {
			logger.Warnf(ctx, "Vector store connection test failed: %v", err)
			c.JSON(http.StatusOK, gin.H{"success": false, "error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "version": version})
		return
	}

	store, status, msg := h.getOwnedStore(ctx, tenantID, id)
	if status != http.StatusOK {
		c.JSON(status, gin.H{"success": false, "error": msg})
		return
	}

	version, err := h.service.TestConnection(ctx, store.EngineType, store.ConnectionConfig)
	if err != nil {
		logger.Warnf(ctx, "Vector store connection test failed: %v", err)
		c.JSON(http.StatusOK, gin.H{"success": false, "error": err.Error()})
		return
	}

	if version != "" && version != store.ConnectionConfig.Version {
		if updateErr := h.service.SaveDetectedVersion(ctx, store, version); updateErr != nil {
			logger.Warnf(ctx, "Failed to update detected version for store %s: %v", store.ID, updateErr)
		}
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "version": version})
}

// TestStoreRaw tests a vector store connection using raw credentials.
func (h *VectorStoreHandler) TestStoreRaw(c *gin.Context) {
	ctx := c.Request.Context()

	tenantID := h.getTenantID(c)
	if tenantID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "unauthorized: tenant context missing"})
		return
	}

	var req TestStoreRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}

	version, err := h.service.TestConnection(ctx, req.EngineType, req.ConnectionConfig)
	if err != nil {
		logger.Warnf(ctx, "Vector store connection test failed: %v", err)
		c.JSON(http.StatusOK, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "version": version})
}
