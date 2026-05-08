package handler

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestParseAnalyticsFilter_DefaultNil(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	req := httptest.NewRequest("GET", "/analytics/hot-questions", nil)
	c.Request = req

	filter, err := parseAnalyticsFilter(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if filter != nil {
		t.Fatalf("expected nil filter when no query parameters provided, got %#v", filter)
	}
}

func TestParseAnalyticsFilter_WithKnowledgeBaseSessionAndMessage(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	req := httptest.NewRequest("GET", "/analytics/unanswered-questions?knowledge_base_id=kb-42&session_id=s-1&message_id=m-1", nil)
	c.Request = req

	filter, err := parseAnalyticsFilter(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if filter == nil {
		t.Fatal("expected non-nil filter")
	}
	if filter.KnowledgeBaseID == nil || *filter.KnowledgeBaseID != "kb-42" {
		t.Fatalf("unexpected knowledge base id: %#v", filter.KnowledgeBaseID)
	}
	if filter.SessionID != "s-1" {
		t.Fatalf("unexpected session id: %q", filter.SessionID)
	}
	if filter.MessageID != "m-1" {
		t.Fatalf("unexpected message id: %q", filter.MessageID)
	}
}

func TestParseAnalyticsFilter_RejectsEmptyKnowledgeBaseID(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	req := httptest.NewRequest("GET", "/analytics/hot-questions?knowledge_base_id=%20", nil)
	c.Request = req

	filter, err := parseAnalyticsFilter(c)
	if err == nil {
		t.Fatal("expected parse error for empty knowledge_base_id")
	}
	if !strings.Contains(err.Error(), "knowledge_base_id must be a non-empty string") {
		t.Fatalf("unexpected error: %v", err)
	}
	if filter != nil {
		t.Fatalf("expected nil filter on parse error, got %#v", filter)
	}
}

func TestParseAnalyticsFilter_WithLimit(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	req := httptest.NewRequest("GET", "/analytics/hot-questions?limit=50", nil)
	c.Request = req

	filter, err := parseAnalyticsFilter(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if filter == nil || filter.Limit == nil {
		t.Fatalf("expected parsed limit, got %#v", filter)
	}
	if *filter.Limit != 50 {
		t.Fatalf("unexpected limit: %d", *filter.Limit)
	}
}

func TestParseAnalyticsFilter_RejectsInvalidLimit(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	req := httptest.NewRequest("GET", "/analytics/hot-questions?limit=0", nil)
	c.Request = req

	filter, err := parseAnalyticsFilter(c)
	if err == nil {
		t.Fatal("expected parse error for invalid limit")
	}
	if filter != nil {
		t.Fatalf("expected nil filter on parse error, got %#v", filter)
	}
}
