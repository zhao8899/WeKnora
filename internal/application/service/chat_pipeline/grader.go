package chatpipeline

import (
	"context"
	"fmt"

	"github.com/Tencent/WeKnora/internal/types"
)

// PluginRetrievalGrader implements a CRAG-style retrieval grader that judges
// the quality of retrieved chunks and assigns a verdict (correct / ambiguous /
// incorrect). The verdict is later consumed by INTO_CHAT_MESSAGE to inject a
// context-aware hint into the user prompt, prompting the model to either trust,
// hedge, or fall back when the retrieval signal is weak.
//
// This implementation uses a lightweight score-based heuristic over the top
// candidates produced by the rerank/merge stages. It is free, deterministic,
// and adds no latency. A future iteration can layer an LLM-based grader on top
// of borderline (ambiguous) verdicts for higher precision.
type PluginRetrievalGrader struct{}

// Score thresholds for verdict classification. These operate on the composite
// post-rerank score which is already normalized to [0,1].
const (
	graderCorrectThreshold   = 0.55
	graderIncorrectThreshold = 0.25
)

// NewPluginRetrievalGrader creates and registers the retrieval grader plugin.
func NewPluginRetrievalGrader(eventManager *EventManager) *PluginRetrievalGrader {
	res := &PluginRetrievalGrader{}
	eventManager.Register(res)
	return res
}

// ActivationEvents returns the event types this plugin handles.
func (p *PluginRetrievalGrader) ActivationEvents() []types.EventType {
	return []types.EventType{types.RETRIEVAL_GRADER}
}

// OnEvent classifies the retrieval quality and writes the verdict into
// chatManage.RetrievalVerdict.
func (p *PluginRetrievalGrader) OnEvent(ctx context.Context,
	eventType types.EventType, chatManage *types.ChatManage, next func() *PluginError,
) *PluginError {
	if !chatManage.NeedsRetrieval() {
		return next()
	}

	// Pick the most recently refined result set available.
	candidates := chatManage.MergeResult
	if len(candidates) == 0 {
		candidates = chatManage.RerankResult
	}
	if len(candidates) == 0 {
		candidates = chatManage.SearchResult
	}

	if len(candidates) == 0 {
		chatManage.RetrievalVerdict = types.RetrievalVerdictIncorrect
		pipelineWarn(ctx, "Grader", "verdict", map[string]interface{}{
			"verdict": chatManage.RetrievalVerdict,
			"reason":  "no_candidates",
		})
		return next()
	}

	topScore := candidates[0].Score
	secondScore := 0.0
	if len(candidates) > 1 {
		secondScore = candidates[1].Score
	}

	verdict := types.RetrievalVerdictAmbiguous
	switch {
	case topScore >= graderCorrectThreshold:
		verdict = types.RetrievalVerdictCorrect
	case topScore < graderIncorrectThreshold:
		verdict = types.RetrievalVerdictIncorrect
	}
	chatManage.RetrievalVerdict = verdict

	pipelineInfo(ctx, "Grader", "verdict", map[string]interface{}{
		"verdict":      verdict,
		"top_score":    fmt.Sprintf("%.4f", topScore),
		"second_score": fmt.Sprintf("%.4f", secondScore),
		"candidates":   len(candidates),
	})
	return next()
}
