DO $$ BEGIN RAISE NOTICE '[Migration 000044] Adding external_id column to knowledges'; END $$;

ALTER TABLE knowledges
ADD COLUMN IF NOT EXISTS external_id TEXT;

CREATE INDEX IF NOT EXISTS idx_knowledges_external_id
ON knowledges(external_id)
WHERE external_id IS NOT NULL;

UPDATE knowledges
SET external_id = metadata->>'external_id'
WHERE external_id IS NULL
  AND metadata IS NOT NULL
  AND metadata->>'external_id' IS NOT NULL;

COMMENT ON COLUMN knowledges.external_id IS 'External source item id for datasource sync and stable citation references.';
