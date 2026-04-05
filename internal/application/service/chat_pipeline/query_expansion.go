package chatpipeline

import (
	"context"
	"regexp"
	"strings"
	"sync"
	"unicode"

	"github.com/Tencent/WeKnora/internal/types"
)

// runQueryExpansion performs query expansion when initial recall is low.
// It generates query variants and runs concurrent retrieval across search targets.
func (p *PluginSearch) runQueryExpansion(ctx context.Context, chatManage *types.ChatManage) []*types.SearchResult {
	pipelineInfo(ctx, "Search", "recall_low", map[string]interface{}{
		"current":   len(chatManage.SearchResult),
		"threshold": chatManage.EmbeddingTopK,
	})
	expansions := p.expandQueries(ctx, chatManage)
	if len(expansions) == 0 {
		return nil
	}

	pipelineInfo(ctx, "Search", "expansion_start", map[string]interface{}{
		"variants": len(expansions),
	})
	expTopK := max(chatManage.EmbeddingTopK*2, chatManage.RerankTopK*2)
	expKwTh := chatManage.KeywordThreshold * 0.8

	// Concurrent expansion retrieval across queries and search targets
	expResults := make([]*types.SearchResult, 0, expTopK*len(expansions))
	var muExp sync.Mutex
	var wgExp sync.WaitGroup
	jobs := len(expansions) * len(chatManage.SearchTargets)
	capSem := 16
	if jobs < capSem {
		capSem = jobs
	}
	if capSem <= 0 {
		capSem = 1
	}
	sem := make(chan struct{}, capSem)
	pipelineInfo(ctx, "Search", "expansion_concurrency", map[string]interface{}{
		"jobs": jobs,
		"cap":  capSem,
	})
	for _, q := range expansions {
		for _, target := range chatManage.SearchTargets {
			wgExp.Add(1)
			go func(q string, t *types.SearchTarget) {
				defer wgExp.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				paramsExp := types.SearchParams{
					QueryText:             q,
					VectorThreshold:       chatManage.VectorThreshold,
					KeywordThreshold:      expKwTh,
					MatchCount:            expTopK,
					DisableVectorMatch:    true,  // Expansion targets keyword recall; vector search already ran on the original query
					DisableKeywordsMatch:  false,
					SkipContextEnrichment: true, // Pipeline handles context assembly in merge stage
				}
				// Apply knowledge ID filter if this is a partial KB search
				if t.Type == types.SearchTargetTypeKnowledge {
					paramsExp.KnowledgeIDs = t.KnowledgeIDs
				}
				res, err := p.knowledgeBaseService.HybridSearch(ctx, t.KnowledgeBaseID, paramsExp)
				if err != nil {
					pipelineWarn(ctx, "Search", "expansion_error", map[string]interface{}{
						"kb_id": t.KnowledgeBaseID,
						"error": err.Error(),
					})
					return
				}
				if len(res) > 0 {
					for _, r := range res {
						r.KnowledgeBaseID = t.KnowledgeBaseID
					}
					pipelineInfo(ctx, "Search", "expansion_hits", map[string]interface{}{
						"kb_id": t.KnowledgeBaseID,
						"query": q,
						"hits":  len(res),
					})
					muExp.Lock()
					expResults = append(expResults, res...)
					muExp.Unlock()
				}
			}(q, target)
		}
	}
	wgExp.Wait()

	if len(expResults) > 0 {
		pipelineInfo(ctx, "Search", "expansion_done", map[string]interface{}{
			"added": len(expResults),
		})
	}
	return expResults
}

// expandQueries generates query variants locally without LLM to improve keyword recall.
// Uses simple techniques: word reordering, stopword removal, key phrase extraction.
func (p *PluginSearch) expandQueries(ctx context.Context, chatManage *types.ChatManage) []string {
	query := strings.TrimSpace(chatManage.RewriteQuery)
	if query == "" {
		return nil
	}

	expansions := make([]string, 0, 5)
	seen := make(map[string]struct{})
	seen[strings.ToLower(query)] = struct{}{}
	if q := strings.ToLower(chatManage.Query); q != "" {
		seen[q] = struct{}{}
	}

	addIfNew := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || len(s) < 3 {
			return
		}
		key := strings.ToLower(s)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		expansions = append(expansions, s)
	}

	// 1. Remove common stopwords and create keyword-only variant
	keywords := extractKeywords(query)
	if len(keywords) >= 2 {
		addIfNew(strings.Join(keywords, " "))
	}

	// 2. Extract quoted phrases or key segments
	phrases := extractPhrases(query)
	for _, phrase := range phrases {
		addIfNew(phrase)
	}

	// 3. Split by common delimiters and use longest segment
	segments := splitByDelimiters(query)
	for _, seg := range segments {
		if len(seg) > 5 {
			addIfNew(seg)
		}
	}

	// 4. Remove question words (什么/如何/怎么/为什么/哪个 etc.)
	cleaned := removeQuestionWords(query)
	if cleaned != query {
		addIfNew(cleaned)
	}

	// Limit to 5 expansions
	if len(expansions) > 5 {
		expansions = expansions[:5]
	}

	pipelineInfo(ctx, "Search", "local_expansion_result", map[string]interface{}{
		"variants": len(expansions),
	})
	return expansions
}

// Common Chinese and English stopwords
var stopwords = map[string]struct{}{
	"的": {}, "是": {}, "在": {}, "了": {}, "和": {}, "与": {}, "或": {},
	"a": {}, "an": {}, "the": {}, "is": {}, "are": {}, "was": {}, "were": {},
	"be": {}, "been": {}, "being": {}, "have": {}, "has": {}, "had": {},
	"do": {}, "does": {}, "did": {}, "will": {}, "would": {}, "could": {},
	"should": {}, "may": {}, "might": {}, "must": {}, "can": {},
	"to": {}, "of": {}, "in": {}, "for": {}, "on": {}, "with": {}, "at": {},
	"by": {}, "from": {}, "as": {}, "into": {}, "through": {}, "about": {},
	"what": {}, "how": {}, "why": {}, "when": {}, "where": {}, "which": {},
	"who": {}, "whom": {}, "whose": {},
}

// Question words in Chinese
var questionWords = regexp.MustCompile(`^(什么是|什么|如何|怎么|怎样|为什么|为何|哪个|哪些|谁|何时|何地|请问|请告诉我|帮我|我想知道|我想了解)`)

func extractKeywords(text string) []string {
	words := tokenize(text)
	keywords := make([]string, 0, len(words))
	for _, w := range words {
		lower := strings.ToLower(w)
		if _, isStop := stopwords[lower]; !isStop && len(w) > 1 {
			keywords = append(keywords, w)
		}
	}
	return keywords
}

// reQuotedPhrase matches content within various quotation marks (Chinese and Western).
var reQuotedPhrase = regexp.MustCompile(`["'"'「」『』]([^"'"'「」『』]+)["'"'「」『』]`)

// reSplitDelimiters splits text by common Chinese/Western punctuation and whitespace.
var reSplitDelimiters = regexp.MustCompile(`[,，;；、。！？!?\s]+`)

func extractPhrases(text string) []string {
	// Extract quoted content
	var phrases []string
	matches := reQuotedPhrase.FindAllStringSubmatch(text, -1)
	for _, m := range matches {
		if len(m) > 1 && len(m[1]) > 2 {
			phrases = append(phrases, m[1])
		}
	}
	return phrases
}

func splitByDelimiters(text string) []string {
	parts := reSplitDelimiters.Split(text, -1)
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func removeQuestionWords(text string) string {
	return strings.TrimSpace(questionWords.ReplaceAllString(text, ""))
}

func tokenize(text string) []string {
	var tokens []string
	var current strings.Builder

	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			current.WriteRune(r)
		} else if unicode.Is(unicode.Han, r) {
			// Flush current token
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
			// Chinese character as single token
			tokens = append(tokens, string(r))
		} else {
			// Delimiter
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
		}
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}
	return tokens
}
