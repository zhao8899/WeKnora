DROP INDEX IF EXISTS idx_models_is_platform;
ALTER TABLE models DROP COLUMN IF EXISTS is_platform;
ALTER TABLE tenants DROP COLUMN IF EXISTS token_quota;
ALTER TABLE tenants DROP COLUMN IF EXISTS token_used;
ALTER TABLE tenants DROP COLUMN IF EXISTS quota_reset_at;
