-- Migration 000033: Extract high-frequency JSONB fields from chunks.metadata into dedicated columns
-- Purpose: Improve FAQ search and question generation query performance
-- The GIN index on metadata helps @> containment queries, but ->> extraction,
-- ILIKE, IN, and jsonb_array_length() expressions need B-tree or dedicated columns.

DO $$ BEGIN RAISE NOTICE '[Migration 000033] Adding dedicated FAQ columns to chunks table'; END $$;

-- 1. Add standard_question column (extracted from metadata->>'standard_question')
-- Only populated for FAQ chunks, NULL for document chunks.
ALTER TABLE chunks ADD COLUMN IF NOT EXISTS standard_question TEXT;

-- 2. Add has_generated_questions flag (materialized from jsonb_array_length(metadata->'generated_questions') > 0)
ALTER TABLE chunks ADD COLUMN IF NOT EXISTS has_generated_questions BOOLEAN NOT NULL DEFAULT false;

-- 3. Create indexes for the new columns
-- B-tree on standard_question for exact match and ILIKE with pg_trgm
CREATE INDEX IF NOT EXISTS idx_chunks_standard_question ON chunks(standard_question) WHERE standard_question IS NOT NULL;

-- Partial index for chunks with generated questions (speeds up "find chunks that have generated questions")
CREATE INDEX IF NOT EXISTS idx_chunks_has_generated_questions ON chunks(knowledge_base_id, has_generated_questions) WHERE has_generated_questions = true;

-- Composite index for FAQ duplicate detection: tenant + knowledge_base + standard_question
CREATE INDEX IF NOT EXISTS idx_chunks_faq_question_lookup ON chunks(tenant_id, knowledge_base_id, standard_question) WHERE standard_question IS NOT NULL;

-- 4. Backfill standard_question from existing FAQ metadata
UPDATE chunks
SET standard_question = metadata->>'standard_question'
WHERE metadata IS NOT NULL
  AND metadata->>'standard_question' IS NOT NULL
  AND metadata->>'standard_question' != ''
  AND standard_question IS NULL;

-- 5. Backfill has_generated_questions from existing metadata
UPDATE chunks
SET has_generated_questions = true
WHERE metadata IS NOT NULL
  AND metadata::text != '{}'
  AND jsonb_array_length(COALESCE(metadata->'generated_questions', '[]'::jsonb)) > 0
  AND has_generated_questions = false;

DO $$ BEGIN RAISE NOTICE '[Migration 000033] FAQ columns migration completed'; END $$;
