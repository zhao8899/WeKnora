package handler

import (
	"context"
	"fmt"
	"net/http"

	"github.com/Tencent/WeKnora/internal/errors"
	infra_web_search "github.com/Tencent/WeKnora/internal/infrastructure/web_search"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	secutils "github.com/Tencent/WeKnora/internal/utils"
	"github.com/gin-gonic/gin"
)

// WebSearchProviderHandler handles HTTP requests for web search provider CRUD
type WebSearchProviderHandler struct {
	repo     interfaces.WebSearchProviderRepository
	service  interfaces.WebSearchProviderService
	registry *infra_web_search.Registry
}

// NewWebSearchProviderHandler creates a new handler
func NewWebSearchProviderHandler(
	repo interfaces.WebSearchProviderRepository,
	service interfaces.WebSearchProviderService,
	registry *infra_web_search.Registry,
) *WebSearchProviderHandler {
	return &WebSearchProviderHandler{repo: repo, service: service, registry: registry}
}

// --- request DTOs ---

// CreateProviderRequest defines the request body for creating a provider
type CreateProviderRequest struct {
	Name        string                            `json:"name" binding:"required"`
	Provider    types.WebSearchProviderType       `json:"provider" binding:"required"`
	Description string                            `json:"description"`
	Parameters  types.WebSearchProviderParameters `json:"parameters"`
	IsDefault   bool                              `json:"is_default"`
	IsPlatform  bool                              `json:"is_platform"`
}

// UpdateProviderRequest defines the request body for updating a provider
type UpdateProviderRequest struct {
	Name        string                            `json:"name"`
	Description string                            `json:"description"`
	Parameters  types.WebSearchProviderParameters `json:"parameters"`
	IsDefault   bool                              `json:"is_default"`
	IsPlatform  *bool                             `json:"is_platform"`
}

// --- helpers ---

// getTenantID extracts tenant ID from gin context (set by auth middleware).
func (h *WebSearchProviderHandler) getTenantID(c *gin.Context) uint64 {
	return c.GetUint64(types.TenantIDContextKey.String())
}

func (h *WebSearchProviderHandler) hideSensitiveInfo(provider *types.WebSearchProviderEntity, c *gin.Context) *types.WebSearchProviderEntity {
	if provider == nil || !provider.IsPlatform || types.IsSuperAdmin(c.Request.Context()) {
		return provider
	}

	copy := *provider
	copy.Parameters = provider.Parameters
	copy.Parameters.APIKey = ""
	return &copy
}

func (h *WebSearchProviderHandler) canManagePlatformProviders(c *gin.Context) bool {
	return types.IsSuperAdmin(c.Request.Context())
}

// getAccessibleProvider loads a provider visible to the given tenant.
// Tenants can access their own providers and platform-shared providers.
// Returns (nil, status, msg) on failure so callers can respond immediately.
func (h *WebSearchProviderHandler) getAccessibleProvider(
	ctx context.Context, tenantID uint64, id string,
) (*types.WebSearchProviderEntity, int, string) {
	provider, err := h.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, http.StatusInternalServerError, "failed to query provider"
	}
	if provider == nil {
		return nil, http.StatusNotFound, "web search provider not found"
	}
	return provider, http.StatusOK, ""
}

// --- endpoints ---

// CreateProvider creates a new web search provider
func (h *WebSearchProviderHandler) CreateProvider(c *gin.Context) {
	ctx := c.Request.Context()

	tenantID := h.getTenantID(c)
	if tenantID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "unauthorized: tenant context missing"})
		return
	}

	var req CreateProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Warnf(ctx, "Invalid create provider request: %v", err)
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}

	if req.IsPlatform && !h.canManagePlatformProviders(c) {
		c.Error(errors.NewForbiddenError("platform web search providers can only be managed by super admins"))
		return
	}

	logger.Infof(
		ctx,
		"Creating web search provider: tenant=%d, platform=%t, name=%s, type=%s",
		tenantID, req.IsPlatform, secutils.SanitizeForLog(req.Name), secutils.SanitizeForLog(string(req.Provider)),
	)

	provider := &types.WebSearchProviderEntity{
		TenantID:    tenantID,
		Name:        secutils.SanitizeForLog(req.Name),
		Provider:    req.Provider,
		Description: secutils.SanitizeForLog(req.Description),
		Parameters:  req.Parameters,
		IsDefault:   req.IsDefault,
		IsPlatform:  req.IsPlatform,
	}

	if err := h.service.CreateProvider(ctx, provider); err != nil {
		logger.Warnf(ctx, "Failed to create web search provider: %v", err)
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    h.hideSensitiveInfo(provider, c),
	})
}

// ListProviders lists all web search providers for the current tenant
func (h *WebSearchProviderHandler) ListProviders(c *gin.Context) {
	ctx := c.Request.Context()

	tenantID := h.getTenantID(c)
	if tenantID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "unauthorized: tenant context missing"})
		return
	}

	providers, err := h.repo.List(ctx, tenantID)
	if err != nil {
		logger.Warnf(ctx, "Failed to list web search providers: %v", err)
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	result := make([]*types.WebSearchProviderEntity, 0, len(providers))
	for _, provider := range providers {
		result = append(result, h.hideSensitiveInfo(provider, c))
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

// GetProvider retrieves a single web search provider by ID
func (h *WebSearchProviderHandler) GetProvider(c *gin.Context) {
	ctx := c.Request.Context()

	tenantID := h.getTenantID(c)
	if tenantID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "unauthorized: tenant context missing"})
		return
	}

	id := c.Param("id")
	provider, status, msg := h.getAccessibleProvider(ctx, tenantID, id)
	if status != http.StatusOK {
		c.JSON(status, gin.H{"success": false, "error": msg})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    h.hideSensitiveInfo(provider, c),
	})
}

// UpdateProvider updates a web search provider
func (h *WebSearchProviderHandler) UpdateProvider(c *gin.Context) {
	ctx := c.Request.Context()

	tenantID := h.getTenantID(c)
	if tenantID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "unauthorized: tenant context missing"})
		return
	}

	id := c.Param("id")

	existing, status, msg := h.getAccessibleProvider(ctx, tenantID, id)
	if status != http.StatusOK {
		c.JSON(status, gin.H{"success": false, "error": msg})
		return
	}
	if existing.IsPlatform && !h.canManagePlatformProviders(c) {
		c.Error(errors.NewForbiddenError("platform web search providers are read-only for tenant admins"))
		return
	}

	var req UpdateProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}

	isPlatform := existing.IsPlatform
	if req.IsPlatform != nil {
		if !h.canManagePlatformProviders(c) {
			c.Error(errors.NewForbiddenError("platform web search providers can only be managed by super admins"))
			return
		}
		isPlatform = *req.IsPlatform
	}

	parameters := existing.Parameters
	if req.Parameters.APIKey != "" {
		parameters.APIKey = req.Parameters.APIKey
	}
	if req.Parameters.EngineID != "" || existing.Parameters.EngineID == "" {
		parameters.EngineID = req.Parameters.EngineID
	}
	if req.Parameters.ExtraConfig != nil {
		parameters.ExtraConfig = req.Parameters.ExtraConfig
	}

	name := existing.Name
	if req.Name != "" {
		name = secutils.SanitizeForLog(req.Name)
	}

	// Build updated entity, keeping immutable fields from existing
	provider := &types.WebSearchProviderEntity{
		ID:          id,
		TenantID:    existing.TenantID,
		Name:        name,
		Provider:    existing.Provider, // Provider type is immutable after creation
		Description: secutils.SanitizeForLog(req.Description),
		Parameters:  parameters,
		IsDefault:   req.IsDefault,
		IsPlatform:  isPlatform,
	}

	if err := h.service.UpdateProvider(ctx, provider); err != nil {
		logger.Warnf(ctx, "Failed to update web search provider %s: %v", id, err)
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	// Re-fetch to get the full stored state
	updated, _ := h.repo.GetByID(ctx, tenantID, id)
	if updated != nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": h.hideSensitiveInfo(updated, c)})
	} else {
		c.JSON(http.StatusOK, gin.H{"success": true})
	}
}

// DeleteProvider deletes a web search provider
func (h *WebSearchProviderHandler) DeleteProvider(c *gin.Context) {
	ctx := c.Request.Context()

	tenantID := h.getTenantID(c)
	if tenantID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "unauthorized: tenant context missing"})
		return
	}

	id := c.Param("id")

	existing, status, msg := h.getAccessibleProvider(ctx, tenantID, id)
	if status != http.StatusOK {
		c.JSON(status, gin.H{"success": false, "error": msg})
		return
	}
	if existing.IsPlatform && !h.canManagePlatformProviders(c) {
		c.Error(errors.NewForbiddenError("platform web search providers can only be deleted by super admins"))
		return
	}

	if err := h.service.DeleteProvider(ctx, existing.TenantID, id); err != nil {
		logger.Warnf(ctx, "Failed to delete web search provider %s: %v", id, err)
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ListProviderTypes returns available provider types and their parameter requirements
func (h *WebSearchProviderHandler) ListProviderTypes(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    types.GetWebSearchProviderTypes(),
	})
}

// TestProviderByID tests an existing saved provider by performing a sample search
func (h *WebSearchProviderHandler) TestProviderByID(c *gin.Context) {
	ctx := c.Request.Context()

	tenantID := h.getTenantID(c)
	if tenantID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "unauthorized: tenant context missing"})
		return
	}

	id := c.Param("id")
	provider, status, msg := h.getAccessibleProvider(ctx, tenantID, id)
	if status != http.StatusOK {
		c.JSON(status, gin.H{"success": false, "error": msg})
		return
	}

	if err := h.doTestSearch(ctx, string(provider.Provider), provider.Parameters); err != nil {
		logger.Warnf(ctx, "Web search provider test failed: %v", err)
		c.JSON(http.StatusOK, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// TestProviderRequest defines the body for testing raw credentials
type TestProviderRequest struct {
	Provider   string                            `json:"provider" binding:"required"`
	Parameters types.WebSearchProviderParameters `json:"parameters"`
}

// TestProviderRaw tests a provider with raw credentials (no persistence)
func (h *WebSearchProviderHandler) TestProviderRaw(c *gin.Context) {
	ctx := c.Request.Context()

	var req TestProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}

	if err := h.doTestSearch(ctx, req.Provider, req.Parameters); err != nil {
		logger.Warnf(ctx, "Web search provider test failed: %v", err)
		c.JSON(http.StatusOK, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// doTestSearch creates a temporary provider and runs a simple test query
func (h *WebSearchProviderHandler) doTestSearch(ctx context.Context, providerType string, params types.WebSearchProviderParameters) error {
	logger.Infof(ctx, "[WebSearch][Test] testing provider type=%s", providerType)
	searchProvider, err := h.registry.CreateProvider(providerType, params)
	if err != nil {
		logger.Warnf(ctx, "[WebSearch][Test] failed to create provider: %v", err)
		return fmt.Errorf("failed to create provider: %w", err)
	}
	results, err := searchProvider.Search(ctx, "test", 1, false)
	if err != nil {
		logger.Warnf(ctx, "[WebSearch][Test] search failed: %v", err)
		return err
	}
	if len(results) == 0 {
		logger.Warnf(ctx, "[WebSearch][Test] search returned 0 results — API key or configuration may be invalid")
		return fmt.Errorf("search returned 0 results, please verify your API key and configuration")
	}
	logger.Infof(ctx, "[WebSearch][Test] succeeded: type=%s, results=%d", providerType, len(results))
	return nil
}
