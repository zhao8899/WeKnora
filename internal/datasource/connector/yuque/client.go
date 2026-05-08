package yuque

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	cfg        *Config
	httpClient *http.Client
}

func newClient(cfg *Config) *Client {
	return &Client{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) apiBase() string {
	return c.cfg.GetBaseURL() + "/api/v2"
}

func (c *Client) request(ctx context.Context, method, path string, body any, out any) error {
	var reqBody io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reqBody = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.apiBase()+path, reqBody)
	if err != nil {
		return err
	}
	req.Header.Set("X-Auth-Token", c.cfg.APIToken)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		var apiErr apiErrorBody
		if err := json.Unmarshal(raw, &apiErr); err == nil && apiErr.Message != "" {
			return fmt.Errorf("http %d: %s", resp.StatusCode, apiErr.Message)
		}
		return fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}

func (c *Client) Ping(ctx context.Context) error {
	var out v2UserResponse
	return c.request(ctx, http.MethodGet, "/user", nil, &out)
}

func (c *Client) GetCurrentUser(ctx context.Context) (*v2User, error) {
	var out v2UserResponse
	if err := c.request(ctx, http.MethodGet, "/user", nil, &out); err != nil {
		return nil, err
	}
	return &out.Data, nil
}

func (c *Client) ListUserRepos(ctx context.Context, login string) ([]v2Repo, error) {
	var out v2RepoListResponse
	if err := c.request(ctx, http.MethodGet, "/users/"+url.PathEscape(login)+"/repos?type=Book", nil, &out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

func (c *Client) ListUserGroups(ctx context.Context, userID int64) ([]v2Group, error) {
	var out v2GroupListResponse
	if err := c.request(ctx, http.MethodGet, fmt.Sprintf("/users/%d/groups", userID), nil, &out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

func (c *Client) ListGroupRepos(ctx context.Context, groupLogin string) ([]v2Repo, error) {
	var out v2RepoListResponse
	if err := c.request(ctx, http.MethodGet, "/groups/"+url.PathEscape(groupLogin)+"/repos?type=Book", nil, &out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

func (c *Client) ListBookDocs(ctx context.Context, bookID int64) ([]v2Doc, error) {
	var out v2DocListResponse
	if err := c.request(ctx, http.MethodGet, fmt.Sprintf("/repos/%d/docs?offset=0&limit=100", bookID), nil, &out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

func (c *Client) GetDocDetail(ctx context.Context, docID int64) (*v2DocDetail, error) {
	var out v2DocDetailResponse
	if err := c.request(ctx, http.MethodGet, fmt.Sprintf("/repos/docs/%d", docID), nil, &out); err != nil {
		return nil, err
	}
	return &out.Data, nil
}
