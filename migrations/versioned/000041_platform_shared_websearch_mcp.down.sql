DROP INDEX IF EXISTS idx_web_search_providers_is_platform;
ALTER TABLE web_search_providers DROP COLUMN IF EXISTS is_platform;

DROP INDEX IF EXISTS idx_mcp_services_is_platform;
ALTER TABLE mcp_services DROP COLUMN IF EXISTS is_platform;
