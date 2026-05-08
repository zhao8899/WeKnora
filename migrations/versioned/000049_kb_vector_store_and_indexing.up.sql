-- Migration: 000049_kb_vector_store_and_indexing
-- Description: Add per-KB vector store binding and indexing strategy.

ALTER TABLE knowledge_bases ADD COLUMN IF NOT EXISTS vector_store_id VARCHAR(36);
ALTER TABLE knowledge_bases ADD COLUMN IF NOT EXISTS indexing_strategy JSONB;

COMMENT ON COLUMN knowledge_bases.vector_store_id IS
    'References vector_stores.id or an environment vector store id. NULL means tenant/default retriever settings.';

COMMENT ON COLUMN knowledge_bases.indexing_strategy IS
    'Indexing pipelines strategy: {"vector_enabled": bool, "keyword_enabled": bool, "wiki_enabled": bool, "graph_enabled": bool}';

CREATE INDEX IF NOT EXISTS idx_knowledge_bases_tenant_vector_store
    ON knowledge_bases(tenant_id, vector_store_id);

UPDATE knowledge_bases
SET indexing_strategy = jsonb_build_object(
    'vector_enabled',  TRUE,
    'keyword_enabled', TRUE,
    'wiki_enabled',    FALSE,
    'graph_enabled',   FALSE
)
WHERE indexing_strategy IS NULL;
