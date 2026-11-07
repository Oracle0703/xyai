-- Group organization scopes only apply to subscription groups
-- (docs/features/organization-leader-design-cn.md D13). The groups page hides
-- the scope of non-subscription groups, so a scope left behind when a group
-- becomes standard would be invisible and would grant organization leaders
-- access again as soon as the group turns back into a subscription group.
-- Clear the scope whenever subscription_type leaves 'subscription'. The UPDATE
-- holds the group row lock, which subscription writes share-lock before they
-- read the scope (design K22), so no write can see a half-applied change.
DELETE FROM group_organization_scopes gos
USING groups g
WHERE g.id = gos.group_id
  AND g.subscription_type <> 'subscription';

CREATE OR REPLACE FUNCTION clear_group_organization_scopes_on_type_change()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    DELETE FROM group_organization_scopes WHERE group_id = NEW.id;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS groups_clear_organization_scopes_on_type_change ON groups;
CREATE TRIGGER groups_clear_organization_scopes_on_type_change
AFTER UPDATE OF subscription_type ON groups
FOR EACH ROW
WHEN (OLD.subscription_type = 'subscription' AND NEW.subscription_type IS DISTINCT FROM 'subscription')
EXECUTE FUNCTION clear_group_organization_scopes_on_type_change();
