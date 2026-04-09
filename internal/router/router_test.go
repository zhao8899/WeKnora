package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func injectUserAndTenant(user *types.User, tenant *types.Tenant) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		if user != nil {
			ctx = context.WithValue(ctx, types.UserContextKey, user)
			ctx = context.WithValue(ctx, types.UserIDContextKey, user.ID)
		}
		if tenant != nil {
			ctx = context.WithValue(ctx, types.TenantInfoContextKey, tenant)
			ctx = context.WithValue(ctx, types.TenantIDContextKey, tenant.ID)
		}
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

func TestRegisterTenantRoutesSuperAdminProtection(t *testing.T) {
	gin.SetMode(gin.TestMode)

	user := &types.User{ID: "owner-1", TenantID: 1001, CanAccessAllTenants: false}
	tenant := &types.Tenant{ID: 1001, OwnerID: "owner-1"}

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{name: "list all tenants", method: http.MethodGet, path: "/api/v1/tenants/all"},
		{name: "search tenants", method: http.MethodGet, path: "/api/v1/tenants/search"},
		{name: "create tenant", method: http.MethodPost, path: "/api/v1/tenants"},
		{name: "delete tenant", method: http.MethodDelete, path: "/api/v1/tenants/2002"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			r.Use(injectUserAndTenant(user, tenant))
			RegisterTenantRoutes(r.Group("/api/v1"), &handler.TenantHandler{})

			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			require.Equal(t, http.StatusForbidden, w.Code)
		})
	}
}

func TestRegisterSystemRoutesSuperAdminProtection(t *testing.T) {
	gin.SetMode(gin.TestMode)

	user := &types.User{ID: "owner-1", TenantID: 1001, CanAccessAllTenants: false}
	tenant := &types.Tenant{ID: 1001, OwnerID: "owner-1"}

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{name: "system info", method: http.MethodGet, path: "/api/v1/system/info"},
		{name: "system diagnostics", method: http.MethodGet, path: "/api/v1/system/diagnostics"},
		{name: "parser engines", method: http.MethodGet, path: "/api/v1/system/parser-engines"},
		{name: "storage engine status", method: http.MethodGet, path: "/api/v1/system/storage-engine-status"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			r.Use(injectUserAndTenant(user, tenant))
			RegisterSystemRoutes(r.Group("/api/v1"), &handler.SystemHandler{})

			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			require.Equal(t, http.StatusForbidden, w.Code)
		})
	}
}
