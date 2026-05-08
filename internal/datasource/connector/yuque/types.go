package yuque

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Tencent/WeKnora/internal/types"
)

const DefaultBaseURL = "https://www.yuque.com"

type Config struct {
	APIToken string `json:"api_token"`
	BaseURL  string `json:"base_url,omitempty"`
}

func (c *Config) GetBaseURL() string {
	url := strings.TrimSpace(c.BaseURL)
	if url == "" {
		return DefaultBaseURL
	}
	if !strings.Contains(url, "://") {
		url = "https://" + url
	}
	return strings.TrimRight(url, "/")
}

func parseYuqueConfig(config *types.DataSourceConfig) (*Config, error) {
	if config == nil {
		return nil, fmt.Errorf("yuque: config is nil")
	}
	credBytes, err := json.Marshal(config.Credentials)
	if err != nil {
		return nil, fmt.Errorf("yuque: marshal credentials: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(credBytes, &cfg); err != nil {
		return nil, fmt.Errorf("yuque: parse credentials: %w", err)
	}
	if strings.TrimSpace(cfg.APIToken) == "" {
		return nil, fmt.Errorf("yuque: api_token is required")
	}
	return &cfg, nil
}

type flexibleStatus string

func (s *flexibleStatus) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		*s = ""
		return nil
	}
	if len(b) > 0 && b[0] == '"' {
		var str string
		if err := json.Unmarshal(b, &str); err != nil {
			return err
		}
		*s = flexibleStatus(str)
		return nil
	}
	var i int64
	if err := json.Unmarshal(b, &i); err != nil {
		return fmt.Errorf("yuque: status must be string or integer, got %s: %w", b, err)
	}
	*s = flexibleStatus(strconv.FormatInt(i, 10))
	return nil
}

type apiErrorBody struct {
	Message string `json:"message"`
	Status  int    `json:"status"`
}

type v2UserResponse struct {
	Data v2User `json:"data"`
}

type v2User struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
	Name  string `json:"name"`
}

type v2GroupListResponse struct {
	Data []v2Group `json:"data"`
}

type v2Group struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
	Name  string `json:"name"`
}

type v2RepoListResponse struct {
	Data []v2Repo `json:"data"`
}

type v2Repo struct {
	ID          int64  `json:"id"`
	Type        string `json:"type"`
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	UserID      int64  `json:"user_id"`
	Namespace   string `json:"namespace"`
	Public      int    `json:"public"`
	Description string `json:"description"`
	UpdatedAt   string `json:"updated_at"`
}

type v2DocListResponse struct {
	Meta struct {
		Total int `json:"total"`
	} `json:"meta"`
	Data []v2Doc `json:"data"`
}

type v2Doc struct {
	ID               int64          `json:"id"`
	Type             string         `json:"type"`
	Slug             string         `json:"slug"`
	Title            string         `json:"title"`
	BookID           int64          `json:"book_id"`
	UserID           int64          `json:"user_id"`
	Status           flexibleStatus `json:"status"`
	ContentUpdatedAt string         `json:"content_updated_at"`
	UpdatedAt        string         `json:"updated_at"`
	WordCount        int            `json:"word_count"`
}

type v2DocDetailResponse struct {
	Data v2DocDetail `json:"data"`
}

type v2DocDetail struct {
	ID               int64          `json:"id"`
	Type             string         `json:"type"`
	Slug             string         `json:"slug"`
	Title            string         `json:"title"`
	BookID           int64          `json:"book_id"`
	Format           string         `json:"format"`
	Body             string         `json:"body"`
	Status           flexibleStatus `json:"status"`
	ContentUpdatedAt string         `json:"content_updated_at"`
	UpdatedAt        string         `json:"updated_at"`
	WordCount        int            `json:"word_count"`
	Book             v2Repo         `json:"book"`
}

type yuqueCursor struct {
	LastSyncTime time.Time                    `json:"last_sync_time"`
	BookDocTimes map[string]map[string]string `json:"book_doc_times,omitempty"`
}

func parseContentUpdatedAt(ts string) time.Time {
	if ts == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return time.Time{}
	}
	return t
}

func sanitizeFileName(name string) string {
	if name == "" {
		return "untitled"
	}
	replacer := strings.NewReplacer(
		"/", "_", "\\", "_", ":", "_", "*", "_",
		"?", "_", "\"", "_", "<", "_", ">", "_", "|", "_",
	)
	result := replacer.Replace(name)
	const maxBytes = 200
	if len(result) > maxBytes {
		result = result[:maxBytes]
		for len(result) > 0 {
			r, size := utf8.DecodeLastRuneInString(result)
			if r != utf8.RuneError || size != 1 {
				break
			}
			result = result[:len(result)-1]
		}
	}
	return result
}
