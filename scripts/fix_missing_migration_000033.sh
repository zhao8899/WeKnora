#!/bin/bash
# Fix missing migration 000033 for chunks table
# This script adds the missing standard_question and has_generated_questions columns

set -e

echo "Applying missing migration 000033 to chunks table..."

docker exec WeKnora-postgres psql -U postgres -d WeKnora -c "
-- Add standard_question column (extracted from metadata->>'standard_question')
ALTER TABLE chunks ADD COLUMN IF NOT EXISTS standard_question TEXT;

-- Add has_generated_questions flag (materialized from jsonb_array_length(metadata->'generated_questions') > 0)
ALTER TABLE chunks ADD COLUMN IF NOT EXISTS has_generated_questions BOOLEAN NOT NULL DEFAULT false;

-- Create indexes for the new columns
CREATE INDEX IF NOT EXISTS idx_chunks_standard_question ON chunks(standard_question) WHERE standard_question IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_chunks_has_generated_questions ON chunks(knowledge_base_id, has_generated_questions) WHERE has_generated_questions = true;
CREATE INDEX IF NOT EXISTS idx_chunks_faq_question_lookup ON chunks(tenant_id, knowledge_base_id, standard_question) WHERE standard_question IS NOT NULL;

-- Backfill standard_question from existing FAQ metadata
UPDATE chunks
SET standard_question = metadata->>'standard_question'
WHERE metadata IS NOT NULL
  AND metadata->>'standard_question' IS NOT NULL
  AND metadata->>'standard_question' != ''
  AND standard_question IS NULL;

-- Backfill has_generated_questions from existing metadata
UPDATE chunks
SET has_generated_questions = true
WHERE metadata IS NOT NULL
  AND metadata::text != '{}'
  AND jsonb_array_length(COALESCE(metadata->'generated_questions', '[]'::jsonb)) > 0
  AND has_generated_questions = false;

-- Update schema_migrations to mark migration 33 as applied
UPDATE schema_migrations SET version = 33, dirty = false WHERE version <= 32 AND dirty = true;
INSERT INTO schema_migrations (version, dirty) 
SELECT 33, false 
WHERE NOT EXISTS (SELECT 1 FROM schema_migrations WHERE version = 33);
"

echo "Migration 000033 applied successfully!"
