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
	// defaultSerpAPISearchURL is the hardcoded SerpAPI endpoint.
	// Not configurable by tenants — prevents SSRF.
	defaultSerpAPISearchURL = "https://serpapi.com/search"
)

var defaultSerpAPITimeout = 15 * time.Second

// SerpAPIProvider implements web search using SerpAPI (Google search by default).
type SerpAPIProvider struct {
	client  *http.Client
	baseURL string
	apiKey  string
}

// NewSerpAPIProvider creates a new SerpAPI provider from parameters.
func NewSerpAPIProvider(params types.WebSearchProviderParameters) (interfaces.WebSearchProvider, error) {
	if params.APIKey == "" {
		return nil, fmt.Errorf("API key is required for SerpAPI provider")
	}
	return &SerpAPIProvider{
		client:  &http.Client{Timeout: defaultSerpAPITimeout},
		baseURL: defaultSerpAPISearchURL,
		apiKey:  params.APIKey,
	}, nil
}

// Name returns the provider name
func (p *SerpAPIProvider) Name() string {
	return "serpapi"
}

// Search performs a web search using SerpAPI
func (p *SerpAPIProvider) Search(
	ctx context.Context,
	query string,
	maxResults int,
	includeDate bool,
) ([]*types.WebSearchResult, error) {
	if query == "" {
		return nil, fmt.Errorf("query is empty")
	}
	logger.Infof(ctx, "[WebSearch][SerpAPI] query=%q maxResults=%d", query, maxResults)

	params := url.Values{}
	params.Set("q", query)
	params.Set("api_key", p.apiKey)
	params.Set("engine", "google")
	params.Set("num", strconv.Itoa(maxResults))
	params.Set("hl", "zh-CN")

	reqURL := fmt.Sprintf("%s?%s", p.baseURL, params.Encode())
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

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
		logger.Warnf(ctx, "[WebSearch][SerpAPI] API returned status %d: %s", resp.StatusCode, string(body))
		return nil, fmt.Errorf("serpapi returned status %d: %s", resp.StatusCode, string(body))
	}

	var respData serpAPISearchResponse
	if err := json.Unmarshal(body, &respData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	results := make([]*types.WebSearchResult, 0, len(respData.OrganicResults))
	for _, item := range respData.OrganicResults {
		result := &types.WebSearchResult{
			Title:   item.Title,
			URL:     item.Link,
			Snippet: item.Snippet,
			Source:  "serpapi",
		}
		if includeDate && item.Date != "" {
			if t, err := time.Parse("Jan 2, 2006", item.Date); err == nil {
				result.PublishedAt = &t
			}
		}
		results = append(results, result)
	}

	logger.Infof(ctx, "[WebSearch][SerpAPI] returned %d results", len(results))
	return results, nil
}

type stringOrNumber string

func (s *stringOrNumber) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*s = ""
		return nil
	}

	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		*s = stringOrNumber(str)
		return nil
	}

	var num json.Number
	if err := json.Unmarshal(data, &num); err == nil {
		*s = stringOrNumber(num.String())
		return nil
	}

	return fmt.Errorf("unsupported JSON value for stringOrNumber: %s", string(data))
}

// serpAPISearchResponse defines the response structure for SerpAPI.
type serpAPISearchResponse struct {
	OrganicResults []struct {
		Position int    `json:"position"`
		Title    string `json:"title"`
		Link     string `json:"link"`
		Snippet  string `json:"snippet"`
		Date     string `json:"date,omitempty"`
	} `json:"organic_results"`
	SearchInformation struct {
		TotalResults       stringOrNumber `json:"total_results"`
		TimeTakenDisplayed float64        `json:"time_taken_displayed"`
		QueryDisplayed     string         `json:"query_displayed"`
	} `json:"search_information"`
}
