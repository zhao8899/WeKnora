-- Migration 000034: Create audit_logs table for enterprise compliance
-- Tracks key operations: authentication, resource CRUD, configuration changes

DO $$ BEGIN RAISE NOTICE '[Migration 000034] Creating audit_logs table'; END $$;

CREATE TABLE IF NOT EXISTS audit_logs (
    id BIGSERIAL PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    user_id VARCHAR(36),              -- NULL for API-key-only requests
    username VARCHAR(255),            -- Snapshot of user display name at log time
    action VARCHAR(50) NOT NULL,      -- create, read, update, delete, login, logout, export, import
    resource_type VARCHAR(50) NOT NULL, -- knowledge_base, knowledge, session, model, tenant, user, faq, agent, etc.
    resource_id VARCHAR(255),         -- ID of the affected resource
    detail TEXT,                      -- JSON blob with action-specific context (e.g. old/new values)
    ip_address VARCHAR(45),           -- IPv4 or IPv6
    user_agent TEXT,
    request_method VARCHAR(10),       -- GET, POST, PUT, DELETE
    request_path TEXT,
    status_code INT,                  -- HTTP response status
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP NOT NULL
);

-- Index for tenant-scoped queries (most common access pattern)
CREATE INDEX IF NOT EXISTS idx_audit_logs_tenant_created ON audit_logs(tenant_id, created_at DESC);

-- Index for user activity lookup
CREATE INDEX IF NOT EXISTS idx_audit_logs_user ON audit_logs(tenant_id, user_id, created_at DESC) WHERE user_id IS NOT NULL;

-- Index for resource history
CREATE INDEX IF NOT EXISTS idx_audit_logs_resource ON audit_logs(tenant_id, resource_type, resource_id) WHERE resource_id IS NOT NULL;

-- Index for action filtering
CREATE INDEX IF NOT EXISTS idx_audit_logs_action ON audit_logs(tenant_id, action, created_at DESC);

-- Auto-cleanup: partition hint (for future partitioning by month)
COMMENT ON TABLE audit_logs IS 'Enterprise audit trail. Consider partitioning by created_at for large deployments.';

DO $$ BEGIN RAISE NOTICE '[Migration 000034] Audit logs table created'; END $$;
