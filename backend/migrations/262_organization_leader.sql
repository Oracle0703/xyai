-- Organization leaders manage every department and unassigned member of an organization.
-- An organization grant and department grants never coexist for one user; the
-- application enforces that cross-table rule in every writer.
CREATE TABLE IF NOT EXISTS organization_access_grants (
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    organization_key VARCHAR(20) NOT NULL CHECK (organization_key IN ('xunyou', 'wsdashi')),
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, organization_key)
);
CREATE INDEX IF NOT EXISTS organization_grants_organization_idx ON organization_access_grants (organization_key, user_id);

-- Disable sources are ranked department_manager < organization_manager < other.
-- An explicit re-disable may escalate the source but never downgrade it.
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_disabled_source_check;
ALTER TABLE users ADD CONSTRAINT users_disabled_source_check CHECK (
    (status = 'disabled' AND disabled_source IS NOT NULL
        AND disabled_source IN ('department_manager', 'organization_manager', 'other'))
    OR (status <> 'disabled' AND disabled_source IS NULL)
);

CREATE OR REPLACE FUNCTION maintain_user_disabled_source() RETURNS trigger AS $$
DECLARE
    old_rank INT;
    new_rank INT;
BEGIN
    IF NEW.status <> 'disabled' THEN
        NEW.disabled_source := NULL;
        RETURN NEW;
    END IF;
    IF NEW.disabled_source IS NOT NULL
        AND NEW.disabled_source NOT IN ('department_manager', 'organization_manager', 'other') THEN
        RAISE EXCEPTION 'invalid disabled source' USING ERRCODE = '23514';
    END IF;
    NEW.disabled_source := COALESCE(NEW.disabled_source, 'other');
    IF TG_OP = 'UPDATE' AND OLD.status = 'disabled' THEN
        old_rank := CASE OLD.disabled_source WHEN 'department_manager' THEN 1 WHEN 'organization_manager' THEN 2 ELSE 3 END;
        new_rank := CASE NEW.disabled_source WHEN 'department_manager' THEN 1 WHEN 'organization_manager' THEN 2 ELSE 3 END;
        IF new_rank < old_rank THEN
            RAISE EXCEPTION 'a disable source cannot be downgraded' USING ERRCODE = '23514';
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
