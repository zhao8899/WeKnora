package web_search

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

const (
	// defaultBraveSearchURL is the hardcoded Brave Search API endpoint.
	// Not configurable by tenants — prevents SSRF.
	defaultBraveSearchURL = "https://api.search.brave.com/res/v1/web/search"
)

var defaultBraveTimeout = 15 * time.Second

// BraveProvider implements web search using Brave Search API.
type BraveProvider struct {
	client  *http.Client
	baseURL string
	apiKey  string
}

// NewBraveProvider creates a new Brave Search provider from parameters.
func NewBraveProvider(params types.WebSearchProviderParameters) (interfaces.WebSearchProvider, error) {
	if params.APIKey == "" {
		return nil, fmt.Errorf("API key is required for Brave Search provider")
	}
	return &BraveProvider{
		client:  &http.Client{Timeout: defaultBraveTimeout},
		baseURL: defaultBraveSearchURL,
		apiKey:  params.APIKey,
	}, nil
}

// Name returns the provider name
func (p *BraveProvider) Name() string {
	return "brave"
}

// Search performs a web search using Brave Search API
func (p *BraveProvider) Search(
	ctx context.Context,
	query string,
	maxResults int,
	includeDate bool,
) ([]*types.WebSearchResult, error) {
	if query == "" {
		return nil, fmt.Errorf("query is empty")
	}
	logger.Infof(ctx, "[WebSearch][Brave] query=%q maxResults=%d", query, maxResults)

	params := url.Values{}
	params.Set("q", query)
	params.Set("count", strconv.Itoa(maxResults))

	reqURL := fmt.Sprintf("%s?%s", p.baseURL, params.Encode())
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		logger.Warnf(ctx, "[WebSearch][Brave] API returned status %d: %s", resp.StatusCode, string(body))
		return nil, fmt.Errorf("brave search returned status %d: %s", resp.StatusCode, string(body))
	}

	var respData braveSearchResponse
	if err := json.Unmarshal(body, &respData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	var items []braveWebResult
	if respData.Web != nil {
		items = respData.Web.Results
	}

	results := make([]*types.WebSearchResult, 0, len(items))
	for _, item := range items {
		result := &types.WebSearchResult{
			Title:   item.Title,
			URL:     item.URL,
			Snippet: item.Description,
			Source:  "brave",
		}
		if includeDate && item.PageAge != "" {
			// Brave returns page_age in ISO 8601 format
			if t, err := time.Parse(time.RFC3339, item.PageAge); err == nil {
				result.PublishedAt = &t
			} else if t, err := time.Parse("2006-01-02T15:04:05", item.PageAge); err == nil {
				result.PublishedAt = &t
			}
		}
		results = append(results, result)
	}

	logger.Infof(ctx, "[WebSearch][Brave] returned %d results", len(results))
	return results, nil
}

// braveSearchResponse defines the top-level response structure for Brave Search API.
type braveSearchResponse struct {
	Web *braveWebResults `json:"web"`
}

// braveWebResults wraps the web results array.
type braveWebResults struct {
	Results []braveWebResult `json:"results"`
}

// braveWebResult represents a single web search result from Brave.
type braveWebResult struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
	PageAge     string `json:"page_age,omitempty"`
}
