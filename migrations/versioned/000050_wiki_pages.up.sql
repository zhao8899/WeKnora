-- Migration: 000050_wiki_pages
-- Description: Wiki page storage and per-KB wiki configuration.
DO $$ BEGIN RAISE NOTICE '[Migration 000050] Applying wiki page schema'; END $$;

ALTER TABLE knowledge_bases ADD COLUMN IF NOT EXISTS wiki_config JSONB;

COMMENT ON COLUMN knowledge_bases.wiki_config IS
    'Wiki configuration: {"synthesis_model_id": string, "max_pages_per_ingest": int, "extraction_granularity": string}';

CREATE TABLE IF NOT EXISTS wiki_pages (
    id                VARCHAR(36) PRIMARY KEY,
    tenant_id         BIGINT NOT NULL,
    knowledge_base_id VARCHAR(36) NOT NULL,
    slug              VARCHAR(255) NOT NULL,
    title             VARCHAR(512) NOT NULL DEFAULT '',
    page_type         VARCHAR(32) NOT NULL DEFAULT 'summary',
    status            VARCHAR(32) NOT NULL DEFAULT 'published',
    content           TEXT NOT NULL DEFAULT '',
    summary           TEXT NOT NULL DEFAULT '',
    aliases           JSONB DEFAULT '[]'::JSONB,
    source_refs       JSONB DEFAULT '[]'::JSONB,
    chunk_refs        JSONB DEFAULT '[]'::JSONB,
    in_links          JSONB DEFAULT '[]'::JSONB,
    out_links         JSONB DEFAULT '[]'::JSONB,
    page_metadata     JSONB DEFAULT '{}'::JSONB,
    version           INT NOT NULL DEFAULT 1,
    created_at        TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    deleted_at        TIMESTAMP WITH TIME ZONE
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_wiki_pages_kb_slug
    ON wiki_pages (knowledge_base_id, slug)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_wiki_pages_kb_id
    ON wiki_pages (knowledge_base_id);

CREATE INDEX IF NOT EXISTS idx_wiki_pages_page_type
    ON wiki_pages (knowledge_base_id, page_type);

CREATE INDEX IF NOT EXISTS idx_wiki_pages_tenant_id
    ON wiki_pages (tenant_id);

CREATE INDEX IF NOT EXISTS idx_wiki_pages_deleted_at
    ON wiki_pages (deleted_at);

CREATE INDEX IF NOT EXISTS idx_wiki_pages_fulltext
    ON wiki_pages USING GIN (to_tsvector('simple', coalesce(title, '') || ' ' || coalesce(content, '')));

CREATE TABLE IF NOT EXISTS wiki_page_issues (
    id                       VARCHAR(36) PRIMARY KEY,
    tenant_id                BIGINT NOT NULL,
    knowledge_base_id         VARCHAR(36) NOT NULL,
    slug                     VARCHAR(255) NOT NULL,
    issue_type               VARCHAR(50) NOT NULL,
    description              TEXT NOT NULL,
    suspected_knowledge_ids  JSONB,
    status                   VARCHAR(20) DEFAULT 'pending' NOT NULL,
    reported_by              VARCHAR(100) NOT NULL,
    created_at               TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at               TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    deleted_at               TIMESTAMP WITH TIME ZONE
);

CREATE INDEX IF NOT EXISTS idx_wiki_page_issues_tenant_id ON wiki_page_issues(tenant_id);
CREATE INDEX IF NOT EXISTS idx_wiki_page_issues_knowledge_base_id ON wiki_page_issues(knowledge_base_id);
CREATE INDEX IF NOT EXISTS idx_wiki_page_issues_slug ON wiki_page_issues(slug);
CREATE INDEX IF NOT EXISTS idx_wiki_page_issues_status ON wiki_page_issues(status);

DO $$ BEGIN RAISE NOTICE '[Migration 000050] wiki page schema applied successfully'; END $$;
