-- Migration: 000048_vector_stores
-- Description: Create tenant-scoped vector store configuration table.

CREATE TABLE IF NOT EXISTS vector_stores (
    id                VARCHAR(36)  NOT NULL PRIMARY KEY,
    name              VARCHAR(255) NOT NULL,
    engine_type       VARCHAR(50)  NOT NULL,
    connection_config JSONB        NOT NULL DEFAULT '{}',
    index_config      JSONB        NOT NULL DEFAULT '{}',
    tenant_id         BIGINT       NOT NULL,
    created_at        TIMESTAMP    DEFAULT CURRENT_TIMESTAMP,
    updated_at        TIMESTAMP    DEFAULT CURRENT_TIMESTAMP,
    deleted_at        TIMESTAMP    NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_vector_stores_name_tenant
    ON vector_stores(name, tenant_id)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_vector_stores_tenant_id ON vector_stores(tenant_id);
CREATE INDEX IF NOT EXISTS idx_vector_stores_engine_type ON vector_stores(engine_type);
CREATE INDEX IF NOT EXISTS idx_vector_stores_deleted_at ON vector_stores(deleted_at);
