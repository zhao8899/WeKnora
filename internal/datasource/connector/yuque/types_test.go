package yuque

import (
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

func TestParseYuqueConfig(t *testing.T) {
	cfg, err := parseYuqueConfig(&types.DataSourceConfig{
		Credentials: map[string]interface{}{
			"api_token": "token-123",
			"base_url":  "yuque.example.com",
		},
	})
	if err != nil {
		t.Fatalf("parseYuqueConfig returned error: %v", err)
	}
	if cfg.GetBaseURL() != "https://yuque.example.com" {
		t.Fatalf("unexpected base url: %s", cfg.GetBaseURL())
	}
}
