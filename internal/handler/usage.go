package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/types"
)

// UsageHandler handles usage statistics endpoints for the audit panel
type UsageHandler struct {
	db *gorm.DB
}

// NewUsageHandler creates a new UsageHandler
func NewUsageHandler(db *gorm.DB) *UsageHandler {
	return &UsageHandler{db: db}
}

// UsageStats represents the aggregated usage summary
type UsageStats struct {
	TotalSessions    int64 `json:"total_sessions"`
	TotalResponses   int64 `json:"total_responses"`    // assistant messages only
	TodaySessions    int64 `json:"today_sessions"`
	TodayResponses   int64 `json:"today_responses"`
	WeekSessions     int64 `json:"week_sessions"`
	WeekResponses    int64 `json:"week_responses"`
	MonthSessions    int64 `json:"month_sessions"`
	MonthResponses   int64 `json:"month_responses"`
	FeedbackLike     int64 `json:"feedback_like"`
	FeedbackDislike  int64 `json:"feedback_dislike"`
	ChannelBreakdown map[string]int64 `json:"channel_breakdown"`
}

// DailyUsagePoint represents a single day's usage data
type DailyUsagePoint struct {
	Date      string `json:"date"`       // YYYY-MM-DD
	Sessions  int64  `json:"sessions"`
	Responses int64  `json:"responses"`
}

// GetUsageStats godoc
// @Summary      获取使用量统计汇总
// @Tags         使用量审计
// @Produce      json
// @Success      200  {object}  map[string]interface{}
// @Security     Bearer
// @Router       /usage/stats [get]
func (h *UsageHandler) GetUsageStats(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := types.MustTenantIDFromContext(ctx)

	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	weekStart := todayStart.AddDate(0, 0, -int(now.Weekday()))
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	db := h.db.WithContext(ctx)
	stats := &UsageStats{ChannelBreakdown: make(map[string]int64)}

	// Session counts
	db.Model(&types.Session{}).Where("tenant_id = ? AND deleted_at IS NULL", tenantID).Count(&stats.TotalSessions)
	db.Model(&types.Session{}).Where("tenant_id = ? AND deleted_at IS NULL AND created_at >= ?", tenantID, todayStart).Count(&stats.TodaySessions)
	db.Model(&types.Session{}).Where("tenant_id = ? AND deleted_at IS NULL AND created_at >= ?", tenantID, weekStart).Count(&stats.WeekSessions)
	db.Model(&types.Session{}).Where("tenant_id = ? AND deleted_at IS NULL AND created_at >= ?", tenantID, monthStart).Count(&stats.MonthSessions)

	// Assistant message (response) counts - join messages with sessions for tenant isolation
	msgBase := db.Model(&types.Message{}).
		Joins("JOIN sessions ON sessions.id = messages.session_id AND sessions.tenant_id = ? AND sessions.deleted_at IS NULL", tenantID).
		Where("messages.role = 'assistant' AND messages.deleted_at IS NULL")
	msgBase.Count(&stats.TotalResponses)
	db.Model(&types.Message{}).
		Joins("JOIN sessions ON sessions.id = messages.session_id AND sessions.tenant_id = ? AND sessions.deleted_at IS NULL", tenantID).
		Where("messages.role = 'assistant' AND messages.deleted_at IS NULL AND messages.created_at >= ?", todayStart).
		Count(&stats.TodayResponses)
	db.Model(&types.Message{}).
		Joins("JOIN sessions ON sessions.id = messages.session_id AND sessions.tenant_id = ? AND sessions.deleted_at IS NULL", tenantID).
		Where("messages.role = 'assistant' AND messages.deleted_at IS NULL AND messages.created_at >= ?", weekStart).
		Count(&stats.WeekResponses)
	db.Model(&types.Message{}).
		Joins("JOIN sessions ON sessions.id = messages.session_id AND sessions.tenant_id = ? AND sessions.deleted_at IS NULL", tenantID).
		Where("messages.role = 'assistant' AND messages.deleted_at IS NULL AND messages.created_at >= ?", monthStart).
		Count(&stats.MonthResponses)

	// Feedback counts
	db.Model(&types.Message{}).
		Joins("JOIN sessions ON sessions.id = messages.session_id AND sessions.tenant_id = ? AND sessions.deleted_at IS NULL", tenantID).
		Where("messages.feedback = 'like' AND messages.deleted_at IS NULL").
		Count(&stats.FeedbackLike)
	db.Model(&types.Message{}).
		Joins("JOIN sessions ON sessions.id = messages.session_id AND sessions.tenant_id = ? AND sessions.deleted_at IS NULL", tenantID).
		Where("messages.feedback = 'dislike' AND messages.deleted_at IS NULL").
		Count(&stats.FeedbackDislike)

	// Channel breakdown (for assistant messages)
	type channelCount struct {
		Channel string
		Count   int64
	}
	var channels []channelCount
	db.Model(&types.Message{}).
		Select("messages.channel, COUNT(*) as count").
		Joins("JOIN sessions ON sessions.id = messages.session_id AND sessions.tenant_id = ? AND sessions.deleted_at IS NULL", tenantID).
		Where("messages.role = 'assistant' AND messages.deleted_at IS NULL AND messages.channel != ''").
		Group("messages.channel").
		Scan(&channels)
	for _, ch := range channels {
		stats.ChannelBreakdown[ch.Channel] = ch.Count
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": stats})
}

// GetDailyTrend godoc
// @Summary      获取每日使用量趋势
// @Tags         使用量审计
// @Produce      json
// @Param        days  query  int  false  "天数（默认30）"
// @Success      200   {object}  map[string]interface{}
// @Security     Bearer
// @Router       /usage/daily-trend [get]
func (h *UsageHandler) GetDailyTrend(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := types.MustTenantIDFromContext(ctx)

	daysStr := c.DefaultQuery("days", "30")
	days, err := strconv.Atoi(daysStr)
	if err != nil || days < 1 || days > 365 {
		c.Error(errors.NewBadRequestError("days must be 1-365"))
		return
	}

	now := time.Now()
	since := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -(days - 1))

	type dailyRow struct {
		Day       string `gorm:"column:day"`
		Sessions  int64  `gorm:"column:sessions"`
		Responses int64  `gorm:"column:responses"`
	}

	var rows []dailyRow
	h.db.WithContext(ctx).Raw(`
		SELECT
			TO_CHAR(d.day, 'YYYY-MM-DD') AS day,
			COALESCE(s.sessions, 0)   AS sessions,
			COALESCE(m.responses, 0)  AS responses
		FROM generate_series(?::date, ?::date, '1 day'::interval) AS d(day)
		LEFT JOIN (
			SELECT DATE(created_at) AS day, COUNT(*) AS sessions
			FROM sessions
			WHERE tenant_id = ? AND deleted_at IS NULL AND created_at >= ?
			GROUP BY DATE(created_at)
		) s ON s.day = d.day
		LEFT JOIN (
			SELECT DATE(messages.created_at) AS day, COUNT(*) AS responses
			FROM messages
			JOIN sessions ON sessions.id = messages.session_id AND sessions.tenant_id = ? AND sessions.deleted_at IS NULL
			WHERE messages.role = 'assistant' AND messages.deleted_at IS NULL AND messages.created_at >= ?
			GROUP BY DATE(messages.created_at)
		) m ON m.day = d.day
		ORDER BY d.day ASC
	`, since.Format("2006-01-02"), now.Format("2006-01-02"),
		tenantID, since,
		tenantID, since,
	).Scan(&rows)

	points := make([]DailyUsagePoint, len(rows))
	for i, r := range rows {
		points[i] = DailyUsagePoint{Date: r.Day, Sessions: r.Sessions, Responses: r.Responses}
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": points})
}
