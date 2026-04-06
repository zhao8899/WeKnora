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
			// No organization role — default to admin for tenant owner, viewer for others
			role = resolveDefaultRole(user)
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

// resolveDefaultRole assigns a default role based on user properties.
// Tenant owners (first user) get admin; others get editor as a safe default.
func resolveDefaultRole(user *types.User) types.OrgMemberRole {
	if user.CanAccessAllTenants {
		return types.OrgRoleAdmin
	}
	// Default to editor for regular authenticated users within their own tenant.
	// This allows basic CRUD while restricting admin-level operations.
	return types.OrgRoleEditor
}
