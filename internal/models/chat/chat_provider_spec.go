package chat

import (
	"context"
	"strings"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/models/provider"
	"github.com/sashabaranov/go-openai"
)

// ProviderSpec describes provider-specific behavior for chat completions.
// Each spec is registered with a ProviderName and optionally a model matcher.
type ProviderSpec struct {
	Provider provider.ProviderName
	// ModelMatcher: if non-nil, this spec only applies when the model name matches.
	// Used for sub-provider routing such as Qwen3 within Aliyun.
	ModelMatcher func(modelName string) bool
	// RequestCustomizer: provider-specific request modification.
	RequestCustomizer func(req *openai.ChatCompletionRequest, opts *ChatOptions, isStream bool) (any, bool)
	// EndpointCustomizer: provider-specific endpoint URL override.
	EndpointCustomizer func(baseURL string, modelID string, isStream bool) string
}

// chatProviderSpecs is the ordered list of provider specs.
// Order matters: more specific specs should come before generic ones.
var chatProviderSpecs = []ProviderSpec{
	{
		Provider:          provider.ProviderAliyun,
		ModelMatcher:      func(name string) bool { return provider.IsQwenThinkingModel(name) },
		RequestCustomizer: qwenThinkingRequestCustomizer,
	},
	{
		Provider:          provider.ProviderLKEAP,
		RequestCustomizer: lkeapRequestCustomizer,
	},
	{
		Provider:          provider.ProviderDeepSeek,
		RequestCustomizer: deepseekRequestCustomizer,
	},
	{
		Provider:          provider.ProviderMoonshot,
		ModelMatcher:      provider.IsFixedTemperatureModel,
		RequestCustomizer: moonshotRequestCustomizer,
	},
	{
		Provider:          provider.ProviderGeneric,
		RequestCustomizer: genericRequestCustomizer,
	},
	{
		Provider:          provider.ProviderVolcengine,
		RequestCustomizer: volcengineRequestCustomizer,
	},
	{
		Provider:          provider.ProviderNvidia,
		RequestCustomizer: genericRequestCustomizer,
	},
}

// findProviderSpec finds the matching spec for the given provider and model name.
func findProviderSpec(providerName provider.ProviderName, modelName string) *ProviderSpec {
	for i := range chatProviderSpecs {
		spec := &chatProviderSpecs[i]
		if spec.Provider != providerName {
			continue
		}
		if spec.ModelMatcher != nil && !spec.ModelMatcher(modelName) {
			continue
		}
		return spec
	}
	return nil
}

// QwenChatCompletionRequest adds the enable_thinking field required by some
// Aliyun Qwen models.
type QwenChatCompletionRequest struct {
	openai.ChatCompletionRequest
	EnableThinking *bool `json:"enable_thinking,omitempty"`
}

// ThinkingConfig models provider-specific thinking configuration payloads.
type ThinkingConfig struct {
	Type string `json:"type"`
}

// ThinkingChatCompletionRequest carries a nested thinking config for providers
// such as LKEAP and Volcengine.
type ThinkingChatCompletionRequest struct {
	openai.ChatCompletionRequest
	Thinking *ThinkingConfig `json:"thinking,omitempty"`
}

func qwenThinkingRequestCustomizer(
	req *openai.ChatCompletionRequest, opts *ChatOptions, isStream bool,
) (any, bool) {
	if !isStream {
		qwenReq := QwenChatCompletionRequest{
			ChatCompletionRequest: *req,
		}
		enableThinking := false
		qwenReq.EnableThinking = &enableThinking
		return qwenReq, true
	}

	qwenReq := QwenChatCompletionRequest{
		ChatCompletionRequest: *req,
	}
	thinking := false
	if opts != nil && opts.Thinking != nil {
		thinking = *opts.Thinking
	}
	qwenReq.EnableThinking = &thinking

	// Use raw HTTP so the SDK does not drop the custom field.
	return qwenReq, true
}

// lkeapRequestCustomizer only applies thinking configuration to DeepSeek V3.x
// models routed through LKEAP.
func lkeapRequestCustomizer(
	req *openai.ChatCompletionRequest, opts *ChatOptions, _ bool,
) (any, bool) {
	modelName := req.Model
	if !strings.Contains(strings.ToLower(modelName), "deepseek-v3") || opts == nil || opts.Thinking == nil {
		return nil, false
	}

	lkeapReq := ThinkingChatCompletionRequest{
		ChatCompletionRequest: *req,
	}

	thinkingType := "disabled"
	if *opts.Thinking {
		thinkingType = "enabled"
	}
	lkeapReq.Thinking = &ThinkingConfig{Type: thinkingType}

	return lkeapReq, true
}

// deepseekRequestCustomizer removes unsupported tool_choice for DeepSeek.
func deepseekRequestCustomizer(
	req *openai.ChatCompletionRequest, opts *ChatOptions, _ bool,
) (any, bool) {
	if opts != nil && opts.ToolChoice != "" {
		logger.Infof(context.Background(), "deepseek model, skip tool_choice")
		req.ToolChoice = nil
	}
	return nil, false
}

// moonshotRequestCustomizer normalizes parameters for Moonshot models with
// fixed temperature constraints.
func moonshotRequestCustomizer(
	req *openai.ChatCompletionRequest, _ *ChatOptions, _ bool,
) (any, bool) {
	if req.Temperature != 1 {
		logger.Infof(context.Background(), "moonshot model %s enforces temperature=1, overriding %v", req.Model, req.Temperature)
	}
	req.Temperature = 1
	return nil, false
}

// genericRequestCustomizer uses chat_template_kwargs to pass thinking flags to
// generic OpenAI-compatible providers such as vLLM.
func genericRequestCustomizer(
	req *openai.ChatCompletionRequest, opts *ChatOptions, _ bool,
) (any, bool) {
	thinking := false
	if opts != nil && opts.Thinking != nil {
		thinking = *opts.Thinking
	}
	req.ChatTemplateKwargs = map[string]interface{}{
		"enable_thinking": thinking,
	}
	return req, true
}

// volcengineRequestCustomizer uses the nested thinking payload format required
// by Volcengine Ark.
func volcengineRequestCustomizer(
	req *openai.ChatCompletionRequest, opts *ChatOptions, _ bool,
) (any, bool) {
	if opts == nil || opts.Thinking == nil {
		return nil, false
	}

	vcReq := ThinkingChatCompletionRequest{
		ChatCompletionRequest: *req,
	}

	thinkingType := "disabled"
	if *opts.Thinking {
		thinkingType = "enabled"
	}
	vcReq.Thinking = &ThinkingConfig{Type: thinkingType}

	return vcReq, true
}
