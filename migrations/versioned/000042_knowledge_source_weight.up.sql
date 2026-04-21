-- Add source weight and freshness tracking columns for knowledges.

ALTER TABLE knowledges
ADD COLUMN IF NOT EXISTS source_weight DOUBLE PRECISION NOT NULL DEFAULT 1.0;

ALTER TABLE knowledges
ADD COLUMN IF NOT EXISTS freshness_flag BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS idx_knowledges_freshness_flag_true
ON knowledges(freshness_flag)
WHERE freshness_flag = TRUE;
