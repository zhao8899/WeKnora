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

type SourceFeedbackRequest struct {
	EvidenceID string `json:"evidence_id" binding:"required"`
	Feedback   string `json:"feedback" binding:"required"`
	Comment    string `json:"comment"`
}

// GetAnswerConfidence godoc
// @Summary      Get answer confidence details
// @Description  Returns evidence strength, source health, and cited evidence details for an assistant answer
// @Tags         chat
// @Accept       json
// @Produce      json
// @Param        message_id  path      string  true  "Assistant message ID"
// @Success      200         {object}  map[string]interface{}                         "Confidence details"
// @Failure      400         {object}  github_com_Tencent_WeKnora_internal_errors.AppError  "Invalid request"
// @Failure      404         {object}  github_com_Tencent_WeKnora_internal_errors.AppError  "Message not found"
// @Failure      500         {object}  github_com_Tencent_WeKnora_internal_errors.AppError  "Internal server error"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /chat/answer/{message_id}/confidence [get]
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

// SubmitSourceFeedback godoc
// @Summary      Submit source feedback
// @Description  Submits up, down, or expired feedback for a cited source in an assistant answer
// @Tags         chat
// @Accept       json
// @Produce      json
// @Param        message_id  path      string                 true  "Assistant message ID"
// @Param        request     body      SourceFeedbackRequest  true  "Source feedback payload"
// @Success      200         {object}  map[string]interface{}                         "Feedback accepted"
// @Failure      400         {object}  github_com_Tencent_WeKnora_internal_errors.AppError  "Invalid request"
// @Failure      404         {object}  github_com_Tencent_WeKnora_internal_errors.AppError  "Message or evidence not found"
// @Failure      500         {object}  github_com_Tencent_WeKnora_internal_errors.AppError  "Internal server error"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /chat/answer/{message_id}/feedback [post]
func (h *ConfidenceHandler) SubmitSourceFeedback(c *gin.Context) {
	messageID := c.Param("message_id")
	if messageID == "" {
		c.Error(errors.NewBadRequestError("message_id is required"))
		return
	}

	var req SourceFeedbackRequest
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
