package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/google/uuid"
)

var wikiLinkRegex = regexp.MustCompile(`\[\[([^\]]+)\]\]`)

type wikiPageService struct {
	repo      interfaces.WikiPageRepository
	kbService interfaces.KnowledgeBaseService
}

func NewWikiPageService(
	repo interfaces.WikiPageRepository,
	kbService interfaces.KnowledgeBaseService,
) interfaces.WikiPageService {
	return &wikiPageService{repo: repo, kbService: kbService}
}

func (s *wikiPageService) CreatePage(ctx context.Context, page *types.WikiPage) (*types.WikiPage, error) {
	if page.ID == "" {
		page.ID = uuid.New().String()
	}
	if page.Slug == "" {
		return nil, errors.New("wiki page slug is required")
	}
	if page.KnowledgeBaseID == "" {
		return nil, errors.New("knowledge_base_id is required")
	}
	if page.PageType == "" {
		page.PageType = types.WikiPageTypeSummary
	}
	if page.Status == "" {
		page.Status = types.WikiPageStatusPublished
	}
	if page.Version == 0 {
		page.Version = 1
	}
	page.Slug = normalizeWikiSlug(page.Slug)
	page.OutLinks = parseWikiOutLinks(page.Content)

	now := time.Now()
	page.CreatedAt = now
	page.UpdatedAt = now

	if err := s.repo.Create(ctx, page); err != nil {
		return nil, fmt.Errorf("create wiki page: %w", err)
	}
	s.updateInLinks(ctx, page.KnowledgeBaseID, page.Slug, page.OutLinks)
	return page, nil
}

func (s *wikiPageService) UpdatePage(ctx context.Context, page *types.WikiPage) (*types.WikiPage, error) {
	existing, err := s.repo.GetBySlug(ctx, page.KnowledgeBaseID, page.Slug)
	if err != nil {
		return nil, fmt.Errorf("get existing page: %w", err)
	}

	oldOutLinks := existing.OutLinks
	contentChanged := existing.Title != page.Title ||
		existing.Content != page.Content ||
		existing.Summary != page.Summary ||
		existing.PageType != page.PageType ||
		existing.Status != page.Status

	existing.Title = page.Title
	existing.Content = page.Content
	existing.Summary = page.Summary
	existing.PageType = page.PageType
	existing.Status = page.Status
	existing.SourceRefs = page.SourceRefs
	existing.ChunkRefs = page.ChunkRefs
	existing.PageMetadata = page.PageMetadata
	existing.Aliases = page.Aliases
	existing.OutLinks = parseWikiOutLinks(existing.Content)
	existing.UpdatedAt = time.Now()

	if contentChanged {
		if err := s.repo.Update(ctx, existing); err != nil {
			return nil, fmt.Errorf("update wiki page: %w", err)
		}
	} else if err := s.repo.UpdateMeta(ctx, existing); err != nil {
		return nil, fmt.Errorf("update wiki page meta: %w", err)
	}

	s.removeInLinks(ctx, existing.KnowledgeBaseID, existing.Slug, oldOutLinks)
	s.updateInLinks(ctx, existing.KnowledgeBaseID, existing.Slug, existing.OutLinks)
	return existing, nil
}

func (s *wikiPageService) GetPageBySlug(ctx context.Context, kbID string, slug string) (*types.WikiPage, error) {
	return s.repo.GetBySlug(ctx, kbID, normalizeWikiSlug(slug))
}

func (s *wikiPageService) GetPageByID(ctx context.Context, id string) (*types.WikiPage, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *wikiPageService) ListPages(ctx context.Context, req *types.WikiPageListRequest) (*types.WikiPageListResponse, error) {
	pages, total, err := s.repo.List(ctx, req)
	if err != nil {
		return nil, err
	}

	pageSize := req.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	page := req.Page
	if page < 1 {
		page = 1
	}
	totalPages := int(total) / pageSize
	if int(total)%pageSize > 0 {
		totalPages++
	}

	return &types.WikiPageListResponse{
		Pages:      pages,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}

func (s *wikiPageService) DeletePage(ctx context.Context, kbID string, slug string) error {
	page, err := s.repo.GetBySlug(ctx, kbID, normalizeWikiSlug(slug))
	if err != nil {
		return err
	}
	s.removeInLinks(ctx, kbID, page.Slug, page.OutLinks)
	return s.repo.Delete(ctx, kbID, page.Slug)
}

func (s *wikiPageService) GetIndex(ctx context.Context, kbID string) (*types.WikiPage, error) {
	page, err := s.repo.GetBySlug(ctx, kbID, "index")
	if err == nil {
		return page, nil
	}
	if !errors.Is(err, repository.ErrWikiPageNotFound) {
		return nil, err
	}
	return s.createDefaultPage(ctx, kbID, "index", "Index", types.WikiPageTypeIndex,
		"# Wiki Index\n\nThis wiki contains knowledge extracted from uploaded documents.\n")
}

func (s *wikiPageService) GetLog(ctx context.Context, kbID string) (*types.WikiPage, error) {
	page, err := s.repo.GetBySlug(ctx, kbID, "log")
	if err == nil {
		return page, nil
	}
	if !errors.Is(err, repository.ErrWikiPageNotFound) {
		return nil, err
	}
	return s.createDefaultPage(ctx, kbID, "log", "Log", types.WikiPageTypeLog,
		"# Wiki Operation Log\n\nChronological record of wiki operations.\n")
}

func (s *wikiPageService) GetGraph(ctx context.Context, kbID string) (*types.WikiGraphData, error) {
	pages, err := s.repo.ListAll(ctx, kbID)
	if err != nil {
		return nil, err
	}

	nodeMap := make(map[string]*types.WikiGraphNode)
	var edges []types.WikiGraphEdge
	for _, page := range pages {
		nodeMap[page.Slug] = &types.WikiGraphNode{
			Slug:      page.Slug,
			Title:     page.Title,
			PageType:  page.PageType,
			LinkCount: len(page.InLinks) + len(page.OutLinks),
		}
	}
	for _, page := range pages {
		for _, target := range page.OutLinks {
			if _, ok := nodeMap[target]; ok {
				edges = append(edges, types.WikiGraphEdge{Source: page.Slug, Target: target})
			}
		}
	}

	nodes := make([]types.WikiGraphNode, 0, len(nodeMap))
	for _, node := range nodeMap {
		nodes = append(nodes, *node)
	}
	return &types.WikiGraphData{Nodes: nodes, Edges: edges}, nil
}

func (s *wikiPageService) GetStats(ctx context.Context, kbID string) (*types.WikiStats, error) {
	counts, err := s.repo.CountByType(ctx, kbID)
	if err != nil {
		return nil, err
	}
	var total int64
	for _, count := range counts {
		total += count
	}
	orphans, err := s.repo.CountOrphans(ctx, kbID)
	if err != nil {
		return nil, err
	}
	pages, err := s.repo.ListAll(ctx, kbID)
	if err != nil {
		return nil, err
	}
	var totalLinks int64
	for _, page := range pages {
		totalLinks += int64(len(page.OutLinks))
	}
	recent, _, err := s.repo.List(ctx, &types.WikiPageListRequest{
		KnowledgeBaseID: kbID,
		Page:            1,
		PageSize:        10,
		SortBy:          "updated_at",
		SortOrder:       "desc",
	})
	if err != nil {
		return nil, err
	}
	return &types.WikiStats{
		TotalPages:    total,
		PagesByType:   counts,
		TotalLinks:    totalLinks,
		OrphanCount:   orphans,
		RecentUpdates: recent,
	}, nil
}

func (s *wikiPageService) RebuildLinks(ctx context.Context, kbID string) error {
	pages, err := s.repo.ListAll(ctx, kbID)
	if err != nil {
		return err
	}

	pageMap := make(map[string]*types.WikiPage, len(pages))
	for _, page := range pages {
		pageMap[page.Slug] = page
		page.InLinks = types.StringArray{}
	}
	for _, page := range pages {
		page.OutLinks = parseWikiOutLinks(page.Content)
		for _, target := range page.OutLinks {
			if targetPage, ok := pageMap[target]; ok {
				targetPage.InLinks = append(targetPage.InLinks, page.Slug)
			}
		}
	}
	for _, page := range pages {
		page.UpdatedAt = time.Now()
		if err := s.repo.UpdateMeta(ctx, page); err != nil {
			logger.Warnf(ctx, "wiki: failed to rebuild links for %s: %v", page.Slug, err)
		}
	}
	return nil
}

func (s *wikiPageService) SearchPages(ctx context.Context, kbID string, query string, limit int) ([]*types.WikiPage, error) {
	return s.repo.Search(ctx, kbID, query, limit)
}

func (s *wikiPageService) createDefaultPage(
	ctx context.Context,
	kbID string,
	slug string,
	title string,
	pageType string,
	content string,
) (*types.WikiPage, error) {
	kb, err := s.kbService.GetKnowledgeBaseByIDOnly(ctx, kbID)
	if err != nil {
		return nil, fmt.Errorf("get knowledge base: %w", err)
	}
	page := &types.WikiPage{
		ID:              uuid.New().String(),
		TenantID:        kb.TenantID,
		KnowledgeBaseID: kbID,
		Slug:            slug,
		Title:           title,
		PageType:        pageType,
		Status:          types.WikiPageStatusPublished,
		Content:         content,
		Summary:         title,
		Version:         1,
	}
	return s.CreatePage(ctx, page)
}

func (s *wikiPageService) updateInLinks(ctx context.Context, kbID string, sourceSlug string, targets types.StringArray) {
	for _, targetSlug := range targets {
		targetPage, err := s.repo.GetBySlug(ctx, kbID, targetSlug)
		if err != nil {
			continue
		}
		if !containsWikiString(targetPage.InLinks, sourceSlug) {
			targetPage.InLinks = append(targetPage.InLinks, sourceSlug)
			targetPage.UpdatedAt = time.Now()
			if err := s.repo.UpdateMeta(ctx, targetPage); err != nil {
				logger.Warnf(ctx, "wiki: failed to update in_links for %s: %v", targetSlug, err)
			}
		}
	}
}

func (s *wikiPageService) removeInLinks(ctx context.Context, kbID string, sourceSlug string, targets types.StringArray) {
	for _, targetSlug := range targets {
		targetPage, err := s.repo.GetBySlug(ctx, kbID, targetSlug)
		if err != nil {
			continue
		}
		next := removeWikiString(targetPage.InLinks, sourceSlug)
		if len(next) == len(targetPage.InLinks) {
			continue
		}
		targetPage.InLinks = next
		targetPage.UpdatedAt = time.Now()
		if err := s.repo.UpdateMeta(ctx, targetPage); err != nil {
			logger.Warnf(ctx, "wiki: failed to remove in_links for %s: %v", targetSlug, err)
		}
	}
}

func parseWikiOutLinks(content string) types.StringArray {
	matches := wikiLinkRegex.FindAllStringSubmatch(content, -1)
	seen := make(map[string]bool)
	links := make(types.StringArray, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		slug := strings.TrimSpace(match[1])
		if parts := strings.SplitN(slug, "|", 2); len(parts) == 2 {
			slug = strings.TrimSpace(parts[0])
		}
		slug = normalizeWikiSlug(slug)
		if slug == "" || seen[slug] {
			continue
		}
		seen[slug] = true
		links = append(links, slug)
	}
	return links
}

func normalizeWikiSlug(slug string) string {
	slug = strings.ToLower(strings.TrimSpace(slug))
	slug = strings.ReplaceAll(slug, " ", "-")
	return slug
}

func containsWikiString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func removeWikiString(values []string, needle string) types.StringArray {
	result := make(types.StringArray, 0, len(values))
	for _, value := range values {
		if value != needle {
			result = append(result, value)
		}
	}
	return result
}
