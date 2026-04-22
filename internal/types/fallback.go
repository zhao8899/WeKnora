package types

import "context"

// FallbackKey identifies a localized fallback message.
type FallbackKey int

const (
	// FallbackEmptyAnswer is shown when the LLM produces no content after retries.
	FallbackEmptyAnswer FallbackKey = iota
	// FallbackNudgeAnswer is sent back to the LLM to prompt it to call final_answer.
	FallbackNudgeAnswer
	// FallbackMaxIterations is shown when the agent exhausts all ReAct iterations.
	FallbackMaxIterations
)

// localizedFallbacks maps FallbackKey → locale prefix → message.
// Locale matching uses the first two characters (e.g. "zh" matches "zh-CN", "zh-TW").
var localizedFallbacks = map[FallbackKey]map[string]string{
	FallbackEmptyAnswer: {
		"zh": "抱歉，我暂时无法生成回答，请稍后重试。",
		"en": "I'm sorry, I was unable to generate a response. Please try again.",
	},
	FallbackNudgeAnswer: {
		"zh": "请通过调用 final_answer 工具提交你的回答。",
		"en": "Please provide your answer by calling the final_answer tool.",
	},
	FallbackMaxIterations: {
		"zh": "抱歉，我未能生成完整的回答，请尝试换一种提问方式。",
		"en": "Sorry, I was unable to generate a complete answer. Please try rephrasing your question.",
	},
}

// LocalizedFallback returns the fallback message for the given key in the
// language stored in ctx, defaulting to Chinese when no match is found.
func LocalizedFallback(ctx context.Context, key FallbackKey) string {
	locale, _ := LanguageFromContext(ctx)
	return LocalizedFallbackForLocale(locale, key)
}

// LocalizedFallbackForLocale returns the fallback message for an explicit locale string.
func LocalizedFallbackForLocale(locale string, key FallbackKey) string {
	msgs, ok := localizedFallbacks[key]
	if !ok {
		return ""
	}
	// Exact match first
	if msg, ok := msgs[locale]; ok {
		return msg
	}
	// Prefix match (e.g. "zh-CN" → "zh")
	if len(locale) >= 2 {
		prefix := locale[:2]
		if msg, ok := msgs[prefix]; ok {
			return msg
		}
	}
	// Default to Chinese
	if msg, ok := msgs["zh"]; ok {
		return msg
	}
	return ""
}
