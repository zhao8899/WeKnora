-- Decouple global/shared model validity from the legacy builtin flag.
-- Future global models should be valid when marked as platform-shared.

ALTER TABLE models DROP CONSTRAINT IF EXISTS chk_models_tenant_id_positive;

ALTER TABLE models
ADD CONSTRAINT chk_models_tenant_id_positive
CHECK (tenant_id > 0 OR is_platform = true OR is_builtin = true);
