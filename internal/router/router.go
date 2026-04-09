package router

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	filesvc "github.com/Tencent/WeKnora/internal/application/service/file"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"go.uber.org/dig"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/Tencent/WeKnora/internal/handler/session"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"

	_ "github.com/Tencent/WeKnora/docs" // swagger docs
)

// RouterParams 路由参数
type RouterParams struct {
	dig.In

	Config                   *config.Config
	UserService              interfaces.UserService
	KBService                interfaces.KnowledgeBaseService
	KnowledgeService         interfaces.KnowledgeService
	ChunkService             interfaces.ChunkService
	SessionService           interfaces.SessionService
	MessageService           interfaces.MessageService
	ModelService             interfaces.ModelService
	EvaluationService        interfaces.EvaluationService
	KBHandler                *handler.KnowledgeBaseHandler
	KnowledgeHandler         *handler.KnowledgeHandler
	TenantHandler            *handler.TenantHandler
	TenantService            interfaces.TenantService
	ChunkHandler             *handler.ChunkHandler
	SessionHandler           *session.Handler
	MessageHandler           *handler.MessageHandler
	ModelHandler             *handler.ModelHandler
	EvaluationHandler        *handler.EvaluationHandler
	AuthHandler              *handler.AuthHandler
	InitializationHandler    *handler.InitializationHandler
	SystemHandler            *handler.SystemHandler
	MCPServiceHandler        *handler.MCPServiceHandler
	WebSearchHandler         *handler.WebSearchHandler
	WebSearchProviderHandler *handler.WebSearchProviderHandler
	FAQHandler               *handler.FAQHandler
	TagHandler               *handler.TagHandler
	CustomAgentHandler       *handler.CustomAgentHandler
	SkillHandler             *handler.SkillHandler
	OrganizationHandler      *handler.OrganizationHandler
	IMHandler                *handler.IMHandler
	DataSourceHandler        *handler.DataSourceHandler
	DB                       *gorm.DB
	RedisClient              *redis.Client `optional:"true"`
}

// NewRouter 创建新的路由
func NewRouter(params RouterParams) *gin.Engine {
	r := gin.New()
	r.ContextWithFallback = true

	// CORS 中间件应放在最前面
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "X-API-Key", "X-Request-ID"},
		ExposeHeaders:    []string{"Content-Length", "Access-Control-Allow-Origin"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// 基础中间件（不需要认证）
	r.Use(middleware.RequestID())
	r.Use(middleware.Language())
	r.Use(middleware.Logger())
	r.Use(middleware.Recovery())
	r.Use(middleware.ErrorHandler())

	// 健康检查（不需要认证）
	r.GET("/health", params.SystemHandler.GetHealth)

	// Swagger API 文档（仅在非生产环境下启用）
	// 通过 GIN_MODE 环境变量判断：release 模式下禁用 Swagger
	if gin.Mode() != gin.ReleaseMode {
		r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler,
			ginSwagger.DefaultModelsExpandDepth(-1), // 默认折叠 Models
			ginSwagger.DocExpansion("list"),         // 展开模式: "list"(展开标签), "full"(全部展开), "none"(全部折叠)
			ginSwagger.DeepLinking(true),            // 启用深度链接
			ginSwagger.PersistAuthorization(true),   // 持久化认证信息
		))
	}

	// 前端静态文件（仅 Lite 版本内嵌前端）
	if handler.Edition == "lite" {
		serveFrontendStatic(r)
	}

	// IM 回调路由（在认证中间件之前注册，使用各平台自身的签名验证）
	RegisterIMRoutes(r, params.IMHandler)

	// 认证中间件
	r.Use(middleware.Auth(params.TenantService, params.UserService, params.Config))

	// API 全局限流中间件（滑动窗口，默认 100req/60s per tenant）
	r.Use(middleware.APIRateLimit(params.RedisClient))

	// 审计日志中间件（记录认证后的写操作）
	if params.DB != nil {
		r.Use(middleware.Audit(params.DB))
	}

	// 文件服务：统一代理本地/MinIO/COS/TOS存储后端（需要认证）
	serveFiles(r)

	// 添加OpenTelemetry追踪中间件
	// r.Use(middleware.TracingMiddleware())

	// 需要认证的API路由
	v1 := r.Group("/api/v1")
	{
		RegisterAuthRoutes(v1, params.AuthHandler)
		RegisterTenantRoutes(v1, params.TenantHandler)
		RegisterKnowledgeBaseRoutes(v1, params.KBHandler)
		RegisterKnowledgeTagRoutes(v1, params.TagHandler)
		RegisterKnowledgeRoutes(v1, params.KnowledgeHandler)
		RegisterFAQRoutes(v1, params.FAQHandler)
		RegisterChunkRoutes(v1, params.ChunkHandler)
		RegisterSessionRoutes(v1, params.SessionHandler)
		RegisterChatRoutes(v1, params.SessionHandler)
		RegisterMessageRoutes(v1, params.MessageHandler)
		RegisterModelRoutes(v1, params.ModelHandler)
		RegisterEvaluationRoutes(v1, params.EvaluationHandler)
		RegisterInitializationRoutes(v1, params.InitializationHandler)
		RegisterSystemRoutes(v1, params.SystemHandler)
		RegisterMCPServiceRoutes(v1, params.MCPServiceHandler)
		RegisterWebSearchRoutes(v1, params.WebSearchHandler)
		RegisterWebSearchProviderRoutes(v1, params.WebSearchProviderHandler)
		RegisterCustomAgentRoutes(v1, params.CustomAgentHandler)
		RegisterSkillRoutes(v1, params.SkillHandler)
		RegisterOrganizationRoutes(v1, params.OrganizationHandler)
		RegisterIMChannelRoutes(v1, params.IMHandler)
		RegisterDataSourceRoutes(v1, params.DataSourceHandler)
	}

	return r
}

// RegisterChunkRoutes 注册分块相关的路由
func RegisterChunkRoutes(r *gin.RouterGroup, handler *handler.ChunkHandler) {
	requireEditor := middleware.RequireRole(types.OrgRoleEditor)

	// 分块路由组
	chunks := r.Group("/chunks")
	{
		// 获取分块列表
		chunks.GET("/:knowledge_id", handler.ListKnowledgeChunks)
		// 通过chunk_id获取单个chunk（不需要knowledge_id）
		chunks.GET("/by-id/:id", handler.GetChunkByIDOnly)
		// 删除分块（需要编辑者以上权限）
		chunks.DELETE("/:knowledge_id/:id", requireEditor, handler.DeleteChunk)
		// 删除知识下的所有分块（需要编辑者以上权限）
		chunks.DELETE("/:knowledge_id", requireEditor, handler.DeleteChunksByKnowledgeID)
		// 更新分块信息（需要编辑者以上权限）
		chunks.PUT("/:knowledge_id/:id", requireEditor, handler.UpdateChunk)
		// 删除单个生成的问题（需要编辑者以上权限）
		chunks.DELETE("/by-id/:id/questions", requireEditor, handler.DeleteGeneratedQuestion)
	}
}

// RegisterKnowledgeRoutes 注册知识相关的路由
func RegisterKnowledgeRoutes(r *gin.RouterGroup, handler *handler.KnowledgeHandler) {
	requireEditor := middleware.RequireRole(types.OrgRoleEditor)
	requireAdmin := middleware.RequireRole(types.OrgRoleAdmin)

	// 知识库下的知识路由组
	kb := r.Group("/knowledge-bases/:id/knowledge")
	{
		// 从文件创建知识（需要编辑者以上权限）
		kb.POST("/file", requireEditor, handler.CreateKnowledgeFromFile)
		// 从URL创建知识（需要编辑者以上权限）
		kb.POST("/url", requireEditor, handler.CreateKnowledgeFromURL)
		// 手工 Markdown 录入（需要编辑者以上权限）
		kb.POST("/manual", requireEditor, handler.CreateManualKnowledge)
		// 获取知识库下的知识列表
		kb.GET("", handler.ListKnowledge)
		// 清空知识库下的所有知识（需要管理员权限）
		kb.DELETE("", requireAdmin, handler.ClearKnowledgeBaseContents)
	}

	// 知识路由组
	k := r.Group("/knowledge")
	{
		// 批量获取知识
		k.GET("/batch", handler.GetKnowledgeBatch)
		// 获取知识详情
		k.GET("/:id", handler.GetKnowledge)
		// 删除知识（需要编辑者以上权限）
		k.DELETE("/:id", requireEditor, handler.DeleteKnowledge)
		// 更新知识（需要编辑者以上权限）
		k.PUT("/:id", requireEditor, handler.UpdateKnowledge)
		// 更新手工 Markdown 知识（需要编辑者以上权限）
		k.PUT("/manual/:id", requireEditor, handler.UpdateManualKnowledge)
		// 重新解析知识（需要编辑者以上权限）
		k.POST("/:id/reparse", requireEditor, handler.ReparseKnowledge)
		// 获取知识文件
		k.GET("/:id/download", handler.DownloadKnowledgeFile)
		// 预览知识文件（内联显示，返回正确 Content-Type）
		k.GET("/:id/preview", handler.PreviewKnowledgeFile)
		// 更新图像分块信息（需要编辑者以上权限）
		k.PUT("/image/:id/:chunk_id", requireEditor, handler.UpdateImageInfo)
		// 批量更新知识标签（需要编辑者以上权限）
		k.PUT("/tags", requireEditor, handler.UpdateKnowledgeTagBatch)
		// 搜索知识
		k.GET("/search", handler.SearchKnowledge)
		// 移动知识到其他知识库（需要编辑者以上权限）
		k.POST("/move", requireEditor, handler.MoveKnowledge)
		// 获取知识移动进度
		k.GET("/move/progress/:task_id", handler.GetKnowledgeMoveProgress)
	}
}

// RegisterFAQRoutes 注册 FAQ 相关路由
func RegisterFAQRoutes(r *gin.RouterGroup, handler *handler.FAQHandler) {
	if handler == nil {
		return
	}
	requireEditor := middleware.RequireRole(types.OrgRoleEditor)

	faq := r.Group("/knowledge-bases/:id/faq")
	{
		faq.GET("/entries", handler.ListEntries)
		faq.GET("/entries/export", handler.ExportEntries)
		faq.GET("/entries/:entry_id", handler.GetEntry)
		faq.POST("/entries", requireEditor, handler.UpsertEntries)
		faq.POST("/entry", requireEditor, handler.CreateEntry)
		faq.PUT("/entries/:entry_id", requireEditor, handler.UpdateEntry)
		faq.POST("/entries/:entry_id/similar-questions", requireEditor, handler.AddSimilarQuestions)
		// Unified batch update API - supports is_enabled, is_recommended, tag_id
		faq.PUT("/entries/fields", requireEditor, handler.UpdateEntryFieldsBatch)
		faq.PUT("/entries/tags", requireEditor, handler.UpdateEntryTagBatch)
		faq.DELETE("/entries", requireEditor, handler.DeleteEntries)
		faq.POST("/search", handler.SearchFAQ)
		// FAQ import result display status
		faq.PUT("/import/last-result/display", requireEditor, handler.UpdateLastImportResultDisplayStatus)
	}
	// FAQ import progress route (outside of knowledge-base scope)
	faqImport := r.Group("/faq/import")
	{
		faqImport.GET("/progress/:task_id", handler.GetImportProgress)
	}
}

// RegisterKnowledgeBaseRoutes 注册知识库相关的路由
func RegisterKnowledgeBaseRoutes(r *gin.RouterGroup, handler *handler.KnowledgeBaseHandler) {
	requireEditor := middleware.RequireRole(types.OrgRoleEditor)
	requireAdmin := middleware.RequireRole(types.OrgRoleAdmin)

	// 知识库路由组
	kb := r.Group("/knowledge-bases")
	{
		// 创建知识库（需要编辑者以上权限）
		kb.POST("", requireEditor, handler.CreateKnowledgeBase)
		// 获取知识库列表
		kb.GET("", handler.ListKnowledgeBases)
		// 获取知识库详情
		kb.GET("/:id", handler.GetKnowledgeBase)
		// 更新知识库（需要编辑者以上权限，handler 层做资源级细粒度校验）
		kb.PUT("/:id", requireEditor, handler.UpdateKnowledgeBase)
		// 删除知识库（需要管理员权限，handler 层校验所有者）
		kb.DELETE("/:id", requireAdmin, handler.DeleteKnowledgeBase)
		// 置顶/取消置顶知识库
		kb.PUT("/:id/pin", handler.TogglePinKnowledgeBase)
		// 混合搜索
		kb.GET("/:id/hybrid-search", handler.HybridSearch)
		// 拷贝知识库（需要编辑者以上权限）
		kb.POST("/copy", requireEditor, handler.CopyKnowledgeBase)
		// 获取知识库复制进度
		kb.GET("/copy/progress/:task_id", handler.GetKBCloneProgress)
		// 获取可移动目标知识库列表
		kb.GET("/:id/move-targets", handler.ListMoveTargets)
	}
}

// RegisterKnowledgeTagRoutes 注册知识库标签相关路由
func RegisterKnowledgeTagRoutes(r *gin.RouterGroup, tagHandler *handler.TagHandler) {
	if tagHandler == nil {
		return
	}
	kbTags := r.Group("/knowledge-bases/:id/tags")
	{
		kbTags.GET("", tagHandler.ListTags)
		kbTags.POST("", tagHandler.CreateTag)
		kbTags.PUT("/:tag_id", tagHandler.UpdateTag)
		kbTags.DELETE("/:tag_id", tagHandler.DeleteTag)
	}
}

// RegisterMessageRoutes 注册消息相关的路由
func RegisterMessageRoutes(r *gin.RouterGroup, handler *handler.MessageHandler) {
	// 消息路由组
	messages := r.Group("/messages")
	{
		// 搜索历史对话（关键词 + 向量混合搜索）
		messages.POST("/search", handler.SearchMessages)
		// 获取聊天历史知识库的统计信息
		messages.GET("/chat-history-stats", handler.GetChatHistoryKBStats)
		// 加载更早的消息，用于向上滚动加载
		messages.GET("/:session_id/load", handler.LoadMessages)
		// 删除消息
		messages.DELETE("/:session_id/:id", handler.DeleteMessage)
	}
}

// RegisterSessionRoutes 注册路由
func RegisterSessionRoutes(r *gin.RouterGroup, handler *session.Handler) {
	sessions := r.Group("/sessions")
	{
		sessions.POST("", handler.CreateSession)
		sessions.DELETE("/batch", handler.BatchDeleteSessions)
		sessions.GET("/:id", handler.GetSession)
		sessions.GET("", handler.GetSessionsByTenant)
		sessions.PUT("/:id", handler.UpdateSession)
		sessions.DELETE("/:id", handler.DeleteSession)
		sessions.DELETE("/:id/messages", handler.ClearSessionMessages)
		sessions.POST("/:session_id/generate_title", handler.GenerateTitle)
		sessions.POST("/:session_id/stop", handler.StopSession)
		// 继续接收活跃流
		sessions.GET("/continue-stream/:session_id", handler.ContinueStream)
	}
}

// RegisterChatRoutes 注册路由
func RegisterChatRoutes(r *gin.RouterGroup, handler *session.Handler) {
	knowledgeChat := r.Group("/knowledge-chat")
	{
		knowledgeChat.POST("/:session_id", handler.KnowledgeQA)
	}

	// Agent-based chat
	agentChat := r.Group("/agent-chat")
	{
		agentChat.POST("/:session_id", handler.AgentQA)
	}

	// 新增知识检索接口，不需要session_id
	knowledgeSearch := r.Group("/knowledge-search")
	{
		knowledgeSearch.POST("", handler.SearchKnowledge)
	}
}

// RegisterTenantRoutes 注册租户相关的路由
func RegisterTenantRoutes(r *gin.RouterGroup, handler *handler.TenantHandler) {
	requireAdmin := middleware.RequireRole(types.OrgRoleAdmin)
	requireSuperAdmin := middleware.RequireSuperAdmin()

	// 添加获取所有租户的路由（需要跨租户权限）
	r.GET("/tenants/all", requireSuperAdmin, handler.ListAllTenants)
	// 添加搜索租户的路由（需要跨租户权限，支持分页和搜索）
	r.GET("/tenants/search", requireSuperAdmin, handler.SearchTenants)
	// 租户路由组
	tenantRoutes := r.Group("/tenants")
	{
		tenantRoutes.POST("", requireSuperAdmin, handler.CreateTenant)
		tenantRoutes.GET("/:id", handler.GetTenant)
		tenantRoutes.PUT("/:id", requireAdmin, handler.UpdateTenant)
		tenantRoutes.DELETE("/:id", requireSuperAdmin, handler.DeleteTenant)
		tenantRoutes.GET("", handler.ListTenants)

		// Generic KV configuration management (tenant-level, 租户管理员可操作)
		tenantRoutes.GET("/kv/:key", handler.GetTenantKV)
		tenantRoutes.PUT("/kv/:key", requireAdmin, handler.UpdateTenantKV)
	}
}

// RegisterModelRoutes 注册模型相关的路由
func RegisterModelRoutes(r *gin.RouterGroup, handler *handler.ModelHandler) {
	requireAdmin := middleware.RequireRole(types.OrgRoleAdmin)

	// 模型路由组
	models := r.Group("/models")
	{
		// 获取模型厂商列表
		models.GET("/providers", handler.ListModelProviders)
		// 创建模型（仅管理员）
		models.POST("", requireAdmin, handler.CreateModel)
		// 获取模型列表
		models.GET("", handler.ListModels)
		// 获取单个模型
		models.GET("/:id", handler.GetModel)
		// 更新模型（仅管理员）
		models.PUT("/:id", requireAdmin, handler.UpdateModel)
		// 删除模型（仅管理员）
		models.DELETE("/:id", requireAdmin, handler.DeleteModel)
	}

	// Platform model management (super-admin only)
	platform := models.Group("/platform")
	platform.Use(middleware.RequireSuperAdmin())
	{
		platform.POST("", handler.CreatePlatformModel)
		platform.GET("", handler.ListPlatformModels)
		platform.PUT("/:id", handler.UpdatePlatformModel)
		platform.DELETE("/:id", handler.DeletePlatformModel)
	}
}

func RegisterEvaluationRoutes(r *gin.RouterGroup, handler *handler.EvaluationHandler) {
	evaluationRoutes := r.Group("/evaluation")
	{
		evaluationRoutes.POST("/", handler.Evaluation)
		evaluationRoutes.GET("/", handler.GetEvaluationResult)
	}
}

// RegisterAuthRoutes registers authentication routes
func RegisterAuthRoutes(r *gin.RouterGroup, handler *handler.AuthHandler) {
	r.POST("/auth/register", handler.Register)
	r.POST("/auth/login", handler.Login)
	r.GET("/auth/oidc/config", handler.GetOIDCConfig)
	r.GET("/auth/oidc/url", handler.GetOIDCAuthorizationURL)
	r.GET("/auth/oidc/callback", handler.OIDCRedirectCallback)
	r.POST("/auth/refresh", handler.RefreshToken)
	r.GET("/auth/validate", handler.ValidateToken)
	r.POST("/auth/logout", handler.Logout)
	r.GET("/auth/me", handler.GetCurrentUser)
	r.POST("/auth/change-password", handler.ChangePassword)
}

func RegisterInitializationRoutes(r *gin.RouterGroup, handler *handler.InitializationHandler) {
	requireEditor := middleware.RequireRole(types.OrgRoleEditor)

	// 初始化接口
	r.GET("/initialization/config/:kbId", handler.GetCurrentConfigByKB)
	r.POST("/initialization/initialize/:kbId", requireEditor, handler.InitializeByKB)
	r.PUT("/initialization/config/:kbId", requireEditor, handler.UpdateKBConfig) // 新的简化版接口，只传模型ID

	// Ollama相关接口
	r.GET("/initialization/ollama/status", handler.CheckOllamaStatus)
	r.GET("/initialization/ollama/models", handler.ListOllamaModels)
	r.POST("/initialization/ollama/models/check", handler.CheckOllamaModels)
	r.POST("/initialization/ollama/models/download", handler.DownloadOllamaModel)
	r.GET("/initialization/ollama/download/progress/:taskId", handler.GetDownloadProgress)
	r.GET("/initialization/ollama/download/tasks", handler.ListDownloadTasks)

	// 远程API相关接口
	r.POST("/initialization/remote/check", handler.CheckRemoteModel)
	r.POST("/initialization/embedding/test", handler.TestEmbeddingModel)
	r.POST("/initialization/rerank/check", handler.CheckRerankModel)
	r.POST("/initialization/asr/check", handler.CheckASRModel)
	r.POST("/initialization/multimodal/test", handler.TestMultimodalFunction)

	r.POST("/initialization/extract/text-relation", handler.ExtractTextRelations)
	r.POST("/initialization/extract/fabri-tag", handler.FabriTag)
	r.POST("/initialization/extract/fabri-text", handler.FabriText)
}

// RegisterSystemRoutes registers system information routes
// 系统级路由仅超级管理员可访问，暴露平台基础设施详情
func RegisterSystemRoutes(r *gin.RouterGroup, handler *handler.SystemHandler) {
	systemRoutes := r.Group("/system")
	systemRoutes.Use(middleware.RequireSuperAdmin())
	{
		systemRoutes.GET("/info", handler.GetSystemInfo)
		systemRoutes.GET("/diagnostics", handler.GetDiagnostics)
		systemRoutes.GET("/parser-engines", handler.ListParserEngines)
		systemRoutes.POST("/parser-engines/check", handler.CheckParserEngines)
		systemRoutes.POST("/docreader/reconnect", handler.ReconnectDocReader)
		systemRoutes.GET("/storage-engine-status", handler.GetStorageEngineStatus)
		systemRoutes.POST("/storage-engine-check", handler.CheckStorageEngine)
		systemRoutes.GET("/minio/buckets", handler.ListMinioBuckets)
	}
}

// RegisterMCPServiceRoutes registers MCP service routes
func RegisterMCPServiceRoutes(r *gin.RouterGroup, handler *handler.MCPServiceHandler) {
	requireAdmin := middleware.RequireRole(types.OrgRoleAdmin)

	mcpServices := r.Group("/mcp-services")
	{
		// Create MCP service（需要管理员权限）
		mcpServices.POST("", requireAdmin, handler.CreateMCPService)
		// List MCP services
		mcpServices.GET("", handler.ListMCPServices)
		// Get MCP service by ID
		mcpServices.GET("/:id", handler.GetMCPService)
		// Update MCP service（需要管理员权限）
		mcpServices.PUT("/:id", requireAdmin, handler.UpdateMCPService)
		// Delete MCP service（需要管理员权限）
		mcpServices.DELETE("/:id", requireAdmin, handler.DeleteMCPService)
		// Test MCP service connection（需要管理员权限）
		mcpServices.POST("/:id/test", requireAdmin, handler.TestMCPService)
		// Get MCP service tools
		mcpServices.GET("/:id/tools", handler.GetMCPServiceTools)
		// Get MCP service resources
		mcpServices.GET("/:id/resources", handler.GetMCPServiceResources)
	}
}

// RegisterWebSearchRoutes registers web search routes
func RegisterWebSearchRoutes(r *gin.RouterGroup, webSearchHandler *handler.WebSearchHandler) {
	// Web search providers
	webSearch := r.Group("/web-search")
	{
		// Get available providers
		webSearch.GET("/providers", webSearchHandler.GetProviders)
	}
}

// RegisterWebSearchProviderRoutes registers CRUD routes for web search provider configurations
func RegisterWebSearchProviderRoutes(r *gin.RouterGroup, h *handler.WebSearchProviderHandler) {
	requireAdmin := middleware.RequireRole(types.OrgRoleAdmin)

	providers := r.Group("/web-search-providers")
	{
		// List available provider types (metadata for UI forms)
		providers.GET("/types", h.ListProviderTypes)
		// Test with raw credentials (no persistence)（需要管理员权限）
		providers.POST("/test", requireAdmin, h.TestProviderRaw)
		// CRUD（写操作需要管理员权限）
		providers.POST("", requireAdmin, h.CreateProvider)
		providers.GET("", h.ListProviders)
		providers.GET("/:id", h.GetProvider)
		providers.PUT("/:id", requireAdmin, h.UpdateProvider)
		providers.DELETE("/:id", requireAdmin, h.DeleteProvider)
		// Test existing saved provider（需要管理员权限）
		providers.POST("/:id/test", requireAdmin, h.TestProviderByID)
	}
}

// RegisterCustomAgentRoutes registers custom agent routes
func RegisterCustomAgentRoutes(r *gin.RouterGroup, agentHandler *handler.CustomAgentHandler) {
	requireEditor := middleware.RequireRole(types.OrgRoleEditor)

	agents := r.Group("/agents")
	{
		// Get placeholder definitions (must be before /:id to avoid conflict)
		agents.GET("/placeholders", agentHandler.GetPlaceholders)
		// Create custom agent（需要编辑者以上权限）
		agents.POST("", requireEditor, agentHandler.CreateAgent)
		// List all agents (including built-in)
		agents.GET("", agentHandler.ListAgents)
		// Get agent by ID
		agents.GET("/:id", agentHandler.GetAgent)
		// Update agent（需要编辑者以上权限）
		agents.PUT("/:id", requireEditor, agentHandler.UpdateAgent)
		// Delete agent（需要编辑者以上权限）
		agents.DELETE("/:id", requireEditor, agentHandler.DeleteAgent)
		// Copy agent（需要编辑者以上权限）
		agents.POST("/:id/copy", requireEditor, agentHandler.CopyAgent)
	}
	// Registered outside the group to avoid Gin route conflict with /agents/:id/shares in organization routes
	r.GET("/agents/:id/suggested-questions", agentHandler.GetSuggestedQuestions)
}

// RegisterSkillRoutes registers skill routes
func RegisterSkillRoutes(r *gin.RouterGroup, skillHandler *handler.SkillHandler) {
	skills := r.Group("/skills")
	{
		// List all preloaded skills
		skills.GET("", skillHandler.ListSkills)
	}
}

// RegisterOrganizationRoutes registers organization and sharing routes
func RegisterOrganizationRoutes(r *gin.RouterGroup, orgHandler *handler.OrganizationHandler) {
	// Organization routes
	orgs := r.Group("/organizations")
	{
		// Create organization
		orgs.POST("", orgHandler.CreateOrganization)
		// List my organizations
		orgs.GET("", orgHandler.ListMyOrganizations)
		// Preview organization by invite code (without joining)
		orgs.GET("/preview/:code", orgHandler.PreviewByInviteCode)
		// Join organization by invite code
		orgs.POST("/join", orgHandler.JoinByInviteCode)
		// Submit join request (for organizations that require approval)
		orgs.POST("/join-request", orgHandler.SubmitJoinRequest)
		// Search searchable (discoverable) organizations
		orgs.GET("/search", orgHandler.SearchOrganizations)
		// Join searchable organization by ID (no invite code)
		orgs.POST("/join-by-id", orgHandler.JoinByOrganizationID)
		// Get organization by ID
		orgs.GET("/:id", orgHandler.GetOrganization)
		// Update organization
		orgs.PUT("/:id", orgHandler.UpdateOrganization)
		// Delete organization
		orgs.DELETE("/:id", orgHandler.DeleteOrganization)
		// Leave organization
		orgs.POST("/:id/leave", orgHandler.LeaveOrganization)
		// Request role upgrade (for existing members)
		orgs.POST("/:id/request-upgrade", orgHandler.RequestRoleUpgrade)
		// Generate invite code
		orgs.POST("/:id/invite-code", orgHandler.GenerateInviteCode)
		// Search users for invite (admin only)
		orgs.GET("/:id/search-users", orgHandler.SearchUsersForInvite)
		// Invite member directly (admin only)
		orgs.POST("/:id/invite", orgHandler.InviteMember)
		// List members
		orgs.GET("/:id/members", orgHandler.ListMembers)
		// Update member role
		orgs.PUT("/:id/members/:user_id", orgHandler.UpdateMemberRole)
		// Remove member
		orgs.DELETE("/:id/members/:user_id", orgHandler.RemoveMember)
		// List join requests (admin only)
		orgs.GET("/:id/join-requests", orgHandler.ListJoinRequests)
		// Review join request (admin only)
		orgs.PUT("/:id/join-requests/:request_id/review", orgHandler.ReviewJoinRequest)
		// List knowledge bases shared to this organization
		orgs.GET("/:id/shares", orgHandler.ListOrgShares)
		// List agents shared to this organization
		orgs.GET("/:id/agent-shares", orgHandler.ListOrgAgentShares)
		// List all knowledge bases in this organization (including mine) for list-page space view
		orgs.GET("/:id/shared-knowledge-bases", orgHandler.ListOrganizationSharedKnowledgeBases)
		// List all agents in this organization (including mine) for list-page space view
		orgs.GET("/:id/shared-agents", orgHandler.ListOrganizationSharedAgents)
	}

	// Knowledge base sharing routes (add to existing kb routes)
	kbShares := r.Group("/knowledge-bases/:id/shares")
	{
		// Share knowledge base
		kbShares.POST("", orgHandler.ShareKnowledgeBase)
		// List shares
		kbShares.GET("", orgHandler.ListKBShares)
		// Update share permission
		kbShares.PUT("/:share_id", orgHandler.UpdateSharePermission)
		// Remove share
		kbShares.DELETE("/:share_id", orgHandler.RemoveShare)
	}

	// Agent sharing routes
	agentShares := r.Group("/agents/:id/shares")
	{
		agentShares.POST("", orgHandler.ShareAgent)
		agentShares.GET("", orgHandler.ListAgentShares)
		agentShares.DELETE("/:share_id", orgHandler.RemoveAgentShare)
	}

	// Shared knowledge bases route
	r.GET("/shared-knowledge-bases", orgHandler.ListSharedKnowledgeBases)
	// Shared agents route
	r.GET("/shared-agents", orgHandler.ListSharedAgents)
	r.POST("/shared-agents/disabled", orgHandler.SetSharedAgentDisabledByMe)
}

// RegisterIMRoutes registers IM callback routes.
// These are registered BEFORE auth middleware since IM platforms use their own signature verification.
func RegisterIMRoutes(r *gin.Engine, imHandler *handler.IMHandler) {
	im := r.Group("/api/v1/im")
	{
		im.GET("/callback/:channel_id", imHandler.IMCallback)
		im.POST("/callback/:channel_id", imHandler.IMCallback)
	}
}

// RegisterIMChannelRoutes registers IM channel CRUD routes (requires authentication).
func RegisterIMChannelRoutes(r *gin.RouterGroup, imHandler *handler.IMHandler) {
	requireAdmin := middleware.RequireRole(types.OrgRoleAdmin)

	// Channel CRUD under agents（IM渠道管理需要管理员权限）
	agentChannels := r.Group("/agents/:id/im-channels")
	{
		agentChannels.POST("", requireAdmin, imHandler.CreateIMChannel)
		agentChannels.GET("", imHandler.ListIMChannels)
	}

	// Channel operations by channel ID（需要管理员权限）
	channels := r.Group("/im-channels")
	{
		channels.PUT("/:id", requireAdmin, imHandler.UpdateIMChannel)
		channels.DELETE("/:id", requireAdmin, imHandler.DeleteIMChannel)
		channels.POST("/:id/toggle", requireAdmin, imHandler.ToggleIMChannel)
	}
}

// serveFrontendStatic registers a middleware that serves the frontend SPA
// from the ./web directory if it exists. Must be called BEFORE auth middleware
// so static files are served without authentication.
func serveFrontendStatic(r *gin.Engine) {
	webDir := os.Getenv("WEKNORA_WEB_DIR")
	if webDir == "" {
		webDir = "./web"
	}
	absDir, _ := filepath.Abs(webDir)
	indexPath := filepath.Join(absDir, "index.html")
	if _, err := os.Stat(indexPath); err != nil {
		return
	}

	logger.Infof(context.Background(), "[Router] Serving frontend static files from %s", absDir)

	fs := http.Dir(absDir)
	fileServer := http.FileServer(fs)

	r.Use(func(c *gin.Context) {
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			c.Next()
			return
		}
		path := c.Request.URL.Path
		if strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/health") || strings.HasPrefix(path, "/swagger/") {
			c.Next()
			return
		}
		fullPath := filepath.Join(absDir, path)
		if info, err := os.Stat(fullPath); err == nil && !info.IsDir() {
			fileServer.ServeHTTP(c.Writer, c.Request)
			c.Abort()
			return
		}
		c.File(indexPath)
		c.Abort()
	})
}

// serveFiles serves files via query parameters and tenant storage settings.
// It is registered after auth middleware, so tenant context comes from authentication.
//
// Route:
//   - /files?file_path=<provider://...>
func serveFiles(r *gin.Engine) {
	baseDir := os.Getenv("LOCAL_STORAGE_BASE_DIR")
	if baseDir == "" {
		baseDir = "/data/files"
	}
	absDir, _ := filepath.Abs(baseDir)
	if info, err := os.Stat(absDir); err != nil || !info.IsDir() {
		if err := os.MkdirAll(absDir, 0o755); err != nil {
			logger.Warnf(context.Background(), "[Router] Cannot create local storage dir %s: %v", absDir, err)
		}
	}

	logger.Infof(context.Background(), "[Router] Serving files from /files (local base: %s)", absDir)

	r.GET("/files", func(c *gin.Context) {
		filePath := strings.TrimSpace(c.Query("file_path"))
		if filePath == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing required parameter: file_path"})
			return
		}
		if strings.Contains(filePath, "..") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid file path"})
			return
		}

		provider := types.ParseProviderScheme(filePath)

		tenant, _ := c.Request.Context().Value(types.TenantInfoContextKey).(*types.Tenant)
		if tenant == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized: tenant context missing"})
			return
		}

		fileSvc, resolvedProvider, err := filesvc.NewFileServiceFromStorageConfig(provider, tenant.StorageEngineConfig, absDir)
		if err != nil {
			logger.Warnf(context.Background(), "[Router] /files resolve file service failed: tenant_id=%d provider=%s err=%v", tenant.ID, provider, err)
			c.Status(http.StatusBadRequest)
			return
		}

		reader, err := fileSvc.GetFile(c.Request.Context(), filePath)
		if err != nil {
			logger.Warnf(context.Background(), "[Router] /files get file failed: tenant_id=%d provider=%s path=%q err=%v", tenant.ID, resolvedProvider, filePath, err)
			c.Status(http.StatusNotFound)
			return
		}
		defer reader.Close()

		ext := filepath.Ext(filePath)
		contentType := "application/octet-stream"
		switch strings.ToLower(ext) {
		case ".png":
			contentType = "image/png"
		case ".jpg", ".jpeg":
			contentType = "image/jpeg"
		case ".gif":
			contentType = "image/gif"
		case ".webp":
			contentType = "image/webp"
		case ".bmp":
			contentType = "image/bmp"
		case ".svg":
			contentType = "image/svg+xml"
		case ".pdf":
			contentType = "application/pdf"
		case ".csv":
			contentType = "text/csv; charset=utf-8"
		}

		c.Header("Content-Type", contentType)
		c.Header("Cache-Control", "public, max-age=86400")
		c.Status(http.StatusOK)
		if _, err := io.Copy(c.Writer, reader); err != nil {
			logger.Warnf(context.Background(), "[Router] /files write response failed: %v", err)
		}
	})
}

// RegisterDataSourceRoutes 注册数据源相关的路由
func RegisterDataSourceRoutes(r *gin.RouterGroup, handler *handler.DataSourceHandler) {
	requireAdmin := middleware.RequireRole(types.OrgRoleAdmin)

	// Data source routes
	ds := r.Group("/datasource")
	{
		// Get available connector types
		ds.GET("/types", handler.GetAvailableConnectors)

		// Validate credentials without persistence (for "Test Connection" button)（需要管理员权限）
		ds.POST("/validate-credentials", requireAdmin, handler.ValidateCredentials)

		// CRUD operations（写操作需要管理员权限）
		ds.POST("", requireAdmin, handler.CreateDataSource)
		ds.GET("", handler.ListDataSources)
		ds.GET("/:id", handler.GetDataSource)
		ds.PUT("/:id", requireAdmin, handler.UpdateDataSource)
		ds.DELETE("/:id", requireAdmin, handler.DeleteDataSource)

		// Connection and resource management（需要管理员权限）
		ds.POST("/:id/validate", requireAdmin, handler.ValidateConnection)
		ds.GET("/:id/resources", handler.ListAvailableResources)

		// Sync management（需要管理员权限）
		ds.POST("/:id/sync", requireAdmin, handler.ManualSync)
		ds.POST("/:id/pause", requireAdmin, handler.PauseDataSource)
		ds.POST("/:id/resume", requireAdmin, handler.ResumeDataSource)

		// Sync logs
		ds.GET("/:id/logs", handler.GetSyncLogs)
		ds.GET("/logs/:log_id", handler.GetSyncLog)
	}
}
