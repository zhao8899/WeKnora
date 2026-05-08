package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

type WikiPageService interface {
	CreatePage(ctx context.Context, page *types.WikiPage) (*types.WikiPage, error)
	UpdatePage(ctx context.Context, page *types.WikiPage) (*types.WikiPage, error)
	GetPageBySlug(ctx context.Context, kbID string, slug string) (*types.WikiPage, error)
	GetPageByID(ctx context.Context, id string) (*types.WikiPage, error)
	ListPages(ctx context.Context, req *types.WikiPageListRequest) (*types.WikiPageListResponse, error)
	DeletePage(ctx context.Context, kbID string, slug string) error
	GetIndex(ctx context.Context, kbID string) (*types.WikiPage, error)
	GetLog(ctx context.Context, kbID string) (*types.WikiPage, error)
	GetGraph(ctx context.Context, kbID string) (*types.WikiGraphData, error)
	GetStats(ctx context.Context, kbID string) (*types.WikiStats, error)
	RebuildLinks(ctx context.Context, kbID string) error
	SearchPages(ctx context.Context, kbID string, query string, limit int) ([]*types.WikiPage, error)
}

type WikiPageRepository interface {
	Create(ctx context.Context, page *types.WikiPage) error
	Update(ctx context.Context, page *types.WikiPage) error
	UpdateMeta(ctx context.Context, page *types.WikiPage) error
	GetByID(ctx context.Context, id string) (*types.WikiPage, error)
	GetBySlug(ctx context.Context, kbID string, slug string) (*types.WikiPage, error)
	List(ctx context.Context, req *types.WikiPageListRequest) ([]*types.WikiPage, int64, error)
	ListAll(ctx context.Context, kbID string) ([]*types.WikiPage, error)
	Delete(ctx context.Context, kbID string, slug string) error
	Search(ctx context.Context, kbID string, query string, limit int) ([]*types.WikiPage, error)
	CountByType(ctx context.Context, kbID string) (map[string]int64, error)
	CountOrphans(ctx context.Context, kbID string) (int64, error)
}
