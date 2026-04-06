package middleware

import (
	"strings"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// auditWriter wraps the audit log repository for the middleware.
type auditWriter struct {
	db *gorm.DB
}

// Audit returns a Gin middleware that records audit logs for mutating API requests.
// Read-only requests (GET, HEAD, OPTIONS) are skipped to keep the log volume manageable.
func Audit(db *gorm.DB) gin.HandlerFunc {
	w := &auditWriter{db: db}

	return func(c *gin.Context) {
		// Only audit mutating requests
		method := c.Request.Method
		if method == "GET" || method == "HEAD" || method == "OPTIONS" {
			c.Next()
			return
		}

		// Process the request first so we know the status code
		c.Next()

		// Extract identity from context (set by Auth middleware)
		tenantID, _ := c.Request.Context().Value(types.TenantIDContextKey).(uint64)
		if tenantID == 0 {
			return // unauthenticated or health-check — nothing to audit
		}

		userID, _ := c.Request.Context().Value(types.UserIDContextKey).(string)
		var username string
		if user, ok := c.Request.Context().Value(types.UserContextKey).(*types.User); ok && user != nil {
			username = user.Username
		}

		action, resourceType, resourceID := classifyRequest(method, c.Request.URL.Path, c.Params)

		log := &types.AuditLog{
			TenantID:      tenantID,
			UserID:        userID,
			Username:      username,
			Action:        action,
			ResourceType:  resourceType,
			ResourceID:    resourceID,
			IPAddress:     c.ClientIP(),
			UserAgent:     c.Request.UserAgent(),
			RequestMethod: method,
			RequestPath:   c.Request.URL.Path,
			StatusCode:    c.Writer.Status(),
		}

		w.save(c, log)
	}
}

// save persists the audit log entry. Errors are logged but never fail the request.
func (w *auditWriter) save(c *gin.Context, log *types.AuditLog) {
	if err := w.db.WithContext(c.Request.Context()).Create(log).Error; err != nil {
		logger.Warnf(c.Request.Context(), "[Audit] Failed to save audit log: %v", err)
	}
}

// classifyRequest maps HTTP method + path to audit action/resource.
func classifyRequest(method, path string, params gin.Params) (types.AuditAction, types.AuditResourceType, string) {
	// Determine action from HTTP method
	var action types.AuditAction
	switch method {
	case "POST":
		action = types.AuditActionCreate
	case "PUT", "PATCH":
		action = types.AuditActionUpdate
	case "DELETE":
		action = types.AuditActionDelete
	default:
		action = types.AuditAction(strings.ToLower(method))
	}

	// Override action for special paths
	lowerPath := strings.ToLower(path)
	if strings.Contains(lowerPath, "/login") || strings.Contains(lowerPath, "/callback") {
		action = types.AuditActionLogin
	} else if strings.Contains(lowerPath, "/export") {
		action = types.AuditActionExport
	} else if strings.Contains(lowerPath, "/import") {
		action = types.AuditActionImport
	}

	// Determine resource type from path
	resourceType := inferResourceType(lowerPath)

	// Extract resource ID from common path parameter names
	resourceID := params.ByName("id")
	if resourceID == "" {
		resourceID = params.ByName("kb_id")
	}
	if resourceID == "" {
		resourceID = params.ByName("knowledge_id")
	}

	return action, resourceType, resourceID
}

// inferResourceType maps URL path segments to audit resource types.
func inferResourceType(path string) types.AuditResourceType {
	segments := map[string]types.AuditResourceType{
		"/knowledge-bases/": types.AuditResourceKnowledgeBase,
		"/knowledge/":       types.AuditResourceKnowledge,
		"/knowledges/":      types.AuditResourceKnowledge,
		"/faq/":             types.AuditResourceFAQ,
		"/sessions/":        types.AuditResourceSession,
		"/models/":          types.AuditResourceModel,
		"/tenants/":         types.AuditResourceTenant,
		"/tenant/":          types.AuditResourceTenant,
		"/users/":           types.AuditResourceUser,
		"/auth/":            types.AuditResourceUser,
		"/agents/":          types.AuditResourceAgent,
		"/mcp-services/":    types.AuditResourceMCPService,
		"/im/":              types.AuditResourceIMChannel,
		"/datasources/":     types.AuditResourceDataSource,
	}
	for seg, rt := range segments {
		if strings.Contains(path, seg) {
			return rt
		}
	}
	return types.AuditResourceType("other")
}
