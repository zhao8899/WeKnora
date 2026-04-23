package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/Tencent/WeKnora/internal/agent"
	"github.com/Tencent/WeKnora/internal/agent/tools"
	llmcontext "github.com/Tencent/WeKnora/internal/application/service/llmcontext"
	"github.com/Tencent/WeKnora/internal/event"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/models/rerank"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// AgentQA performs agent-based question answering with conversation history and streaming support
// customAgent is optional - if provided, uses custom agent configuration instead of tenant defaults
// summaryModelID is optional - if provided, overrides the model from customAgent config
func (s *sessionService) AgentQA(
	ctx context.Context,
	req *types.QARequest,
	eventBus *event.EventBus,
) error {
	sessionID := req.Session.ID
	sessionJSON, err := json.Marshal(req.Session)
	if err != nil {
		logger.Errorf(ctx, "Failed to marshal session, session ID: %s, error: %v", sessionID, err)
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	// customAgent is required for AgentQA (handler has already done permission check for shared agent)
	if req.CustomAgent == nil {
		logger.Warnf(ctx, "Custom agent not provided for session: %s", sessionID)
		return errors.New("custom agent configuration is required for agent QA")
	}

	// Resolve retrieval tenant using shared helper
	agentTenantID := s.resolveRetrievalTenantID(ctx, req)
	logger.Infof(ctx, "Start agent-based question answering, session ID: %s, agent tenant ID: %d, query: %s, session: %s",
		sessionID, agentTenantID, req.Query, string(sessionJSON))

	var tenantInfo *types.Tenant
	if v := ctx.Value(types.TenantInfoContextKey); v != nil {
		tenantInfo, _ = v.(*types.Tenant)
	}
	// When agent belongs to another tenant (shared agent), use agent's tenant for KB/model scope; load tenantInfo if needed
	if tenantInfo == nil || tenantInfo.ID != agentTenantID {
		if s.tenantService != nil {
			if agentTenant, err := s.tenantService.GetTenantByID(ctx, agentTenantID); err == nil && agentTenant != nil {
				tenantInfo = agentTenant
				logger.Infof(ctx, "Using agent tenant info for retrieval scope, tenant ID: %d", agentTenantID)
			}
		}
	}
	if tenantInfo == nil {
		logger.Warnf(ctx, "Tenant info not available for agent tenant %d, proceeding with defaults", agentTenantID)
		tenantInfo = &types.Tenant{ID: agentTenantID}
	}

	// Build AgentConfig from custom agent and tenant info
	// This is the runtime snapshot used for the current request after tenant,
	// shared-agent scope, and request-level switches are merged together.
	agentConfig, err := s.buildAgentConfig(ctx, req, tenantInfo, agentTenantID)
	if err != nil {
		return err
	}
	logger.Infof(ctx, "AgentQA runtime config prepared: max_iterations=%d, kb_count=%d, knowledge_count=%d, web_search=%v, multi_turn=%v",
		agentConfig.MaxIterations, len(agentConfig.KnowledgeBases), len(agentConfig.KnowledgeIDs),
		agentConfig.WebSearchEnabled, agentConfig.MultiTurnEnabled)

	// Set VLM model ID for tool result image analysis (runtime-only field)
	if req.CustomAgent != nil && req.CustomAgent.Config.VLMModelID != "" {
		agentConfig.VLMModelID = req.CustomAgent.Config.VLMModelID
	}

	// Resolve model ID using shared helper (AgentQA requires a model, so error if not found)
	effectiveModelID, err := s.resolveChatModelID(ctx, req, agentConfig.KnowledgeBases, agentConfig.KnowledgeIDs)
	if err != nil {
		return err
	}
	if effectiveModelID == "" {
		logger.Warnf(ctx, "No summary model configured for custom agent %s", req.CustomAgent.ID)
		return errors.New("summary model (model_id) is not configured in custom agent settings")
	}

	summaryModel, err := s.modelService.GetChatModel(ctx, effectiveModelID)
	if err != nil {
		logger.Warnf(ctx, "Failed to get chat model: %v", err)
		return fmt.Errorf("failed to get chat model: %w", err)
	}
	logger.Infof(ctx, "AgentQA selected summary model: model_id=%s", effectiveModelID)

	// Get rerank model from custom agent config (only required when knowledge bases are configured)
	var rerankModel rerank.Reranker
	hasKnowledge := len(agentConfig.KnowledgeBases) > 0 || len(agentConfig.KnowledgeIDs) > 0
	if hasKnowledge {
		rerankModelID := req.CustomAgent.Config.RerankModelID
		if rerankModelID == "" {
			logger.Warnf(ctx, "No rerank model configured for custom agent %s, but knowledge bases are specified", req.CustomAgent.ID)
			return errors.New("rerank model (rerank_model_id) is not configured in custom agent settings")
		}

		rerankModel, err = s.modelService.GetRerankModel(ctx, rerankModelID)
		if err != nil {
			logger.Warnf(ctx, "Failed to get rerank model: %v", err)
			return fmt.Errorf("failed to get rerank model: %w", err)
		}
		logger.Infof(ctx, "AgentQA selected rerank model: model_id=%s", rerankModelID)
	} else {
		logger.Infof(ctx, "No knowledge bases configured, skipping rerank model initialization")
	}

	// Get or create contextManager for this session
	// Agent mode always goes through the context manager so history compression
	// and agent-specific system prompt switching happen in one place.
	contextManager := s.getContextManagerForSession()

	// Set system prompt for the current agent in context manager
	// This ensures the context uses the correct system prompt when switching agents
	systemPrompt := agentConfig.ResolveSystemPrompt(agentConfig.WebSearchEnabled)
	if systemPrompt != "" {
		if err := contextManager.SetSystemPrompt(ctx, sessionID, systemPrompt); err != nil {
			logger.Warnf(ctx, "Failed to set system prompt in context manager: %v", err)
		} else {
			logger.Infof(ctx, "System prompt updated in context manager for agent")
		}
	}

	// Get LLM context from context manager
	llmContext, err := s.getContextForSession(ctx, contextManager, sessionID)
	if err != nil {
		logger.Warnf(ctx, "Failed to get LLM context: %v, continuing without history", err)
		llmContext = []chat.Message{}
	}
	logger.Infof(ctx, "Loaded %d messages from LLM context manager", len(llmContext))

	// Apply multi-turn configuration for Agent mode
	// Note: In Agent mode, context is managed by contextManager with compression strategies,
	// so we don't apply HistoryTurns limit here. HistoryTurns is used in normal (KnowledgeQA) mode.
	if !agentConfig.MultiTurnEnabled {
		// Multi-turn disabled, clear history
		logger.Infof(ctx, "Multi-turn disabled for this agent, clearing history context")
		llmContext = []chat.Message{}
	}

	// Create agent engine with EventBus and ContextManager
	logger.Info(ctx, "Creating agent engine")
	engine, err := s.agentService.CreateAgentEngine(
		ctx,
		agentConfig,
		summaryModel,
		rerankModel,
		eventBus,
		contextManager,
		sessionID,
	)
	if err != nil {
		logger.Errorf(ctx, "Failed to create agent engine: %v", err)
		return err
	}
	logger.Infof(ctx, "Agent engine created successfully: session=%s, allowed_tools=%d, search_targets=%d",
		sessionID, len(agentConfig.AllowedTools), len(agentConfig.SearchTargets))

	// Route image data based on agent model's vision capability
	// Vision-capable models receive raw image URLs directly; text-only models
	// get the precomputed image description appended to the user query.
	var agentModelSupportsVision bool
	if effectiveModelID != "" {
		if modelInfo, err := s.modelService.GetModelByID(ctx, effectiveModelID); err == nil && modelInfo != nil {
			agentModelSupportsVision = modelInfo.Parameters.SupportsVision
		}
	}

	agentQuery := req.Query
	var agentImageURLs []string
	if agentModelSupportsVision && len(req.ImageURLs) > 0 {
		agentImageURLs = req.ImageURLs
		logger.Infof(ctx, "Agent model supports vision, passing %d image(s) directly", len(agentImageURLs))
	} else if req.ImageDescription != "" {
		agentQuery = req.Query + "\n\n[用户上传图片内容]\n" + req.ImageDescription
		logger.Infof(ctx, "Agent model does not support vision, appending image description (%d chars)", len(req.ImageDescription))
	}

	logger.Infof(ctx, "AgentQA execution input ready: query_len=%d, llm_context_msgs=%d, image_count=%d",
		len(agentQuery), len(llmContext), len(agentImageURLs))

	// Execute agent with streaming (asynchronously)
	// Events will be emitted to EventBus and handled by the Handler layer
	logger.Info(ctx, "Executing agent with streaming")
	if _, err := engine.Execute(ctx, sessionID, req.AssistantMessageID, agentQuery, llmContext, agentImageURLs); err != nil {
		logger.Errorf(ctx, "Agent execution failed: %v", err)
		// Emit error event to the EventBus used by this agent
		eventBus.Emit(ctx, event.Event{
			Type:      event.EventError,
			SessionID: sessionID,
			Data: event.ErrorData{
				Error:     err.Error(),
				Stage:     "agent_execution",
				SessionID: sessionID,
			},
		})
	}
	// Return empty - events will be handled by Handler via EventBus subscription
	return nil
}

// buildAgentConfig creates a runtime AgentConfig from the QARequest's custom agent configuration,
// tenant info, and resolved knowledge bases / search targets.
func (s *sessionService) buildAgentConfig(
	ctx context.Context,
	req *types.QARequest,
	tenantInfo *types.Tenant,
	agentTenantID uint64,
) (*types.AgentConfig, error) {
	customAgent := req.CustomAgent
	agentConfig := mergeAgentRuntimeConfig(customAgent.Config, tenantInfo.AgentConfig, req.WebSearchEnabled)

	// Configure skills based on CustomAgentConfig
	s.configureSkillsFromAgent(ctx, agentConfig, customAgent)

	// Resolve knowledge bases using shared helper
	agentConfig.KnowledgeBases, agentConfig.KnowledgeIDs = s.resolveKnowledgeBases(ctx, req)

	logger.Infof(ctx, "Agent runtime config merged: max_iterations=%d, temperature=%.2f, allowed_tools=%v, web_search=%v, tools_source=%s, prompt_source=%s",
		agentConfig.MaxIterations, agentConfig.Temperature, agentConfig.AllowedTools, agentConfig.WebSearchEnabled,
		agentConfig.AllowedToolsSource, agentConfig.SystemPromptSource)

	// Set web search max results from tenant config if not set (default: 5)
	if agentConfig.WebSearchMaxResults == 0 {
		agentConfig.WebSearchMaxResults = 5
		if tenantInfo.WebSearchConfig != nil && tenantInfo.WebSearchConfig.MaxResults > 0 {
			agentConfig.WebSearchMaxResults = tenantInfo.WebSearchConfig.MaxResults
		}
	}

	// Resolve web search provider ID: valid agent-level override > tenant default (is_default=true)
	agentConfig.WebSearchProviderID = s.resolveValidWebSearchProviderID(ctx, tenantInfo.ID, agentConfig.WebSearchProviderID)

	logger.Infof(ctx, "Merged agent config from tenant %d and session %s", tenantInfo.ID, req.Session.ID)

	// Log knowledge bases if present
	if len(agentConfig.KnowledgeBases) > 0 {
		logger.Infof(ctx, "Agent configured with %d knowledge base(s): %v",
			len(agentConfig.KnowledgeBases), agentConfig.KnowledgeBases)
	} else {
		logger.Infof(ctx, "No knowledge bases specified for agent, running in pure agent mode")
	}

	// Build search targets using agent's tenant (handler has validated access for shared agent)
	searchTargets, err := s.buildSearchTargets(ctx, agentTenantID, agentConfig.KnowledgeBases, agentConfig.KnowledgeIDs)
	if err != nil {
		logger.Warnf(ctx, "Failed to build search targets for agent: %v", err)
	}
	agentConfig.SearchTargets = searchTargets
	logger.Infof(ctx, "Agent search targets built: %d targets", len(searchTargets))

	if agentConfig.MaxContextTokens <= 0 {
		agentConfig.MaxContextTokens = types.DefaultMaxContextTokens
	}

	return agentConfig, nil
}

func mergeAgentRuntimeConfig(
	customCfg types.CustomAgentConfig,
	tenantCfg *types.AgentConfig,
	requestWebSearchEnabled bool,
) *types.AgentConfig {
	agentConfig := &types.AgentConfig{
		WebSearchEnabled:            customCfg.WebSearchEnabled && requestWebSearchEnabled,
		WebSearchMaxResults:         customCfg.WebSearchMaxResults,
		WebSearchProviderID:         customCfg.WebSearchProviderID,
		MultiTurnEnabled:            customCfg.MultiTurnEnabled,
		HistoryTurns:                customCfg.HistoryTurns,
		ParallelToolCalls:           customCfg.ParallelToolCalls,
		MaxParallelToolCalls:        customCfg.MaxParallelToolCalls,
		MCPSelectionMode:            customCfg.MCPSelectionMode,
		MCPServices:                 customCfg.MCPServices,
		Thinking:                    customCfg.Thinking,
		RetrieveKBOnlyWhenMentioned: customCfg.RetrieveKBOnlyWhenMentioned,
		AllowedToolsSource:          "default",
		SystemPromptSource:          "default",
	}

	if customCfg.AgentMode == types.AgentModeSmartReasoning {
		agentConfig.MultiTurnEnabled = true
	}

	if customCfg.MaxIterations > 0 {
		agentConfig.MaxIterations = customCfg.MaxIterations
	} else if tenantCfg != nil && tenantCfg.MaxIterations > 0 {
		agentConfig.MaxIterations = tenantCfg.MaxIterations
	} else {
		agentConfig.MaxIterations = agent.DefaultAgentMaxIterations
	}

	if customCfg.Temperature > 0 {
		agentConfig.Temperature = customCfg.Temperature
	} else if tenantCfg != nil && tenantCfg.Temperature >= 0 {
		agentConfig.Temperature = tenantCfg.Temperature
	} else {
		agentConfig.Temperature = agent.DefaultAgentTemperature
	}

	if agentConfig.WebSearchMaxResults == 0 {
		agentConfig.WebSearchMaxResults = 5
	}
	if agentConfig.HistoryTurns == 0 {
		agentConfig.HistoryTurns = 5
	}
	if agentConfig.MaxParallelToolCalls == 0 && tenantCfg != nil && tenantCfg.MaxParallelToolCalls > 0 {
		agentConfig.MaxParallelToolCalls = tenantCfg.MaxParallelToolCalls
	}
	if !agentConfig.ParallelToolCalls && tenantCfg != nil && tenantCfg.ParallelToolCalls {
		agentConfig.ParallelToolCalls = true
	}

	switch {
	case len(customCfg.AllowedTools) > 0:
		agentConfig.AllowedTools = customCfg.AllowedTools
		agentConfig.AllowedToolsSource = "custom_agent"
	case tenantCfg != nil && len(tenantCfg.AllowedTools) > 0:
		agentConfig.AllowedTools = tenantCfg.AllowedTools
		agentConfig.AllowedToolsSource = "tenant_agent_config"
	default:
		agentConfig.AllowedTools = tools.DefaultAllowedTools()
	}

	switch {
	case customCfg.SystemPrompt != "":
		agentConfig.UseCustomSystemPrompt = true
		agentConfig.SystemPrompt = customCfg.SystemPrompt
		agentConfig.SystemPromptSource = "custom_agent"
	case tenantCfg != nil && tenantCfg.ResolveSystemPrompt(agentConfig.WebSearchEnabled) != "":
		agentConfig.UseCustomSystemPrompt = true
		agentConfig.SystemPrompt = tenantCfg.ResolveSystemPrompt(agentConfig.WebSearchEnabled)
		agentConfig.SystemPromptSource = "tenant_agent_config"
	}

	return agentConfig
}

// configureSkillsFromAgent configures skills settings in AgentConfig based on CustomAgentConfig
// Returns the skill directories and allowed skills based on the selection mode:
//   - "all": uses all preloaded skills
//   - "selected": uses the explicitly selected skills
//   - "none" or "": skills are disabled
func (s *sessionService) configureSkillsFromAgent(
	ctx context.Context,
	agentConfig *types.AgentConfig,
	customAgent *types.CustomAgent,
) {
	if customAgent == nil {
		return
	}
	// When sandbox is disabled, skills cannot be enabled (no script execution environment)
	sandboxMode := os.Getenv("WEKNORA_SANDBOX_MODE")
	if sandboxMode == "" || sandboxMode == "disabled" {
		agentConfig.SkillsEnabled = false
		agentConfig.SkillDirs = nil
		agentConfig.AllowedSkills = nil
		logger.Infof(ctx, "Sandbox is disabled: skills are not available")
		return
	}

	switch customAgent.Config.SkillsSelectionMode {
	case "all":
		// Enable all preloaded skills
		agentConfig.SkillsEnabled = true
		agentConfig.SkillDirs = []string{DefaultPreloadedSkillsDir}
		agentConfig.AllowedSkills = nil // Empty means all skills allowed
		logger.Infof(ctx, "SkillsSelectionMode=all: enabled all preloaded skills")
	case "selected":
		// Enable only selected skills
		if len(customAgent.Config.SelectedSkills) > 0 {
			agentConfig.SkillsEnabled = true
			agentConfig.SkillDirs = []string{DefaultPreloadedSkillsDir}
			agentConfig.AllowedSkills = customAgent.Config.SelectedSkills
			logger.Infof(ctx, "SkillsSelectionMode=selected: enabled %d selected skills: %v",
				len(customAgent.Config.SelectedSkills), customAgent.Config.SelectedSkills)
		} else {
			agentConfig.SkillsEnabled = false
			logger.Infof(ctx, "SkillsSelectionMode=selected but no skills selected: skills disabled")
		}
	case "none", "":
		// Skills disabled
		agentConfig.SkillsEnabled = false
		logger.Infof(ctx, "SkillsSelectionMode=%s: skills disabled", customAgent.Config.SkillsSelectionMode)
	default:
		// Unknown mode, disable skills
		agentConfig.SkillsEnabled = false
		logger.Warnf(ctx, "Unknown SkillsSelectionMode=%s: skills disabled", customAgent.Config.SkillsSelectionMode)
	}

}

// getContextManagerForSession creates a context manager for the session.
func (s *sessionService) getContextManagerForSession() interfaces.ContextManager {
	return llmcontext.NewContextManagerFromConfig(s.sessionStorage, s.messageRepo)
}

// getContextForSession retrieves LLM context for a session
func (s *sessionService) getContextForSession(
	ctx context.Context,
	contextManager interfaces.ContextManager,
	sessionID string,
) ([]chat.Message, error) {
	history, err := contextManager.GetContext(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get context: %w", err)
	}

	// Log context statistics
	stats, _ := contextManager.GetContextStats(ctx, sessionID)
	if stats != nil {
		logger.Infof(ctx, "LLM context stats for session %s: messages=%d, tokens=~%d, compressed=%v",
			sessionID, stats.MessageCount, stats.TokenCount, stats.IsCompressed)
	}

	return history, nil
}
