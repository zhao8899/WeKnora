package types

import (
	"database/sql/driver"
	"encoding/json"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	WikiPageTypeSummary    = "summary"
	WikiPageTypeEntity     = "entity"
	WikiPageTypeConcept    = "concept"
	WikiPageTypeIndex      = "index"
	WikiPageTypeLog        = "log"
	WikiPageTypeSynthesis  = "synthesis"
	WikiPageTypeComparison = "comparison"
)

const (
	WikiPageStatusDraft     = "draft"
	WikiPageStatusPublished = "published"
	WikiPageStatusArchived  = "archived"
)

const (
	WikiDefaultIndexSlug = "index"
	WikiDefaultLogSlug   = "log"
)

type WikiPage struct {
	ID              string         `json:"id" gorm:"type:varchar(36);primaryKey"`
	TenantID        uint64         `json:"tenant_id" gorm:"index"`
	KnowledgeBaseID string         `json:"knowledge_base_id" gorm:"type:varchar(36);index"`
	Slug            string         `json:"slug" gorm:"type:varchar(255);uniqueIndex:idx_kb_slug"`
	Title           string         `json:"title" gorm:"type:varchar(512)"`
	PageType        string         `json:"page_type" gorm:"type:varchar(32);index"`
	Status          string         `json:"status" gorm:"type:varchar(32);default:'published'"`
	Content         string         `json:"content" gorm:"type:text"`
	Summary         string         `json:"summary" gorm:"type:text"`
	Aliases         StringArray    `json:"aliases" gorm:"type:json"`
	SourceRefs      StringArray    `json:"source_refs" gorm:"type:json"`
	ChunkRefs       StringArray    `json:"chunk_refs" gorm:"type:json"`
	InLinks         StringArray    `json:"in_links" gorm:"type:json"`
	OutLinks        StringArray    `json:"out_links" gorm:"type:json"`
	PageMetadata    JSON           `json:"page_metadata" gorm:"column:page_metadata;type:json"`
	Version         int            `json:"version" gorm:"default:1"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `json:"deleted_at" gorm:"index"`
}

func (WikiPage) TableName() string {
	return "wiki_pages"
}

type WikiExtractionGranularity string

const (
	WikiExtractionFocused    WikiExtractionGranularity = "focused"
	WikiExtractionStandard   WikiExtractionGranularity = "standard"
	WikiExtractionExhaustive WikiExtractionGranularity = "exhaustive"
)

func (g WikiExtractionGranularity) IsValid() bool {
	switch g {
	case WikiExtractionFocused, WikiExtractionStandard, WikiExtractionExhaustive:
		return true
	default:
		return false
	}
}

func (g WikiExtractionGranularity) Normalize() WikiExtractionGranularity {
	if g.IsValid() {
		return g
	}
	return WikiExtractionStandard
}

type WikiConfig struct {
	SynthesisModelID      string                    `yaml:"synthesis_model_id" json:"synthesis_model_id"`
	MaxPagesPerIngest     int                       `yaml:"max_pages_per_ingest" json:"max_pages_per_ingest"`
	ExtractionGranularity WikiExtractionGranularity `yaml:"extraction_granularity" json:"extraction_granularity,omitempty"`
}

func (c WikiConfig) Value() (driver.Value, error) {
	return json.Marshal(c)
}

func (c *WikiConfig) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	b, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(b, c)
}

type WikiPageListRequest struct {
	KnowledgeBaseID string `json:"knowledge_base_id"`
	PageType        string `json:"page_type,omitempty"`
	Status          string `json:"status,omitempty"`
	Query           string `json:"query,omitempty"`
	Page            int    `json:"page,omitempty"`
	PageSize        int    `json:"page_size,omitempty"`
	SortBy          string `json:"sort_by,omitempty"`
	SortOrder       string `json:"sort_order,omitempty"`
}

func (r *WikiPageListRequest) Normalize() {
	if r == nil {
		return
	}
	if r.Page < 1 {
		r.Page = 1
	}
	switch {
	case r.PageSize < 1:
		r.PageSize = 20
	case r.PageSize > 100:
		r.PageSize = 100
	}

	r.KnowledgeBaseID = strings.TrimSpace(r.KnowledgeBaseID)
	r.PageType = strings.TrimSpace(r.PageType)
	r.Status = strings.TrimSpace(r.Status)
	r.Query = strings.TrimSpace(r.Query)

	sortBy := strings.ToLower(strings.TrimSpace(r.SortBy))
	switch sortBy {
	case "", "updated_at", "created_at", "title", "page_type", "slug":
		r.SortBy = sortBy
	default:
		r.SortBy = "updated_at"
	}

	sortOrder := strings.ToLower(strings.TrimSpace(r.SortOrder))
	switch sortOrder {
	case "asc", "desc":
		r.SortOrder = sortOrder
	default:
		r.SortOrder = "desc"
	}
}

type WikiPageListResponse struct {
	Pages      []*WikiPage `json:"pages"`
	Total      int64       `json:"total"`
	Page       int         `json:"page"`
	PageSize   int         `json:"page_size"`
	TotalPages int         `json:"total_pages"`
}

type WikiGraphData struct {
	Nodes []WikiGraphNode `json:"nodes"`
	Edges []WikiGraphEdge `json:"edges"`
}

type WikiGraphNode struct {
	Slug      string `json:"slug"`
	Title     string `json:"title"`
	PageType  string `json:"page_type"`
	LinkCount int    `json:"link_count"`
}

type WikiGraphEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

type WikiStats struct {
	TotalPages    int64            `json:"total_pages"`
	PagesByType   map[string]int64 `json:"pages_by_type"`
	TotalLinks    int64            `json:"total_links"`
	OrphanCount   int64            `json:"orphan_count"`
	RecentUpdates []*WikiPage      `json:"recent_updates"`
	PendingTasks  int64            `json:"pending_tasks"`
	PendingIssues int64            `json:"pending_issues"`
	IsActive      bool             `json:"is_active"`
}
