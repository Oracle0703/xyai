-- Organization leaders may assign only subscription groups scoped to one of
-- their organizations (docs/features/organization-leader-design-cn.md D13).
-- A group without rows here is assignable by full administrators and full-site
-- subscription sub-admins only. Existing groups start unscoped.
CREATE TABLE IF NOT EXISTS group_organization_scopes (
    group_id BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    organization_key VARCHAR(20) NOT NULL CHECK (organization_key IN ('xunyou', 'wsdashi')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (group_id, organization_key)
);
CREATE INDEX IF NOT EXISTS group_organization_scopes_organization_idx ON group_organization_scopes (organization_key, group_id);
