-- Add document access logs for analytics and dashboard aggregation.

CREATE TABLE IF NOT EXISTS document_access_logs (
    id           VARCHAR(36) PRIMARY KEY,
    tenant_id    BIGINT NOT NULL,
    knowledge_id VARCHAR(36),
    session_id   VARCHAR(36) NOT NULL,
    message_id   VARCHAR(36) NOT NULL,
    access_type  VARCHAR(20) NOT NULL CHECK (access_type IN ('retrieved', 'reranked', 'cited')),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_document_access_logs_tenant_created_at
ON document_access_logs(tenant_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_document_access_logs_knowledge_id
ON document_access_logs(knowledge_id);

CREATE INDEX IF NOT EXISTS idx_document_access_logs_access_type
ON document_access_logs(access_type);

CREATE INDEX IF NOT EXISTS idx_document_access_logs_message_id
ON document_access_logs(message_id);
