package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

var ErrWikiPageNotFound = errors.New("wiki page not found")
var ErrWikiPageConflict = errors.New("wiki page version conflict")

type wikiPageRepository struct {
	db *gorm.DB
}

func NewWikiPageRepository(db *gorm.DB) interfaces.WikiPageRepository {
	return &wikiPageRepository{db: db}
}

func (r *wikiPageRepository) Create(ctx context.Context, page *types.WikiPage) error {
	return r.db.WithContext(ctx).Create(page).Error
}

func (r *wikiPageRepository) Update(ctx context.Context, page *types.WikiPage) error {
	expectedVersion := page.Version
	page.Version = expectedVersion + 1

	result := r.db.WithContext(ctx).
		Model(page).
		Where("id = ? AND version = ?", page.ID, expectedVersion).
		Updates(page)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		var count int64
		r.db.WithContext(ctx).Model(&types.WikiPage{}).Where("id = ?", page.ID).Count(&count)
		if count == 0 {
			return ErrWikiPageNotFound
		}
		return ErrWikiPageConflict
	}
	return nil
}

func (r *wikiPageRepository) UpdateMeta(ctx context.Context, page *types.WikiPage) error {
	result := r.db.WithContext(ctx).
		Model(page).
		Where("id = ?", page.ID).
		Updates(map[string]interface{}{
			"in_links":      page.InLinks,
			"out_links":     page.OutLinks,
			"status":        page.Status,
			"source_refs":   page.SourceRefs,
			"chunk_refs":    page.ChunkRefs,
			"page_metadata": page.PageMetadata,
			"updated_at":    page.UpdatedAt,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrWikiPageNotFound
	}
	return nil
}

func (r *wikiPageRepository) GetByID(ctx context.Context, id string) (*types.WikiPage, error) {
	var page types.WikiPage
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&page).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrWikiPageNotFound
		}
		return nil, err
	}
	return &page, nil
}

func (r *wikiPageRepository) GetBySlug(ctx context.Context, kbID string, slug string) (*types.WikiPage, error) {
	var page types.WikiPage
	if err := r.db.WithContext(ctx).
		Where("knowledge_base_id = ? AND slug = ?", kbID, slug).
		First(&page).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrWikiPageNotFound
		}
		return nil, err
	}
	return &page, nil
}

func (r *wikiPageRepository) List(ctx context.Context, req *types.WikiPageListRequest) ([]*types.WikiPage, int64, error) {
	query := r.db.WithContext(ctx).Model(&types.WikiPage{}).
		Where("knowledge_base_id = ?", req.KnowledgeBaseID)

	if req.PageType != "" {
		query = query.Where("page_type = ?", req.PageType)
	}
	if req.Status != "" {
		query = query.Where("status = ?", req.Status)
	}
	if req.Query != "" {
		like := "%" + escapeLikePattern(req.Query) + "%"
		query = query.Where("title ILIKE ? OR content ILIKE ? OR summary ILIKE ? OR slug ILIKE ?", like, like, like, like)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	sortBy := "updated_at"
	switch req.SortBy {
	case "title", "created_at", "updated_at", "page_type":
		sortBy = req.SortBy
	}
	sortOrder := "DESC"
	if strings.EqualFold(req.SortOrder, "asc") {
		sortOrder = "ASC"
	}
	query = query.Order(fmt.Sprintf("%s %s", sortBy, sortOrder))

	page := req.Page
	if page < 1 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	var pages []*types.WikiPage
	if err := query.Offset((page - 1) * pageSize).Limit(pageSize).Find(&pages).Error; err != nil {
		return nil, 0, err
	}
	return pages, total, nil
}

func (r *wikiPageRepository) ListAll(ctx context.Context, kbID string) ([]*types.WikiPage, error) {
	var pages []*types.WikiPage
	if err := r.db.WithContext(ctx).
		Where("knowledge_base_id = ?", kbID).
		Order("page_type ASC, title ASC").
		Find(&pages).Error; err != nil {
		return nil, err
	}
	return pages, nil
}

func (r *wikiPageRepository) Delete(ctx context.Context, kbID string, slug string) error {
	result := r.db.WithContext(ctx).
		Where("knowledge_base_id = ? AND slug = ?", kbID, slug).
		Delete(&types.WikiPage{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrWikiPageNotFound
	}
	return nil
}

func (r *wikiPageRepository) Search(ctx context.Context, kbID string, query string, limit int) ([]*types.WikiPage, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}
	like := "%" + escapeLikePattern(query) + "%"

	var pages []*types.WikiPage
	if err := r.db.WithContext(ctx).
		Where("knowledge_base_id = ?", kbID).
		Where("status != ?", types.WikiPageStatusArchived).
		Where("title ILIKE ? OR content ILIKE ? OR summary ILIKE ? OR slug ILIKE ?", like, like, like, like).
		Order("updated_at DESC").
		Limit(limit).
		Find(&pages).Error; err != nil {
		return nil, err
	}
	return pages, nil
}

func (r *wikiPageRepository) CountByType(ctx context.Context, kbID string) (map[string]int64, error) {
	type result struct {
		PageType string
		Count    int64
	}
	var results []result
	if err := r.db.WithContext(ctx).
		Model(&types.WikiPage{}).
		Select("page_type, count(*) as count").
		Where("knowledge_base_id = ?", kbID).
		Group("page_type").
		Scan(&results).Error; err != nil {
		return nil, err
	}

	counts := make(map[string]int64)
	for _, item := range results {
		counts[item.PageType] = item.Count
	}
	return counts, nil
}

func (r *wikiPageRepository) CountOrphans(ctx context.Context, kbID string) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&types.WikiPage{}).
		Where("knowledge_base_id = ?", kbID).
		Where("(in_links IS NULL OR in_links = '[]'::JSONB)").
		Where("page_type NOT IN ?", []string{types.WikiPageTypeIndex, types.WikiPageTypeLog}).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func escapeLikePattern(s string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(s)
}
