package rss

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

func TestValidateReachableFeed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(sampleRSS))
	}))
	defer server.Close()

	connector := NewConnector()
	config := &types.DataSourceConfig{
		Settings: map[string]interface{}{
			"feed_urls": []string{server.URL},
		},
	}

	if err := connector.Validate(context.Background(), config); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestFetchAllParsesRSSItems(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(sampleRSS))
	}))
	defer server.Close()

	connector := NewConnector()
	config := &types.DataSourceConfig{
		Settings: map[string]interface{}{
			"feed_urls": []string{server.URL},
		},
	}

	items, err := connector.FetchAll(context.Background(), config, nil)
	if err != nil {
		t.Fatalf("FetchAll() error = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	if items[0].Title != "First post" {
		t.Fatalf("items[0].Title = %q, want %q", items[0].Title, "First post")
	}
	if items[0].SourceResourceID == "" {
		t.Fatal("expected SourceResourceID to be set")
	}
}

func TestFetchIncrementalReturnsOnlyChangedEntries(t *testing.T) {
	var response = sampleRSS
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(response))
	}))
	defer server.Close()

	connector := NewConnector()
	config := &types.DataSourceConfig{
		Settings: map[string]interface{}{
			"feed_urls": []string{server.URL},
		},
	}

	firstItems, cursor, err := connector.FetchIncremental(context.Background(), config, nil)
	if err != nil {
		t.Fatalf("FetchIncremental(first) error = %v", err)
	}
	if len(firstItems) != 2 {
		t.Fatalf("len(firstItems) = %d, want 2", len(firstItems))
	}

	response = updatedRSS
	secondItems, _, err := connector.FetchIncremental(context.Background(), config, cursor)
	if err != nil {
		t.Fatalf("FetchIncremental(second) error = %v", err)
	}
	if len(secondItems) != 1 {
		t.Fatalf("len(secondItems) = %d, want 1", len(secondItems))
	}
	if secondItems[0].Title != "First post updated" {
		t.Fatalf("secondItems[0].Title = %q, want %q", secondItems[0].Title, "First post updated")
	}
}

const sampleRSS = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Example Feed</title>
    <description>Example Description</description>
    <item>
      <guid>post-1</guid>
      <title>First post</title>
      <link>https://example.com/1</link>
      <description><![CDATA[<p>Hello RSS</p>]]></description>
      <pubDate>Mon, 02 Jan 2006 15:04:05 MST</pubDate>
    </item>
    <item>
      <guid>post-2</guid>
      <title>Second post</title>
      <link>https://example.com/2</link>
      <description><![CDATA[<p>More content</p>]]></description>
      <pubDate>Tue, 03 Jan 2006 15:04:05 MST</pubDate>
    </item>
  </channel>
</rss>`

const updatedRSS = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Example Feed</title>
    <description>Example Description</description>
    <item>
      <guid>post-1</guid>
      <title>First post updated</title>
      <link>https://example.com/1</link>
      <description><![CDATA[<p>Hello RSS updated</p>]]></description>
      <pubDate>Wed, 04 Jan 2006 15:04:05 MST</pubDate>
    </item>
    <item>
      <guid>post-2</guid>
      <title>Second post</title>
      <link>https://example.com/2</link>
      <description><![CDATA[<p>More content</p>]]></description>
      <pubDate>Tue, 03 Jan 2006 15:04:05 MST</pubDate>
    </item>
  </channel>
</rss>`
