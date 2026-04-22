package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
)

type stubTenantService struct {
	lastUpdated *types.Tenant
}

func (s *stubTenantService) CreateTenant(ctx context.Context, tenant *types.Tenant) (*types.Tenant, error) {
	return tenant, nil
}

func (s *stubTenantService) GetTenantByID(ctx context.Context, id uint64) (*types.Tenant, error) {
	return nil, nil
}

func (s *stubTenantService) ListTenants(ctx context.Context) ([]*types.Tenant, error) {
	return nil, nil
}

func (s *stubTenantService) UpdateTenant(ctx context.Context, tenant *types.Tenant) (*types.Tenant, error) {
	cloned := *tenant
	s.lastUpdated = &cloned
	return tenant, nil
}

func (s *stubTenantService) DeleteTenant(ctx context.Context, id uint64) error {
	return nil
}

func (s *stubTenantService) UpdateAPIKey(ctx context.Context, id uint64) (string, error) {
	return "", nil
}

func (s *stubTenantService) ExtractTenantIDFromAPIKey(apiKey string) (uint64, error) {
	return 0, nil
}

func (s *stubTenantService) ListAllTenants(ctx context.Context) ([]*types.Tenant, error) {
	return nil, nil
}

func (s *stubTenantService) SearchTenants(ctx context.Context, keyword string, tenantID uint64, page, pageSize int) ([]*types.Tenant, int64, error) {
	return nil, 0, nil
}

func (s *stubTenantService) GetTenantByIDForUser(ctx context.Context, tenantID uint64, userID string) (*types.Tenant, error) {
	return nil, nil
}

func TestUpdateTenantAgentConfigInternal_PersistsTenantAgentConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)

	service := &stubTenantService{}
	handler := &TenantHandler{service: service}
	tenant := &types.Tenant{ID: 7}

	body := map[string]interface{}{
		"max_iterations": 18,
		"allowed_tools":  []string{"thinking", "query_knowledge_graph"},
		"temperature":    0.5,
		"system_prompt":  "tenant prompt",
	}
	payload, _ := json.Marshal(body)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/tenants/kv/agent-config", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(req.Context(), types.TenantInfoContextKey, tenant)
	c.Request = req.WithContext(ctx)

	handler.updateTenantAgentConfigInternal(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.lastUpdated == nil || service.lastUpdated.AgentConfig == nil {
		t.Fatal("expected tenant agent config to be persisted")
	}
	if service.lastUpdated.AgentConfig.MaxIterations != 18 {
		t.Fatalf("expected max_iterations=18, got %d", service.lastUpdated.AgentConfig.MaxIterations)
	}
	if len(service.lastUpdated.AgentConfig.AllowedTools) != 2 {
		t.Fatalf("expected allowed tools to be persisted, got %v", service.lastUpdated.AgentConfig.AllowedTools)
	}
	if service.lastUpdated.AgentConfig.SystemPrompt != "tenant prompt" {
		t.Fatalf("expected system prompt to be persisted, got %q", service.lastUpdated.AgentConfig.SystemPrompt)
	}
}

func TestGetTenantAgentConfig_ReturnsPersistedAllowedTools(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := &TenantHandler{}
	tenant := &types.Tenant{
		ID: 7,
		AgentConfig: &types.AgentConfig{
			MaxIterations: 22,
			AllowedTools:  []string{"thinking", "todo_write"},
			Temperature:   0.4,
			SystemPrompt:  "tenant prompt",
			UseCustomSystemPrompt: true,
		},
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tenants/kv/agent-config", nil)
	ctx := context.WithValue(req.Context(), types.TenantInfoContextKey, tenant)
	c.Request = req.WithContext(ctx)

	handler.GetTenantAgentConfig(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	var response struct {
		Data struct {
			AllowedTools []string `json:"allowed_tools"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(response.Data.AllowedTools) != 2 || response.Data.AllowedTools[1] != "todo_write" {
		t.Fatalf("expected persisted allowed tools, got %v", response.Data.AllowedTools)
	}
}
