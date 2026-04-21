-- Migration: 000032_answer_evidence
-- Description: Create answer_evidence table for answer/source traceability
DO $$ BEGIN RAISE NOTICE '[Migration 000032] Creating answer_evidence table'; END $$;

CREATE TABLE IF NOT EXISTS answer_evidence (
    id VARCHAR(36) NOT NULL PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    session_id VARCHAR(36) NOT NULL,
    answer_message_id VARCHAR(36) NOT NULL,
    source_knowledge_id VARCHAR(36) NOT NULL,
    source_knowledge_base_id VARCHAR(36),
    source_chunk_id VARCHAR(255),
    source_title VARCHAR(255),
    source_type VARCHAR(50) NOT NULL,
    source_channel VARCHAR(50) DEFAULT '',
    match_type VARCHAR(50) NOT NULL,
    retrieval_score DOUBLE PRECISION NOT NULL DEFAULT 0,
    rerank_score DOUBLE PRECISION NOT NULL DEFAULT 0,
    position INT NOT NULL DEFAULT 0,
    source_snapshot JSONB,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_answer_evidence_tenant_message ON answer_evidence (tenant_id, answer_message_id);
CREATE INDEX IF NOT EXISTS idx_answer_evidence_message_position ON answer_evidence (answer_message_id, position);
CREATE INDEX IF NOT EXISTS idx_answer_evidence_source_knowledge ON answer_evidence (source_knowledge_id);
CREATE INDEX IF NOT EXISTS idx_answer_evidence_source_kb ON answer_evidence (source_knowledge_base_id);

DO $$ BEGIN RAISE NOTICE '[Migration 000032] answer_evidence table created successfully'; END $$;
