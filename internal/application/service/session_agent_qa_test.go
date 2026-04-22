package service

import (
	"testing"

	"github.com/Tencent/WeKnora/internal/agent"
	"github.com/Tencent/WeKnora/internal/types"
)

func TestMergeAgentRuntimeConfig_PrefersCustomAgentValues(t *testing.T) {
	customCfg := types.CustomAgentConfig{
		AgentMode:         types.AgentModeSmartReasoning,
		MaxIterations:     12,
		Temperature:       0.6,
		SystemPrompt:      "custom prompt",
		AllowedTools:      []string{"thinking", "todo_write"},
		WebSearchEnabled:  true,
		WebSearchMaxResults: 8,
	}
	tenantCfg := &types.AgentConfig{
		MaxIterations: 20,
		Temperature:   0.3,
		SystemPrompt:  "tenant prompt",
		AllowedTools:  []string{"knowledge_search"},
	}

	merged := mergeAgentRuntimeConfig(customCfg, tenantCfg, true)

	if merged.MaxIterations != 12 {
		t.Fatalf("expected custom max_iterations, got %d", merged.MaxIterations)
	}
	if merged.Temperature != 0.6 {
		t.Fatalf("expected custom temperature, got %v", merged.Temperature)
	}
	if merged.SystemPrompt != "custom prompt" || merged.SystemPromptSource != "custom_agent" {
		t.Fatalf("expected custom system prompt source, got prompt=%q source=%s", merged.SystemPrompt, merged.SystemPromptSource)
	}
	if len(merged.AllowedTools) != 2 || merged.AllowedToolsSource != "custom_agent" {
		t.Fatalf("expected custom allowed tools source, got tools=%v source=%s", merged.AllowedTools, merged.AllowedToolsSource)
	}
	if !merged.MultiTurnEnabled {
		t.Fatal("expected smart-reasoning mode to force multi-turn enabled")
	}
	if !merged.WebSearchEnabled || merged.WebSearchMaxResults != 8 {
		t.Fatalf("expected custom web search config, got enabled=%v max=%d", merged.WebSearchEnabled, merged.WebSearchMaxResults)
	}
}

func TestMergeAgentRuntimeConfig_FallsBackToTenantAgentConfig(t *testing.T) {
	customCfg := types.CustomAgentConfig{
		AgentMode: types.AgentModeSmartReasoning,
	}
	tenantCfg := &types.AgentConfig{
		MaxIterations: 25,
		Temperature:   0.4,
		SystemPrompt:  "tenant prompt",
		AllowedTools:  []string{"query_knowledge_graph"},
	}

	merged := mergeAgentRuntimeConfig(customCfg, tenantCfg, false)

	if merged.MaxIterations != 25 {
		t.Fatalf("expected tenant max_iterations, got %d", merged.MaxIterations)
	}
	if merged.Temperature != 0.4 {
		t.Fatalf("expected tenant temperature, got %v", merged.Temperature)
	}
	if merged.SystemPrompt != "tenant prompt" || merged.SystemPromptSource != "tenant_agent_config" {
		t.Fatalf("expected tenant system prompt source, got prompt=%q source=%s", merged.SystemPrompt, merged.SystemPromptSource)
	}
	if len(merged.AllowedTools) != 1 || merged.AllowedTools[0] != "query_knowledge_graph" {
		t.Fatalf("expected tenant allowed tools, got %v", merged.AllowedTools)
	}
	if merged.AllowedToolsSource != "tenant_agent_config" {
		t.Fatalf("expected tenant allowed tools source, got %s", merged.AllowedToolsSource)
	}
}

func TestMergeAgentRuntimeConfig_FallsBackToSystemDefaults(t *testing.T) {
	merged := mergeAgentRuntimeConfig(types.CustomAgentConfig{}, nil, true)

	if merged.MaxIterations != agent.DefaultAgentMaxIterations {
		t.Fatalf("expected default max_iterations, got %d", merged.MaxIterations)
	}
	if merged.Temperature != agent.DefaultAgentTemperature {
		t.Fatalf("expected default temperature, got %v", merged.Temperature)
	}
	if len(merged.AllowedTools) == 0 || merged.AllowedToolsSource != "default" {
		t.Fatalf("expected default allowed tools source, got tools=%v source=%s", merged.AllowedTools, merged.AllowedToolsSource)
	}
	if merged.SystemPrompt != "" || merged.SystemPromptSource != "default" {
		t.Fatalf("expected default prompt source with empty prompt, got prompt=%q source=%s", merged.SystemPrompt, merged.SystemPromptSource)
	}
	if merged.WebSearchMaxResults != 5 || merged.HistoryTurns != 5 {
		t.Fatalf("expected runtime defaults for web search/history, got web=%d history=%d", merged.WebSearchMaxResults, merged.HistoryTurns)
	}
}
