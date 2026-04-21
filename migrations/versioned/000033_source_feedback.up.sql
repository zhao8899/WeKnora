-- Migration: 000033_source_feedback
-- Description: Create source_feedback table for source-level user feedback
DO $$ BEGIN RAISE NOTICE '[Migration 000033] Creating source_feedback table'; END $$;

CREATE TABLE IF NOT EXISTS source_feedback (
    id VARCHAR(36) NOT NULL PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    answer_message_id VARCHAR(36) NOT NULL,
    answer_evidence_id VARCHAR(36) NOT NULL,
    user_id VARCHAR(64) NOT NULL DEFAULT '',
    feedback VARCHAR(16) NOT NULL,
    comment TEXT DEFAULT '',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uq_source_feedback_actor UNIQUE (answer_message_id, answer_evidence_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_source_feedback_tenant_message ON source_feedback (tenant_id, answer_message_id);
CREATE INDEX IF NOT EXISTS idx_source_feedback_evidence ON source_feedback (answer_evidence_id);

DO $$ BEGIN RAISE NOTICE '[Migration 000033] source_feedback table created successfully'; END $$;
