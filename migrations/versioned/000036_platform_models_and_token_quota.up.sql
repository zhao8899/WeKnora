-- 1. Add is_platform flag to models table
ALTER TABLE models ADD COLUMN IF NOT EXISTS is_platform BOOLEAN NOT NULL DEFAULT FALSE;
CREATE INDEX IF NOT EXISTS idx_models_is_platform ON models(is_platform) WHERE is_platform = true;

-- 2. Add token quota fields to tenants table
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS token_quota BIGINT NOT NULL DEFAULT 0;
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS token_used BIGINT NOT NULL DEFAULT 0;
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS quota_reset_at TIMESTAMP WITH TIME ZONE;

-- 3. Upgrade existing is_builtin models that have real API keys to is_platform
UPDATE models SET is_platform = true
WHERE is_builtin = true
  AND deleted_at IS NULL
  AND parameters::text LIKE '%"api_key"%'
  AND parameters->>'api_key' != '';
