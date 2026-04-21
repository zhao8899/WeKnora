DO $$ BEGIN RAISE NOTICE '[Migration 000045] Adding config_encrypted column to data_sources'; END $$;

ALTER TABLE data_sources
ADD COLUMN IF NOT EXISTS config_encrypted TEXT;

COMMENT ON COLUMN data_sources.config_encrypted IS 'AES-256-GCM encrypted datasource config. Preferred over plaintext config when present.';
