package yuque

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

type Connector struct{}

func NewConnector() *Connector { return &Connector{} }

func (c *Connector) Type() string { return types.ConnectorTypeYuque }

func (c *Connector) Validate(ctx context.Context, config *types.DataSourceConfig) error {
	cfg, err := parseYuqueConfig(config)
	if err != nil {
		return err
	}
	cli := newClient(cfg)
	if err := cli.Ping(ctx); err != nil {
		return fmt.Errorf("yuque connection failed: %w", err)
	}
	return nil
}

func (c *Connector) ListResources(ctx context.Context, config *types.DataSourceConfig) ([]types.Resource, error) {
	cfg, err := parseYuqueConfig(config)
	if err != nil {
		return nil, err
	}
	cli := newClient(cfg)

	me, err := cli.GetCurrentUser(ctx)
	if err != nil {
		return nil, fmt.Errorf("get current user: %w", err)
	}

	repos := make(map[int64]v2Repo)

	personal, err := cli.ListUserRepos(ctx, me.Login)
	if err != nil {
		return nil, fmt.Errorf("list personal repos: %w", err)
	}
	for _, repo := range personal {
		repos[repo.ID] = repo
	}

	groups, err := cli.ListUserGroups(ctx, me.ID)
	if err != nil {
		return nil, fmt.Errorf("list groups: %w", err)
	}
	for _, group := range groups {
		groupRepos, err := cli.ListGroupRepos(ctx, group.Login)
		if err != nil {
			logger.Warnf(ctx, "[Yuque] skip group %s: %v", group.Login, err)
			continue
		}
		for _, repo := range groupRepos {
			if _, exists := repos[repo.ID]; !exists {
				repos[repo.ID] = repo
			}
		}
	}

	out := make([]types.Resource, 0, len(repos))
	for _, repo := range repos {
		out = append(out, types.Resource{
			ExternalID:  strconv.FormatInt(repo.ID, 10),
			Name:        repo.Name,
			Type:        "book",
			Description: repo.Namespace,
			URL:         cfg.GetBaseURL() + "/" + repo.Namespace,
			ModifiedAt:  parseContentUpdatedAt(repo.UpdatedAt),
			Metadata: map[string]interface{}{
				"public":    repo.Public,
				"book_type": repo.Type,
			},
		})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].ExternalID < out[j].ExternalID })
	return out, nil
}

func (c *Connector) FetchAll(ctx context.Context, config *types.DataSourceConfig, resourceIDs []string) ([]types.FetchedItem, error) {
	items, _, err := c.walk(ctx, config, resourceIDs, nil, false)
	return items, err
}

func (c *Connector) FetchIncremental(ctx context.Context, config *types.DataSourceConfig, cursor *types.SyncCursor) ([]types.FetchedItem, *types.SyncCursor, error) {
	prev := &yuqueCursor{}
	if cursor != nil && cursor.ConnectorCursor != nil {
		raw, _ := json.Marshal(cursor.ConnectorCursor)
		_ = json.Unmarshal(raw, prev)
	}
	return c.walk(ctx, config, config.ResourceIDs, prev, true)
}

func (c *Connector) walk(ctx context.Context, config *types.DataSourceConfig, resourceIDs []string, prev *yuqueCursor, incremental bool) ([]types.FetchedItem, *types.SyncCursor, error) {
	cfg, err := parseYuqueConfig(config)
	if err != nil {
		return nil, nil, err
	}
	cli := newClient(cfg)

	if len(resourceIDs) == 0 {
		return nil, nil, fmt.Errorf("yuque: no resource ids configured")
	}

	newCursor := &yuqueCursor{LastSyncTime: time.Now(), BookDocTimes: make(map[string]map[string]string)}
	var out []types.FetchedItem

	for _, bookIDStr := range resourceIDs {
		bookID, err := strconv.ParseInt(bookIDStr, 10, 64)
		if err != nil {
			return nil, nil, fmt.Errorf("yuque: invalid book id %q: %w", bookIDStr, err)
		}

		docs, err := cli.ListBookDocs(ctx, bookID)
		if err != nil {
			return nil, nil, fmt.Errorf("yuque: list docs for book %d: %w", bookID, err)
		}

		newCursor.BookDocTimes[bookIDStr] = make(map[string]string, len(docs))
		currentDocs := make(map[string]bool, len(docs))

		for _, doc := range docs {
			if doc.Type != "" && doc.Type != "Doc" {
				continue
			}
			if doc.Status != "" && doc.Status != "1" {
				continue
			}

			docID := strconv.FormatInt(doc.ID, 10)
			currentDocs[docID] = true
			newCursor.BookDocTimes[bookIDStr][docID] = doc.ContentUpdatedAt

			if incremental && prev != nil && prev.BookDocTimes != nil {
				if prevTimes, ok := prev.BookDocTimes[bookIDStr]; ok && prevTimes[docID] == doc.ContentUpdatedAt {
					continue
				}
			}

			detail, err := cli.GetDocDetail(ctx, doc.ID)
			if err != nil {
				out = append(out, types.FetchedItem{
					ExternalID:       docID,
					Title:            doc.Title,
					SourceResourceID: bookIDStr,
				Metadata: map[string]string{
					"error":   err.Error(),
					"channel": "yuque",
					"doc_id":  docID,
					"book_id": bookIDStr,
					"slug":    doc.Slug,
					},
				})
				continue
			}

			if detail.Format != "" && detail.Format != "markdown" && detail.Format != "lake" {
				out = append(out, types.FetchedItem{
					ExternalID:       docID,
					Title:            doc.Title,
					SourceResourceID: bookIDStr,
				Metadata: map[string]string{
					"channel":     "yuque",
					"doc_id":      docID,
					"book_id":     bookIDStr,
					"slug":        doc.Slug,
						"skip_reason": "unsupported format: " + detail.Format,
					},
				})
				continue
			}

			out = append(out, types.FetchedItem{
				ExternalID:       docID,
				Title:            doc.Title,
				Content:          []byte(detail.Body),
				ContentType:      "text/markdown",
				FileName:         sanitizeFileName(doc.Title) + ".md",
				URL:              cfg.GetBaseURL() + "/" + detail.Book.Namespace + "/" + doc.Slug,
				UpdatedAt:        parseContentUpdatedAt(doc.ContentUpdatedAt),
				SourceResourceID: bookIDStr,
				Metadata: map[string]string{
					"doc_id":     docID,
					"book_id":    bookIDStr,
					"slug":       doc.Slug,
					"creator":    strconv.FormatInt(doc.UserID, 10),
					"word_count": strconv.Itoa(doc.WordCount),
					"channel":    "yuque",
				},
			})
		}

		if incremental && prev != nil && prev.BookDocTimes != nil {
			if prevTimes, ok := prev.BookDocTimes[bookIDStr]; ok {
				for prevDocID := range prevTimes {
					if !currentDocs[prevDocID] {
						out = append(out, types.FetchedItem{
							ExternalID:       prevDocID,
							IsDeleted:        true,
							SourceResourceID: bookIDStr,
						})
					}
				}
			}
		}
	}

	nextCursorBytes, _ := json.Marshal(newCursor)
	nextCursorMap := map[string]interface{}{}
	_ = json.Unmarshal(nextCursorBytes, &nextCursorMap)

	return out, &types.SyncCursor{
		LastSyncTime:    time.Now(),
		ConnectorCursor: nextCursorMap,
	}, nil
}
