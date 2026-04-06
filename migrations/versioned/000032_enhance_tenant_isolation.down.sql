-- Migration 000032: Rollback tenant isolation and JSONB performance enhancements

-- Drop check constraints
ALTER TABLE models DROP CONSTRAINT IF EXISTS chk_models_tenant_id_positive;
ALTER TABLE knowledge_bases DROP CONSTRAINT IF EXISTS chk_kb_tenant_id_positive;
ALTER TABLE knowledges DROP CONSTRAINT IF EXISTS chk_knowledge_tenant_id_positive;

-- Drop composite indexes
DROP INDEX IF EXISTS idx_knowledge_bases_tenant_type;
DROP INDEX IF EXISTS idx_knowledges_tenant_kb;
DROP INDEX IF EXISTS idx_sessions_tenant_user;
DROP INDEX IF EXISTS idx_messages_session_created;
DROP INDEX IF EXISTS idx_models_tenant_type_default;
DROP INDEX IF EXISTS idx_custom_agents_tenant_builtin;

-- Drop GIN indexes on JSONB columns
DROP INDEX IF EXISTS idx_models_parameters_gin;
DROP INDEX IF EXISTS idx_knowledge_bases_chunking_config_gin;
DROP INDEX IF EXISTS idx_knowledge_bases_image_processing_config_gin;
DROP INDEX IF EXISTS idx_knowledge_bases_vlm_config_gin;
DROP INDEX IF EXISTS idx_knowledge_bases_asr_config_gin;
DROP INDEX IF EXISTS idx_knowledge_bases_storage_config_gin;
DROP INDEX IF EXISTS idx_knowledge_bases_extract_config_gin;
DROP INDEX IF EXISTS idx_knowledge_bases_faq_config_gin;
DROP INDEX IF EXISTS idx_knowledge_bases_question_generation_config_gin;
DROP INDEX IF EXISTS idx_knowledge_metadata_gin;
DROP INDEX IF EXISTS idx_sessions_agent_config_gin;
DROP INDEX IF EXISTS idx_sessions_context_config_gin;
DROP INDEX IF EXISTS idx_custom_agents_config_gin;
DROP INDEX IF EXISTS idx_mcp_services_headers_gin;
DROP INDEX IF EXISTS idx_mcp_services_auth_config_gin;
DROP INDEX IF EXISTS idx_web_search_providers_parameters_gin;

-- Revert tenant_id columns back to INTEGER
-- Note: This will fail if any tenant_id exceeds INTEGER range
ALTER TABLE users ALTER COLUMN tenant_id TYPE INTEGER;
ALTER TABLE models ALTER COLUMN tenant_id TYPE INTEGER;
ALTER TABLE knowledge_bases ALTER COLUMN tenant_id TYPE INTEGER;
ALTER TABLE knowledges ALTER COLUMN tenant_id TYPE INTEGER;
ALTER TABLE chunks ALTER COLUMN tenant_id TYPE INTEGER;
ALTER TABLE sessions ALTER COLUMN tenant_id TYPE INTEGER;
ALTER TABLE messages ALTER COLUMN tenant_id TYPE INTEGER;
ALTER TABLE custom_agents ALTER COLUMN tenant_id TYPE INTEGER;
ALTER TABLE mcp_services ALTER COLUMN tenant_id TYPE INTEGER;
ALTER TABLE knowledge_tags ALTER COLUMN tenant_id TYPE INTEGER;
ALTER TABLE data_sources ALTER COLUMN tenant_id TYPE INTEGER;
ALTER TABLE web_search_providers ALTER COLUMN tenant_id TYPE INTEGER;
ALTER TABLE im_channels ALTER COLUMN tenant_id TYPE INTEGER;
ALTER TABLE im_channel_sessions ALTER COLUMN tenant_id TYPE INTEGER;
ALTER TABLE agent_shares ALTER COLUMN tenant_id TYPE INTEGER;
ALTER TABLE kb_shares ALTER COLUMN tenant_id TYPE INTEGER;
ALTER TABLE organization_members ALTER COLUMN tenant_id TYPE INTEGER;
ALTER TABLE embeddings ALTER COLUMN tenant_id TYPE INTEGER;
