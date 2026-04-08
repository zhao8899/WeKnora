package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRequireSuperAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("unauthorized_when_user_missing", func(t *testing.T) {
		r := gin.New()
		r.Use(RequireSuperAdmin())
		r.GET("/protected", func(c *gin.Context) {
			c.Status(http.StatusOK)
		})

		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("forbidden_when_not_super_admin", func(t *testing.T) {
		r := gin.New()
		r.Use(func(c *gin.Context) {
			user := &types.User{ID: "u1", TenantID: 10000, CanAccessAllTenants: false}
			ctx := context.WithValue(c.Request.Context(), types.UserContextKey, user)
			c.Request = c.Request.WithContext(ctx)
			c.Next()
		})
		r.Use(RequireSuperAdmin())
		r.GET("/protected", func(c *gin.Context) {
			c.Status(http.StatusOK)
		})

		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("ok_when_super_admin", func(t *testing.T) {
		r := gin.New()
		r.Use(func(c *gin.Context) {
			user := &types.User{ID: "u2", TenantID: 10000, CanAccessAllTenants: true}
			ctx := context.WithValue(c.Request.Context(), types.UserContextKey, user)
			c.Request = c.Request.WithContext(ctx)
			c.Next()
		})
		r.Use(RequireSuperAdmin())
		r.GET("/protected", func(c *gin.Context) {
			c.Status(http.StatusOK)
		})

		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
	})
}

func TestResolveDefaultRole(t *testing.T) {
	superAdmin := &types.User{ID: "sa", CanAccessAllTenants: true}
	owner := &types.User{ID: "owner", CanAccessAllTenants: false}
	normal := &types.User{ID: "u", CanAccessAllTenants: false}
	tenant := &types.Tenant{ID: 10000, OwnerID: "owner"}

	require.Equal(t, types.OrgRoleAdmin, resolveDefaultRole(superAdmin, tenant))
	require.Equal(t, types.OrgRoleAdmin, resolveDefaultRole(owner, tenant))
	require.Equal(t, types.OrgRoleViewer, resolveDefaultRole(normal, tenant))
}
