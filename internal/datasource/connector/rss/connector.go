package rss

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	htmltomd "github.com/JohannesKaufmann/html-to-markdown/v2"

	"github.com/Tencent/WeKnora/internal/types"
	secutils "github.com/Tencent/WeKnora/internal/utils"
)

const defaultUserAgent = "WeKnora-RSS/1.0"

type Connector struct {
	client *http.Client
}

func NewConnector() *Connector {
	return &Connector{
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Connector) Type() string {
	return types.ConnectorTypeRSS
}

func (c *Connector) Validate(ctx context.Context, config *types.DataSourceConfig) error {
	urls, err := feedURLs(config)
	if err != nil {
		return err
	}
	if len(urls) == 0 {
		return fmt.Errorf("rss: feed_urls is required")
	}

	for _, feedURL := range urls {
		if err := secutils.ValidateURLForSSRF(feedURL); err != nil {
			return fmt.Errorf("rss: invalid feed url %q: %w", feedURL, err)
		}
		if _, err := c.fetchFeed(ctx, feedURL, userAgent(config)); err != nil {
			return fmt.Errorf("rss: fetch %q: %w", feedURL, err)
		}
	}

	return nil
}

func (c *Connector) ListResources(ctx context.Context, config *types.DataSourceConfig) ([]types.Resource, error) {
	urls, err := feedURLs(config)
	if err != nil {
		return nil, err
	}

	resources := make([]types.Resource, 0, len(urls))
	for _, feedURL := range urls {
		feed, err := c.fetchFeed(ctx, feedURL, userAgent(config))
		if err != nil {
			return nil, err
		}

		name := feed.Title
		if name == "" {
			name = feedURL
		}

		resources = append(resources, types.Resource{
			ExternalID: urlHash(feedURL),
			Name:       name,
			Type:       "feed",
			Description: strings.TrimSpace(firstNonEmpty(
				feed.Description,
				fmt.Sprintf("%d entries", len(feed.Items)),
			)),
			URL:        feedURL,
			ModifiedAt: latestFeedTime(feed),
			Metadata: map[string]interface{}{
				"feed_url":    feedURL,
				"feed_type":   feed.Kind,
				"entry_count": len(feed.Items),
			},
		})
	}

	return resources, nil
}

func (c *Connector) FetchAll(ctx context.Context, config *types.DataSourceConfig, resourceIDs []string) ([]types.FetchedItem, error) {
	urls, err := feedURLs(config)
	if err != nil {
		return nil, err
	}

	allowed := make(map[string]bool, len(resourceIDs))
	for _, id := range resourceIDs {
		allowed[id] = true
	}

	var items []types.FetchedItem
	for _, feedURL := range urls {
		feedID := urlHash(feedURL)
		if len(allowed) > 0 && !allowed[feedID] {
			continue
		}

		feed, err := c.fetchFeed(ctx, feedURL, userAgent(config))
		if err != nil {
			return nil, err
		}

		for _, entry := range feed.Items {
			items = append(items, c.entryToFetchedItem(feedURL, entry))
		}
	}

	return items, nil
}

func (c *Connector) FetchIncremental(ctx context.Context, config *types.DataSourceConfig, cursor *types.SyncCursor) ([]types.FetchedItem, *types.SyncCursor, error) {
	urls, err := feedURLs(config)
	if err != nil {
		return nil, nil, err
	}

	prevHeaders, prevEntries := parseCursorState(cursor)
	currentHeaders := make(map[string]interface{}, len(urls))
	currentEntries := make(map[string]interface{}, len(urls))

	var changed []types.FetchedItem
	for _, feedURL := range urls {
		feedID := urlHash(feedURL)
		etag, lastModified, _ := c.headURL(ctx, feedURL, userAgent(config))
		headerKey := strings.TrimSpace(etag) + "|" + strings.TrimSpace(lastModified)
		if headerKey != "|" {
			currentHeaders[feedID] = headerKey
		}

		feed, err := c.fetchFeed(ctx, feedURL, userAgent(config))
		if err != nil {
			return nil, nil, err
		}

		prevFeedEntries := prevEntries[feedID]
		nextFeedEntries := make(map[string]string, len(feed.Items))

		if prevFeedEntries == nil {
			prevFeedEntries = map[string]string{}
		}

		if headerKey != "|" && prevHeaders[feedID] == headerKey {
			for _, entry := range feed.Items {
				nextFeedEntries[entry.ExternalID] = entry.StateKey()
			}
			currentEntries[feedID] = nextFeedEntries
			continue
		}

		for _, entry := range feed.Items {
			stateKey := entry.StateKey()
			nextFeedEntries[entry.ExternalID] = stateKey
			if prevFeedEntries[entry.ExternalID] != stateKey {
				changed = append(changed, c.entryToFetchedItem(feedURL, entry))
			}
		}
		currentEntries[feedID] = nextFeedEntries
	}

	nextCursor := &types.SyncCursor{
		LastSyncTime: time.Now(),
		ConnectorCursor: map[string]interface{}{
			"feed_headers": currentHeaders,
			"feed_entries": currentEntries,
		},
	}

	return changed, nextCursor, nil
}

type parsedFeed struct {
	Title       string
	Description string
	Kind        string
	Items       []parsedEntry
}

type parsedEntry struct {
	ExternalID string
	Title      string
	Link       string
	Content    string
	Summary    string
	UpdatedAt  time.Time
}

func (e parsedEntry) StateKey() string {
	return sha256hex([]byte(e.Title + "|" + e.Link + "|" + e.Content + "|" + e.Summary + "|" + e.UpdatedAt.UTC().Format(time.RFC3339Nano)))
}

func (c *Connector) fetchFeed(ctx context.Context, feedURL, ua string) (*parsedFeed, error) {
	if err := secutils.ValidateURLForSSRF(feedURL); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return nil, err
	}
	if ua != "" {
		req.Header.Set("User-Agent", ua)
		req.Header.Set("Accept", "application/rss+xml, application/atom+xml, application/xml, text/xml;q=0.9")
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	feed, err := parseFeed(body)
	if err != nil {
		return nil, err
	}
	return feed, nil
}

func (c *Connector) headURL(ctx context.Context, feedURL, ua string) (etag, lastModified string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, feedURL, nil)
	if err != nil {
		return "", "", err
	}
	if ua != "" {
		req.Header.Set("User-Agent", ua)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	return resp.Header.Get("ETag"), resp.Header.Get("Last-Modified"), nil
}

func (c *Connector) entryToFetchedItem(feedURL string, entry parsedEntry) types.FetchedItem {
	title := strings.TrimSpace(firstNonEmpty(entry.Title, entry.Link, entry.ExternalID, "untitled"))
	content := strings.TrimSpace(entry.Content)
	if content == "" {
		content = strings.TrimSpace(entry.Summary)
	}
	if markdown, err := htmltomd.ConvertString(content); err == nil && strings.TrimSpace(markdown) != "" {
		content = strings.TrimSpace(markdown)
	}
	if content == "" {
		content = title
	}

	return types.FetchedItem{
		ExternalID:  entry.ExternalID,
		Title:       title,
		Content:     []byte(content),
		ContentType: "text/markdown",
		FileName:    safeFilename(title) + ".md",
		URL:         entry.Link,
		UpdatedAt:   entry.UpdatedAt,
		Metadata: map[string]string{
			"feed_url": feedURL,
		},
		SourceResourceID: urlHash(feedURL),
	}
}

func parseFeed(data []byte) (*parsedFeed, error) {
	type rssGUID struct {
		Value string `xml:",chardata"`
	}
	type rssItem struct {
		Title          string  `xml:"title"`
		Link           string  `xml:"link"`
		Description    string  `xml:"description"`
		ContentEncoded string  `xml:"http://purl.org/rss/1.0/modules/content/ encoded"`
		GUID           rssGUID `xml:"guid"`
		PubDate        string  `xml:"pubDate"`
	}
	type rssChannel struct {
		Title       string    `xml:"title"`
		Description string    `xml:"description"`
		Items       []rssItem `xml:"item"`
	}
	type rssDoc struct {
		Channel rssChannel `xml:"channel"`
	}

	var rss rssDoc
	if err := xml.Unmarshal(data, &rss); err == nil && (rss.Channel.Title != "" || len(rss.Channel.Items) > 0) {
		items := make([]parsedEntry, 0, len(rss.Channel.Items))
		for _, item := range rss.Channel.Items {
			link := strings.TrimSpace(item.Link)
			externalID := firstNonEmpty(strings.TrimSpace(item.GUID.Value), link, urlHash(item.Title+"|"+link))
			items = append(items, parsedEntry{
				ExternalID: externalID,
				Title:      strings.TrimSpace(item.Title),
				Link:       link,
				Content:    strings.TrimSpace(firstNonEmpty(item.ContentEncoded, item.Description)),
				Summary:    strings.TrimSpace(item.Description),
				UpdatedAt:  parseFeedTime(item.PubDate),
			})
		}
		return &parsedFeed{
			Title:       strings.TrimSpace(rss.Channel.Title),
			Description: strings.TrimSpace(rss.Channel.Description),
			Kind:        "rss",
			Items:       items,
		}, nil
	}

	type atomLink struct {
		Href string `xml:"href,attr"`
		Rel  string `xml:"rel,attr"`
	}
	type atomText struct {
		Type  string `xml:"type,attr"`
		Value string `xml:",innerxml"`
	}
	type atomEntry struct {
		ID        string     `xml:"id"`
		Title     string     `xml:"title"`
		Summary   atomText   `xml:"summary"`
		Content   atomText   `xml:"content"`
		Updated   string     `xml:"updated"`
		Published string     `xml:"published"`
		Links     []atomLink `xml:"link"`
	}
	type atomFeed struct {
		Title    string      `xml:"title"`
		Subtitle atomText    `xml:"subtitle"`
		Entries  []atomEntry `xml:"entry"`
	}

	var atom atomFeed
	if err := xml.Unmarshal(data, &atom); err == nil && (atom.Title != "" || len(atom.Entries) > 0) {
		items := make([]parsedEntry, 0, len(atom.Entries))
		for _, entry := range atom.Entries {
			link := ""
			for _, candidate := range entry.Links {
				if candidate.Rel == "" || candidate.Rel == "alternate" {
					link = strings.TrimSpace(candidate.Href)
					break
				}
			}
			updatedAt := parseFeedTime(firstNonEmpty(entry.Updated, entry.Published))
			externalID := firstNonEmpty(strings.TrimSpace(entry.ID), link, urlHash(entry.Title+"|"+link))
			items = append(items, parsedEntry{
				ExternalID: externalID,
				Title:      strings.TrimSpace(entry.Title),
				Link:       link,
				Content:    strings.TrimSpace(entry.Content.Value),
				Summary:    strings.TrimSpace(entry.Summary.Value),
				UpdatedAt:  updatedAt,
			})
		}
		return &parsedFeed{
			Title:       strings.TrimSpace(atom.Title),
			Description: strings.TrimSpace(atom.Subtitle.Value),
			Kind:        "atom",
			Items:       items,
		}, nil
	}

	return nil, fmt.Errorf("rss: unsupported feed format")
}

func latestFeedTime(feed *parsedFeed) time.Time {
	var latest time.Time
	for _, item := range feed.Items {
		if item.UpdatedAt.After(latest) {
			latest = item.UpdatedAt
		}
	}
	return latest
}

func feedURLs(config *types.DataSourceConfig) ([]string, error) {
	if config == nil || config.Settings == nil {
		return nil, nil
	}
	raw, ok := config.Settings["feed_urls"]
	if !ok {
		raw = config.Settings["urls"]
	}

	var urls []string
	switch v := raw.(type) {
	case []string:
		urls = append(urls, v...)
	case []interface{}:
		for _, item := range v {
			if s, ok := item.(string); ok {
				urls = append(urls, s)
			}
		}
	case string:
		if strings.TrimSpace(v) != "" {
			urls = append(urls, strings.TrimSpace(v))
		}
	}

	seen := map[string]bool{}
	result := make([]string, 0, len(urls))
	for _, item := range urls {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		result = append(result, trimmed)
	}

	return result, nil
}

func userAgent(config *types.DataSourceConfig) string {
	if config == nil || config.Settings == nil {
		return defaultUserAgent
	}
	if value, ok := config.Settings["user_agent"].(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return defaultUserAgent
}

func parseCursorState(cursor *types.SyncCursor) (map[string]string, map[string]map[string]string) {
	headers := map[string]string{}
	entries := map[string]map[string]string{}
	if cursor == nil || cursor.ConnectorCursor == nil {
		return headers, entries
	}

	if rawHeaders, ok := cursor.ConnectorCursor["feed_headers"].(map[string]interface{}); ok {
		for k, v := range rawHeaders {
			if s, ok := v.(string); ok {
				headers[k] = s
			}
		}
	}
	if rawEntries, ok := cursor.ConnectorCursor["feed_entries"].(map[string]interface{}); ok {
		for feedID, rawMap := range rawEntries {
			stateMap := map[string]string{}
			if concrete, ok := rawMap.(map[string]interface{}); ok {
				for k, v := range concrete {
					if s, ok := v.(string); ok {
						stateMap[k] = s
					}
				}
			}
			entries[feedID] = stateMap
		}
	}

	return headers, entries
}

func parseFeedTime(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Now()
	}
	layouts := []string{
		time.RFC3339,
		time.RFC3339Nano,
		time.RFC1123Z,
		time.RFC1123,
		time.RFC822Z,
		time.RFC822,
		time.RFC850,
		time.ANSIC,
		time.UnixDate,
		time.RubyDate,
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed
		}
	}
	return time.Now()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func urlHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:8])
}

func sha256hex(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func safeFilename(value string) string {
	replacer := strings.NewReplacer("/", "-", "\\", "-", ":", "-", "*", "-", "?", "-", `"`, "-", "<", "-", ">", "-", "|", "-")
	result := replacer.Replace(value)
	if result == "" {
		result = "untitled"
	}
	if len(result) > 100 {
		result = result[:100]
	}
	return result
}
