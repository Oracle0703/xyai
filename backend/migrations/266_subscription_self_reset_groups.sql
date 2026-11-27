-- Per-organization self-reset allowlist. The table intentionally starts empty
-- so the extension is fail-closed until an administrator selects groups.
CREATE TABLE IF NOT EXISTS subscription_self_reset_groups (
    group_id BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    organization_key VARCHAR(20) NOT NULL CHECK (organization_key IN ('xunyou', 'wsdashi', 'other')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (group_id, organization_key)
);

-- A group that leaves subscription mode must not regain an old allowlist entry
-- if it is changed back later. Soft-deleted groups remain filtered at read time
-- and are reported by the operational inspection query.
CREATE OR REPLACE FUNCTION clear_self_reset_groups_on_type_change()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    DELETE FROM subscription_self_reset_groups WHERE group_id = NEW.id;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS groups_clear_self_reset_on_type_change ON groups;
CREATE TRIGGER groups_clear_self_reset_on_type_change
AFTER UPDATE OF subscription_type ON groups
FOR EACH ROW
WHEN (OLD.subscription_type = 'subscription' AND NEW.subscription_type IS DISTINCT FROM 'subscription')
EXECUTE FUNCTION clear_self_reset_groups_on_type_change();
