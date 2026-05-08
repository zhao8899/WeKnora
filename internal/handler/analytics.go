package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/Tencent/WeKnora/internal/application/service"
	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
)

type AnalyticsHandler struct {
	service *service.AnalyticsService
}

func NewAnalyticsHandler(service *service.AnalyticsService) *AnalyticsHandler {
	return &AnalyticsHandler{service: service}
}

func (h *AnalyticsHandler) GetHotQuestions(c *gin.Context) {
	if !ensurePlatformAdmin(c) {
		return
	}
	filter, err := parseAnalyticsFilter(c)
	if err != nil {
		c.Error(err)
		return
	}
	data, err := h.service.HotQuestions(c.Request.Context(), filter)
	if err != nil {
		c.Error(apperrors.NewInternalServerError(err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

func (h *AnalyticsHandler) GetCoverageGaps(c *gin.Context) {
	if !ensurePlatformAdmin(c) {
		return
	}
	filter, err := parseAnalyticsFilter(c)
	if err != nil {
		c.Error(err)
		return
	}
	data, err := h.service.CoverageGaps(c.Request.Context(), filter)
	if err != nil {
		c.Error(apperrors.NewInternalServerError(err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

func (h *AnalyticsHandler) GetStaleDocuments(c *gin.Context) {
	if !ensurePlatformAdmin(c) {
		return
	}
	filter, err := parseAnalyticsFilter(c)
	if err != nil {
		c.Error(err)
		return
	}
	data, err := h.service.StaleDocuments(c.Request.Context(), filter)
	if err != nil {
		c.Error(apperrors.NewInternalServerError(err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

func (h *AnalyticsHandler) GetCitationHeatmap(c *gin.Context) {
	if !ensurePlatformAdmin(c) {
		return
	}
	filter, err := parseAnalyticsFilter(c)
	if err != nil {
		c.Error(err)
		return
	}
	data, err := h.service.CitationHeatmap(c.Request.Context(), filter)
	if err != nil {
		c.Error(apperrors.NewInternalServerError(err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

func (h *AnalyticsHandler) GetUnansweredQuestions(c *gin.Context) {
	if !ensurePlatformAdmin(c) {
		return
	}
	filter, err := parseAnalyticsFilter(c)
	if err != nil {
		c.Error(err)
		return
	}
	data, err := h.service.UnansweredQuestions(c.Request.Context(), filter)
	if err != nil {
		c.Error(apperrors.NewInternalServerError(err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

func parseAnalyticsFilter(c *gin.Context) (*types.AnalyticsFilter, error) {
	filter := &types.AnalyticsFilter{
		SessionID: strings.TrimSpace(c.Query("session_id")),
		MessageID: strings.TrimSpace(c.Query("message_id")),
	}

	kbIDText := strings.TrimSpace(c.Query("knowledge_base_id"))
	if _, ok := c.GetQuery("knowledge_base_id"); ok {
		if kbIDText == "" {
			return nil, apperrors.NewBadRequestError("knowledge_base_id must be a non-empty string")
		}
		filter.KnowledgeBaseID = &kbIDText
	}

	limitText := strings.TrimSpace(c.Query("limit"))
	if limitText != "" {
		parsed, err := strconv.Atoi(limitText)
		if err != nil || parsed <= 0 {
			return nil, apperrors.NewBadRequestError("limit must be a positive integer")
		}
		filter.Limit = &parsed
	}

	if filter.KnowledgeBaseID == nil && filter.SessionID == "" && filter.MessageID == "" && filter.Limit == nil {
		return nil, nil
	}
	return filter, nil
}

func ensurePlatformAdmin(c *gin.Context) bool {
	userValue, ok := c.Get(types.UserContextKey.String())
	if !ok {
		c.Error(apperrors.NewUnauthorizedError("user context missing"))
		return false
	}
	user, ok := userValue.(*types.User)
	if !ok || user == nil {
		c.Error(apperrors.NewUnauthorizedError("invalid user context"))
		return false
	}
	if !user.CanAccessAllTenants {
		c.Error(apperrors.NewForbiddenError("platform admin required"))
		return false
	}
	return true
}
