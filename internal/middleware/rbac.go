package middleware

import (
	"net/http"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
)

// RequireRole returns a Gin middleware that enforces a minimum organization role.
// It reads the user from context (set by Auth middleware) and checks whether
// the user's role meets the required level.
//
// Usage in route registration:
//
//	admin := v1.Group("/admin")
//	admin.Use(middleware.RequireRole(types.OrgRoleAdmin))
//
// For endpoints that allow editors:
//
//	kb.PUT("/:id", middleware.RequireRole(types.OrgRoleEditor), handler.Update)
func RequireRole(required types.OrgMemberRole) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := c.Request.Context().Value(types.UserContextKey).(*types.User)
		if !ok || user == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "unauthorized",
					"message": "Authentication required",
				},
			})
			return
		}

		// If user has the role stored in context (set during auth when org membership is resolved)
		role, _ := c.Request.Context().Value(types.OrgRoleContextKey).(types.OrgMemberRole)
		if role == "" {
			// No organization role — resolve from user + tenant ownership
			tenant, _ := c.Request.Context().Value(types.TenantInfoContextKey).(*types.Tenant)
			role = resolveDefaultRole(user, tenant)
		}

		if !role.HasPermission(required) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "forbidden",
					"message": "Insufficient permissions. Required role: " + string(required),
				},
			})
			return
		}

		c.Next()
	}
}

// RequireSuperAdmin enforces super-admin access.
// Super-admin is defined as a user with can_access_all_tenants=true.
func RequireSuperAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := c.Request.Context().Value(types.UserContextKey).(*types.User)
		if !ok || user == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "unauthorized",
					"message": "Authentication required",
				},
			})
			return
		}
		if !user.CanAccessAllTenants {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "forbidden",
					"message": "Super-admin access required",
				},
			})
			return
		}
		c.Next()
	}
}

// resolveDefaultRole assigns a default role based on user properties and tenant ownership.
// Super-admins and tenant owners get admin; other authenticated users default to viewer.
func resolveDefaultRole(user *types.User, tenant *types.Tenant) types.OrgMemberRole {
	// Super-admin: can access all tenants
	if user.CanAccessAllTenants {
		return types.OrgRoleAdmin
	}
	// Tenant owner: the user who created this tenant
	if tenant != nil && tenant.OwnerID != "" && tenant.OwnerID == user.ID {
		return types.OrgRoleAdmin
	}
	// Strict default for non-owner users: viewer only.
	return types.OrgRoleViewer
}
