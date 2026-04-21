DROP INDEX IF EXISTS idx_knowledges_freshness_flag_true;

ALTER TABLE knowledges
DROP COLUMN IF EXISTS freshness_flag;

ALTER TABLE knowledges
DROP COLUMN IF EXISTS source_weight;
