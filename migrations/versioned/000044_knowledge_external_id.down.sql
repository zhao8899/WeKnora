DROP INDEX IF EXISTS idx_knowledges_external_id;

ALTER TABLE knowledges
DROP COLUMN IF EXISTS external_id;
