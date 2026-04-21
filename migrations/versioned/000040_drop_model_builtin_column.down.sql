-- Restore the legacy builtin flag on models.

ALTER TABLE models ADD COLUMN IF NOT EXISTS is_builtin BOOLEAN NOT NULL DEFAULT FALSE;
CREATE INDEX IF NOT EXISTS idx_models_is_builtin ON models(is_builtin);

ALTER TABLE models DROP CONSTRAINT IF EXISTS chk_models_tenant_id_positive;

ALTER TABLE models
ADD CONSTRAINT chk_models_tenant_id_positive
CHECK (tenant_id > 0 OR is_platform = true OR is_builtin = true);
