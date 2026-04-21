package handler

import (
	"net/http"

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
	data, err := h.service.HotQuestions(c.Request.Context())
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
	data, err := h.service.CoverageGaps(c.Request.Context())
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
	data, err := h.service.StaleDocuments(c.Request.Context())
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
	data, err := h.service.CitationHeatmap(c.Request.Context())
	if err != nil {
		c.Error(apperrors.NewInternalServerError(err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
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
