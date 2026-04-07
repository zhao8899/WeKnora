-- Add owner_id column to tenants table
-- This establishes the tenant creator as the tenant admin,
-- so they get admin permissions without needing can_access_all_tenants.

ALTER TABLE tenants ADD COLUMN IF NOT EXISTS owner_id VARCHAR(36);
CREATE INDEX IF NOT EXISTS idx_tenants_owner_id ON tenants(owner_id);

-- Backfill: for each tenant, set the earliest registered user as owner
UPDATE tenants t
SET owner_id = sub.first_user_id
FROM (
    SELECT tenant_id, id AS first_user_id
    FROM (
        SELECT tenant_id, id,
               ROW_NUMBER() OVER (PARTITION BY tenant_id ORDER BY created_at ASC) AS rn
        FROM users
        WHERE deleted_at IS NULL
    ) ranked
    WHERE rn = 1
) sub
WHERE t.id = sub.tenant_id
  AND t.owner_id IS NULL;
