-- Migration 000032: Enhance tenant isolation and JSONB query performance
-- 1. Convert tenant_id from INTEGER to BIGINT across all tables
-- 2. Add GIN indexes on JSONB columns for faster queries
-- 3. Add composite indexes for common query patterns

-- Convert tenant_id columns from INTEGER to BIGINT
-- This is safe because BIGINT is a superset of INTEGER
ALTER TABLE users ALTER COLUMN tenant_id TYPE BIGINT;
ALTER TABLE models ALTER COLUMN tenant_id TYPE BIGINT;
ALTER TABLE knowledge_bases ALTER COLUMN tenant_id TYPE BIGINT;
ALTER TABLE knowledges ALTER COLUMN tenant_id TYPE BIGINT;
ALTER TABLE chunks ALTER COLUMN tenant_id TYPE BIGINT;
ALTER TABLE sessions ALTER COLUMN tenant_id TYPE BIGINT;
ALTER TABLE messages ALTER COLUMN tenant_id TYPE BIGINT;
ALTER TABLE custom_agents ALTER COLUMN tenant_id TYPE BIGINT;
ALTER TABLE mcp_services ALTER COLUMN tenant_id TYPE BIGINT;
ALTER TABLE knowledge_tags ALTER COLUMN tenant_id TYPE BIGINT;
ALTER TABLE data_sources ALTER COLUMN tenant_id TYPE BIGINT;
ALTER TABLE web_search_providers ALTER COLUMN tenant_id TYPE BIGINT;
ALTER TABLE im_channels ALTER COLUMN tenant_id TYPE BIGINT;
ALTER TABLE im_channel_sessions ALTER COLUMN tenant_id TYPE BIGINT;
ALTER TABLE organization_members ALTER COLUMN tenant_id TYPE BIGINT;

-- Add GIN indexes on JSONB columns for faster containment queries
CREATE INDEX IF NOT EXISTS idx_models_parameters_gin ON models USING GIN (parameters);
CREATE INDEX IF NOT EXISTS idx_knowledge_bases_chunking_config_gin ON knowledge_bases USING GIN (chunking_config);
CREATE INDEX IF NOT EXISTS idx_knowledge_bases_image_processing_config_gin ON knowledge_bases USING GIN (image_processing_config);
CREATE INDEX IF NOT EXISTS idx_knowledge_bases_vlm_config_gin ON knowledge_bases USING GIN (vlm_config);
CREATE INDEX IF NOT EXISTS idx_knowledge_bases_asr_config_gin ON knowledge_bases USING GIN (asr_config);
CREATE INDEX IF NOT EXISTS idx_knowledge_bases_storage_config_gin ON knowledge_bases USING GIN (storage_config);
CREATE INDEX IF NOT EXISTS idx_knowledge_bases_extract_config_gin ON knowledge_bases USING GIN (extract_config);
CREATE INDEX IF NOT EXISTS idx_knowledge_bases_faq_config_gin ON knowledge_bases USING GIN (faq_config);
CREATE INDEX IF NOT EXISTS idx_knowledge_bases_question_generation_config_gin ON knowledge_bases USING GIN (question_generation_config);
CREATE INDEX IF NOT EXISTS idx_knowledge_metadata_gin ON knowledges USING GIN (metadata);
CREATE INDEX IF NOT EXISTS idx_sessions_agent_config_gin ON sessions USING GIN (agent_config);
CREATE INDEX IF NOT EXISTS idx_sessions_context_config_gin ON sessions USING GIN (context_config);
CREATE INDEX IF NOT EXISTS idx_custom_agents_config_gin ON custom_agents USING GIN (config);
CREATE INDEX IF NOT EXISTS idx_mcp_services_headers_gin ON mcp_services USING GIN (headers);
CREATE INDEX IF NOT EXISTS idx_mcp_services_auth_config_gin ON mcp_services USING GIN (auth_config);
CREATE INDEX IF NOT EXISTS idx_web_search_providers_parameters_gin ON web_search_providers USING GIN (parameters);

-- Add composite indexes for common query patterns
CREATE INDEX IF NOT EXISTS idx_knowledge_bases_tenant_type ON knowledge_bases (tenant_id, type) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_knowledges_tenant_kb ON knowledges (tenant_id, knowledge_base_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_sessions_tenant_user ON sessions (tenant_id, user_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_messages_session_created ON messages (session_id, created_at);
CREATE INDEX IF NOT EXISTS idx_models_tenant_type_default ON models (tenant_id, type, is_default) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_custom_agents_tenant_builtin ON custom_agents (tenant_id, is_builtin);

-- Add check constraints for data integrity
ALTER TABLE models ADD CONSTRAINT chk_models_tenant_id_positive CHECK (tenant_id > 0 OR is_builtin = true);
ALTER TABLE knowledge_bases ADD CONSTRAINT chk_kb_tenant_id_positive CHECK (tenant_id > 0);
ALTER TABLE knowledges ADD CONSTRAINT chk_knowledge_tenant_id_positive CHECK (tenant_id > 0);
