-- Migration: 177_add_sub_admin_permissions
-- Add account-level sub-admin menu permissions. Existing users receive no permissions.

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS admin_permissions JSONB NOT NULL DEFAULT '[]'::jsonb;

UPDATE users
SET admin_permissions = '[]'::jsonb
WHERE admin_permissions IS NULL;
