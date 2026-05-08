package handler

import (
	stderrors "errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/Tencent/WeKnora/internal/application/repository"
	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

type WikiPageHandler struct {
	wikiService interfaces.WikiPageService
	kbService   interfaces.KnowledgeBaseService
}

func NewWikiPageHandler(
	wikiService interfaces.WikiPageService,
	kbService interfaces.KnowledgeBaseService,
) *WikiPageHandler {
	return &WikiPageHandler{wikiService: wikiService, kbService: kbService}
}

func (h *WikiPageHandler) validateWikiKB(c *gin.Context) (string, uint64, error) {
	ctx := c.Request.Context()
	kbID := strings.TrimSpace(c.Param("kb_id"))
	tenantID := c.GetUint64(types.TenantIDContextKey.String())
	if kbID == "" {
		return "", 0, apperrors.NewBadRequestError("Knowledge base ID is required")
	}

	kb, err := h.kbService.GetKnowledgeBaseByID(ctx, kbID)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{"knowledge_base_id": kbID})
		return "", 0, apperrors.NewNotFoundError("Knowledge base not found")
	}
	if !kb.IsWikiEnabled() {
		return "", 0, apperrors.NewBadRequestError("Wiki feature is not enabled for this knowledge base")
	}
	return kbID, tenantID, nil
}

func wikiSlugParam(c *gin.Context) string {
	return strings.TrimSpace(strings.TrimPrefix(c.Param("slug"), "/"))
}

func writeWikiError(c *gin.Context, err error) {
	if err == nil {
		return
	}
	if appErr, ok := apperrors.IsAppError(err); ok {
		c.JSON(appErr.HTTPCode, gin.H{"error": appErr.Message})
		return
	}
	if stderrors.Is(err, repository.ErrWikiPageNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Wiki page not found"})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}

func (h *WikiPageHandler) ListPages(c *gin.Context) {
	kbID, _, err := h.validateWikiKB(c)
	if err != nil {
		writeWikiError(c, err)
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	req := &types.WikiPageListRequest{
		KnowledgeBaseID: kbID,
		PageType:        c.Query("page_type"),
		Status:          c.Query("status"),
		Query:           c.Query("query"),
		Page:            page,
		PageSize:        pageSize,
		SortBy:          c.DefaultQuery("sort_by", "updated_at"),
		SortOrder:       c.DefaultQuery("sort_order", "desc"),
	}
	req.Normalize()

	resp, err := h.wikiService.ListPages(c.Request.Context(), req)
	if err != nil {
		writeWikiError(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *WikiPageHandler) CreatePage(c *gin.Context) {
	kbID, tenantID, err := h.validateWikiKB(c)
	if err != nil {
		writeWikiError(c, err)
		return
	}
	var page types.WikiPage
	if err := c.ShouldBindJSON(&page); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body: " + err.Error()})
		return
	}
	page.KnowledgeBaseID = kbID
	page.TenantID = tenantID

	created, err := h.wikiService.CreatePage(c.Request.Context(), &page)
	if err != nil {
		writeWikiError(c, err)
		return
	}
	c.JSON(http.StatusCreated, created)
}

func (h *WikiPageHandler) GetPage(c *gin.Context) {
	kbID, _, err := h.validateWikiKB(c)
	if err != nil {
		writeWikiError(c, err)
		return
	}
	slug := wikiSlugParam(c)
	if slug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Page slug is required"})
		return
	}

	page, err := h.wikiService.GetPageBySlug(c.Request.Context(), kbID, slug)
	if err != nil {
		writeWikiError(c, err)
		return
	}
	c.JSON(http.StatusOK, page)
}

func (h *WikiPageHandler) UpdatePage(c *gin.Context) {
	kbID, tenantID, err := h.validateWikiKB(c)
	if err != nil {
		writeWikiError(c, err)
		return
	}
	slug := wikiSlugParam(c)
	if slug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Page slug is required"})
		return
	}

	var page types.WikiPage
	if err := c.ShouldBindJSON(&page); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body: " + err.Error()})
		return
	}
	page.KnowledgeBaseID = kbID
	page.TenantID = tenantID
	page.Slug = slug

	updated, err := h.wikiService.UpdatePage(c.Request.Context(), &page)
	if err != nil {
		writeWikiError(c, err)
		return
	}
	c.JSON(http.StatusOK, updated)
}

func (h *WikiPageHandler) DeletePage(c *gin.Context) {
	kbID, _, err := h.validateWikiKB(c)
	if err != nil {
		writeWikiError(c, err)
		return
	}
	slug := wikiSlugParam(c)
	if slug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Page slug is required"})
		return
	}
	if err := h.wikiService.DeletePage(c.Request.Context(), kbID, slug); err != nil {
		writeWikiError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *WikiPageHandler) GetIndex(c *gin.Context) {
	kbID, _, err := h.validateWikiKB(c)
	if err != nil {
		writeWikiError(c, err)
		return
	}
	page, err := h.wikiService.GetIndex(c.Request.Context(), kbID)
	if err != nil {
		writeWikiError(c, err)
		return
	}
	c.JSON(http.StatusOK, page)
}

func (h *WikiPageHandler) GetLog(c *gin.Context) {
	kbID, _, err := h.validateWikiKB(c)
	if err != nil {
		writeWikiError(c, err)
		return
	}
	page, err := h.wikiService.GetLog(c.Request.Context(), kbID)
	if err != nil {
		writeWikiError(c, err)
		return
	}
	c.JSON(http.StatusOK, page)
}

func (h *WikiPageHandler) GetGraph(c *gin.Context) {
	kbID, _, err := h.validateWikiKB(c)
	if err != nil {
		writeWikiError(c, err)
		return
	}
	graph, err := h.wikiService.GetGraph(c.Request.Context(), kbID)
	if err != nil {
		writeWikiError(c, err)
		return
	}
	c.JSON(http.StatusOK, graph)
}

func (h *WikiPageHandler) GetStats(c *gin.Context) {
	kbID, _, err := h.validateWikiKB(c)
	if err != nil {
		writeWikiError(c, err)
		return
	}
	stats, err := h.wikiService.GetStats(c.Request.Context(), kbID)
	if err != nil {
		writeWikiError(c, err)
		return
	}
	c.JSON(http.StatusOK, stats)
}

func (h *WikiPageHandler) SearchPages(c *gin.Context) {
	kbID, _, err := h.validateWikiKB(c)
	if err != nil {
		writeWikiError(c, err)
		return
	}
	query := strings.TrimSpace(c.Query("q"))
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Search query 'q' is required"})
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	switch {
	case limit < 1:
		limit = 10
	case limit > 100:
		limit = 100
	}
	pages, err := h.wikiService.SearchPages(c.Request.Context(), kbID, query, limit)
	if err != nil {
		writeWikiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"pages": pages})
}

func (h *WikiPageHandler) RebuildLinks(c *gin.Context) {
	kbID, _, err := h.validateWikiKB(c)
	if err != nil {
		writeWikiError(c, err)
		return
	}
	if err := h.wikiService.RebuildLinks(c.Request.Context(), kbID); err != nil {
		writeWikiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Links rebuilt successfully"})
}
