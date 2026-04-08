-- Restore the previous constraint that only allowed zero-tenant global models
-- when they were marked as legacy builtin.

ALTER TABLE models DROP CONSTRAINT IF EXISTS chk_models_tenant_id_positive;

ALTER TABLE models
ADD CONSTRAINT chk_models_tenant_id_positive
CHECK (tenant_id > 0 OR is_builtin = true);
