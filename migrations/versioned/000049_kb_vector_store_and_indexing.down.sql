DROP INDEX IF EXISTS idx_knowledge_bases_tenant_vector_store;
ALTER TABLE knowledge_bases DROP COLUMN IF EXISTS indexing_strategy;
ALTER TABLE knowledge_bases DROP COLUMN IF EXISTS vector_store_id;
