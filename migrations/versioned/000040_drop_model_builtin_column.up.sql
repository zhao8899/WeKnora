-- Remove the legacy builtin flag from models now that runtime governance
-- has fully converged on tenant-owned + platform-shared models.

ALTER TABLE models DROP CONSTRAINT IF EXISTS chk_models_tenant_id_positive;

ALTER TABLE models
ADD CONSTRAINT chk_models_tenant_id_positive
CHECK (tenant_id > 0 OR is_platform = true);

DROP INDEX IF EXISTS idx_models_is_builtin;

ALTER TABLE models DROP COLUMN IF EXISTS is_builtin;
