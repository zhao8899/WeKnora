-- Migration: 000050_wiki_pages (rollback)
DO $$ BEGIN RAISE NOTICE '[Migration 000050 DOWN] Reverting wiki page schema'; END $$;

DROP TABLE IF EXISTS wiki_page_issues;
DROP TABLE IF EXISTS wiki_pages;

ALTER TABLE knowledge_bases DROP COLUMN IF EXISTS wiki_config;

DO $$ BEGIN RAISE NOTICE '[Migration 000050 DOWN] wiki page schema reverted successfully'; END $$;
