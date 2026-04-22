package types

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	SourceTypeDocument = "document"
	SourceTypeFAQ      = "faq"
	SourceTypeWeb      = "web"

	SourceFeedbackUp      = "up"
	SourceFeedbackDown    = "down"
	SourceFeedbackExpired = "expired"
)

// AnswerEvidence stores which sources supported an assistant answer.
type AnswerEvidence struct {
	ID                    string    `json:"id" gorm:"type:varchar(36);primaryKey"`
	TenantID              uint64    `json:"tenant_id" gorm:"index"`
	SessionID             string    `json:"session_id" gorm:"type:varchar(36);index"`
	AnswerMessageID       string    `json:"answer_message_id" gorm:"type:varchar(36);index"`
	SourceKnowledgeID     string    `json:"source_knowledge_id" gorm:"type:varchar(36);index"`
	SourceKnowledgeBaseID string    `json:"source_knowledge_base_id,omitempty" gorm:"type:varchar(36);index"`
	SourceChunkID         string    `json:"source_chunk_id,omitempty" gorm:"type:varchar(255)"`
	SourceTitle           string    `json:"source_title,omitempty" gorm:"type:varchar(255)"`
	SourceType            string    `json:"source_type" gorm:"type:varchar(50)"`
	SourceChannel         string    `json:"source_channel,omitempty" gorm:"type:varchar(50);default:''"`
	MatchType             string    `json:"match_type" gorm:"type:varchar(50)"`
	RetrievalScore        float64   `json:"retrieval_score"`
	RerankScore           float64   `json:"rerank_score"`
	Position              int       `json:"position"`
	SourceSnapshot        JSON      `json:"source_snapshot,omitempty" gorm:"type:jsonb"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

func (AnswerEvidence) TableName() string {
	return "answer_evidence"
}

func (a *AnswerEvidence) BeforeCreate(tx *gorm.DB) error {
	if a.ID == "" {
		a.ID = uuid.New().String()
	}
	return nil
}

// SourceFeedback stores user feedback for a cited source.
type SourceFeedback struct {
	ID               string    `json:"id" gorm:"type:varchar(36);primaryKey"`
	TenantID         uint64    `json:"tenant_id" gorm:"index"`
	AnswerMessageID  string    `json:"answer_message_id" gorm:"type:varchar(36);index"`
	AnswerEvidenceID string    `json:"answer_evidence_id" gorm:"type:varchar(36);index"`
	UserID           string    `json:"user_id" gorm:"type:varchar(64);default:''"`
	Feedback         string    `json:"feedback" gorm:"type:varchar(16)"`
	Comment          string    `json:"comment,omitempty" gorm:"type:text;default:''"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func (SourceFeedback) TableName() string {
	return "source_feedback"
}

func (s *SourceFeedback) BeforeCreate(tx *gorm.DB) error {
	if s.ID == "" {
		s.ID = uuid.New().String()
	}
	return nil
}

type AnswerConfidenceEvidenceItem struct {
	ID              string  `json:"id"`
	KnowledgeID     string  `json:"knowledge_id"`
	KnowledgeBaseID string  `json:"knowledge_base_id,omitempty"`
	ChunkID         string  `json:"chunk_id,omitempty"`
	Title           string  `json:"title,omitempty"`
	SourceType      string  `json:"source_type"`
	SourceChannel   string  `json:"source_channel,omitempty"`
	MatchType       string  `json:"match_type"`
	RetrievalScore  float64 `json:"retrieval_score"`
	RerankScore     float64 `json:"rerank_score"`
	Position        int     `json:"position"`
	CurrentFeedback string  `json:"current_feedback,omitempty"`
}

type AnswerConfidenceResponse struct {
	MessageID             string                          `json:"message_id"`
	ConfidenceScore       float64                         `json:"confidence_score"`
	ConfidenceLabel       string                          `json:"confidence_label"`
	EvidenceStrengthScore float64                         `json:"evidence_strength_score"`
	EvidenceStrengthLabel string                          `json:"evidence_strength_label"`
	SourceHealthScore     float64                         `json:"source_health_score"`
	SourceHealthLabel     string                          `json:"source_health_label"`
	SourceCount           int                             `json:"source_count"`
	ReferenceCount        int                             `json:"reference_count"`
	EvidenceStatus        string                          `json:"evidence_status"`
	SourceTypeCounts      map[string]int                  `json:"source_type_counts"`
	Evidences             []*AnswerConfidenceEvidenceItem `json:"evidences"`
}
