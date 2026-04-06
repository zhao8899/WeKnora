package types

import "time"

// AuditAction defines the type of audited operation.
type AuditAction string

const (
	AuditActionCreate AuditAction = "create"
	AuditActionRead   AuditAction = "read"
	AuditActionUpdate AuditAction = "update"
	AuditActionDelete AuditAction = "delete"
	AuditActionLogin  AuditAction = "login"
	AuditActionLogout AuditAction = "logout"
	AuditActionExport AuditAction = "export"
	AuditActionImport AuditAction = "import"
)

// AuditResourceType identifies the kind of resource being audited.
type AuditResourceType string

const (
	AuditResourceKnowledgeBase AuditResourceType = "knowledge_base"
	AuditResourceKnowledge     AuditResourceType = "knowledge"
	AuditResourceFAQ           AuditResourceType = "faq"
	AuditResourceSession       AuditResourceType = "session"
	AuditResourceModel         AuditResourceType = "model"
	AuditResourceTenant        AuditResourceType = "tenant"
	AuditResourceUser          AuditResourceType = "user"
	AuditResourceAgent         AuditResourceType = "agent"
	AuditResourceMCPService    AuditResourceType = "mcp_service"
	AuditResourceIMChannel     AuditResourceType = "im_channel"
	AuditResourceDataSource    AuditResourceType = "data_source"
)

// AuditLog represents a single audit trail entry.
type AuditLog struct {
	ID            int64             `json:"id"              gorm:"primaryKey;autoIncrement"`
	TenantID      uint64            `json:"tenant_id"       gorm:"index"`
	UserID        string            `json:"user_id,omitempty"`
	Username      string            `json:"username,omitempty"`
	Action        AuditAction       `json:"action"          gorm:"type:varchar(50)"`
	ResourceType  AuditResourceType `json:"resource_type"   gorm:"type:varchar(50)"`
	ResourceID    string            `json:"resource_id,omitempty"`
	Detail        string            `json:"detail,omitempty" gorm:"type:text"`
	IPAddress     string            `json:"ip_address,omitempty" gorm:"type:varchar(45)"`
	UserAgent     string            `json:"user_agent,omitempty" gorm:"type:text"`
	RequestMethod string            `json:"request_method,omitempty" gorm:"type:varchar(10)"`
	RequestPath   string            `json:"request_path,omitempty" gorm:"type:text"`
	StatusCode    int               `json:"status_code,omitempty"`
	CreatedAt     time.Time         `json:"created_at"      gorm:"autoCreateTime"`
}

// TableName returns the database table name for GORM.
func (AuditLog) TableName() string { return "audit_logs" }

// AuditLogQuery defines filters for listing audit logs.
type AuditLogQuery struct {
	TenantID     uint64
	UserID       string
	Action       AuditAction
	ResourceType AuditResourceType
	ResourceID   string
	StartTime    *time.Time
	EndTime      *time.Time
	Page         int
	PageSize     int
}
