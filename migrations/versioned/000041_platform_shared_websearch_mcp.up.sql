-- Add platform-shared flags for web search providers and MCP services

ALTER TABLE web_search_providers
ADD COLUMN IF NOT EXISTS is_platform BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS idx_web_search_providers_is_platform
ON web_search_providers(is_platform)
WHERE is_platform = true;

ALTER TABLE mcp_services
ADD COLUMN IF NOT EXISTS is_platform BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS idx_mcp_services_is_platform
ON mcp_services(is_platform)
WHERE is_platform = true;
