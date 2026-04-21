package handler

import (
	"net/http"
	"strings"

	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

type ConfidenceHandler struct {
	service interfaces.ConfidenceService
}

func NewConfidenceHandler(service interfaces.ConfidenceService) *ConfidenceHandler {
	return &ConfidenceHandler{service: service}
}

type sourceFeedbackRequest struct {
	EvidenceID string `json:"evidence_id" binding:"required"`
	Feedback   string `json:"feedback" binding:"required"`
	Comment    string `json:"comment"`
}

func (h *ConfidenceHandler) GetAnswerConfidence(c *gin.Context) {
	messageID := c.Param("message_id")
	if messageID == "" {
		c.Error(errors.NewBadRequestError("message_id is required"))
		return
	}

	data, err := h.service.GetAnswerConfidence(c.Request.Context(), messageID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.Error(errors.NewNotFoundError(err.Error()))
			return
		}
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

func (h *ConfidenceHandler) SubmitSourceFeedback(c *gin.Context) {
	messageID := c.Param("message_id")
	if messageID == "" {
		c.Error(errors.NewBadRequestError("message_id is required"))
		return
	}

	var req sourceFeedbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}

	if err := h.service.SubmitSourceFeedback(
		c.Request.Context(), messageID, req.EvidenceID, req.Feedback, req.Comment,
	); err != nil {
		if strings.Contains(err.Error(), "invalid feedback") {
			c.Error(errors.NewBadRequestError(err.Error()))
			return
		}
		if strings.Contains(err.Error(), "not found") {
			c.Error(errors.NewNotFoundError(err.Error()))
			return
		}
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
