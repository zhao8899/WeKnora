package types

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	DocumentAccessTypeRetrieved = "retrieved"
	DocumentAccessTypeReranked  = "reranked"
	DocumentAccessTypeCited     = "cited"
)

type DocumentAccessLog struct {
	ID          string    `json:"id" gorm:"type:varchar(36);primaryKey"`
	TenantID    uint64    `json:"tenant_id" gorm:"index"`
	KnowledgeID string    `json:"knowledge_id,omitempty" gorm:"type:varchar(36);index"`
	SessionID   string    `json:"session_id" gorm:"type:varchar(36);index"`
	MessageID   string    `json:"message_id" gorm:"type:varchar(36);index"`
	AccessType  string    `json:"access_type" gorm:"type:varchar(20);index"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (DocumentAccessLog) TableName() string {
	return "document_access_logs"
}

func (d *DocumentAccessLog) BeforeCreate(tx *gorm.DB) error {
	if d.ID == "" {
		d.ID = uuid.New().String()
	}
	return nil
}

type HotQuestion struct {
	MessageID      string    `json:"message_id"`
	SessionID      string    `json:"session_id"`
	Question       string    `json:"question"`
	RetrievedCount int64     `json:"retrieved_count"`
	RerankedCount  int64     `json:"reranked_count"`
	CitedCount     int64     `json:"cited_count"`
	LastAccessAt   time.Time `json:"last_access_at"`
}

type CoverageGap struct {
	MessageID       string    `json:"message_id"`
	SessionID       string    `json:"session_id"`
	Question        string    `json:"question"`
	ConfidenceScore float64   `json:"confidence_score"`
	ConfidenceLabel string    `json:"confidence_label"`
	SourceCount     int64     `json:"source_count"`
	AnswerCreatedAt time.Time `json:"answer_created_at"`
}

type StaleDocument struct {
	KnowledgeID       string     `json:"knowledge_id"`
	Title             string     `json:"title"`
	SourceWeight      float64    `json:"source_weight"`
	FreshnessFlag     bool       `json:"freshness_flag"`
	DownFeedbackCount int64      `json:"down_feedback_count"`
	LastFeedbackAt    *time.Time `json:"last_feedback_at,omitempty"`
}

type CitationHeat struct {
	KnowledgeID    string  `json:"knowledge_id"`
	Title          string  `json:"title"`
	CitedCount     int64   `json:"cited_count"`
	RerankedCount  int64   `json:"reranked_count"`
	RetrievedCount int64   `json:"retrieved_count"`
	SourceWeight   float64 `json:"source_weight"`
	FreshnessFlag  bool    `json:"freshness_flag"`
}
