-- Personnel departments are independent from API routing and subscription groups.
CREATE TABLE IF NOT EXISTS departments (
    id BIGSERIAL PRIMARY KEY,
    organization_key VARCHAR(20) NOT NULL CHECK (organization_key IN ('xunyou', 'wsdashi', 'other')),
    name VARCHAR(100) NOT NULL CHECK (length(btrim(name)) > 0),
    status VARCHAR(20) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive')),
    sort_order INTEGER NOT NULL DEFAULT 0,
    version BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS departments_organization_name_key ON departments (organization_key, lower(btrim(name)));
CREATE INDEX IF NOT EXISTS departments_list_idx ON departments (organization_key, status, sort_order, id);

ALTER TABLE users ADD COLUMN IF NOT EXISTS department_id BIGINT REFERENCES departments(id) ON DELETE RESTRICT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS department_version BIGINT NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS users_department_idx ON users (department_id, id);

CREATE TABLE IF NOT EXISTS department_access_grants (
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    department_id BIGINT NOT NULL REFERENCES departments(id) ON DELETE RESTRICT,
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, department_id)
);
CREATE INDEX IF NOT EXISTS department_grants_department_idx ON department_access_grants (department_id, user_id);

-- Cover every email update path, including self-service and future imports.
-- This trigger never alters subscriptions or usage logs.
CREATE OR REPLACE FUNCTION maintain_user_department() RETURNS trigger AS $$
DECLARE
    org_key TEXT;
    target_org TEXT;
    target_status TEXT;
BEGIN
    org_key := CASE lower(split_part(NEW.email, '@', 2))
        WHEN 'xunyou.com' THEN 'xunyou'
        WHEN 'wsdashi.com' THEN 'wsdashi'
        ELSE 'other' END;
    IF NEW.department_id IS NOT NULL THEN
        SELECT organization_key, status INTO target_org, target_status
        FROM departments WHERE id = NEW.department_id FOR SHARE;
        IF NOT FOUND THEN
            RAISE EXCEPTION 'department does not exist' USING ERRCODE = '23503';
        END IF;
        IF target_org <> org_key THEN
            IF TG_OP = 'UPDATE' AND NEW.department_id IS NOT DISTINCT FROM OLD.department_id
                AND NEW.email IS DISTINCT FROM OLD.email THEN
                NEW.department_id := NULL;
            ELSE
                RAISE EXCEPTION 'department organization mismatch' USING ERRCODE = '23514';
            END IF;
        ELSIF target_status <> 'active' AND (TG_OP = 'INSERT' OR NEW.department_id IS DISTINCT FROM OLD.department_id) THEN
            RAISE EXCEPTION 'department is inactive' USING ERRCODE = '23514';
        END IF;
    END IF;
    IF TG_OP = 'UPDATE' THEN
        IF NEW.department_id IS DISTINCT FROM OLD.department_id THEN
            NEW.department_version := OLD.department_version + 1;
        ELSE
            NEW.department_version := OLD.department_version;
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS users_department_guard ON users;
CREATE TRIGGER users_department_guard BEFORE INSERT OR UPDATE OF email, department_id, department_version
ON users FOR EACH ROW EXECUTE FUNCTION maintain_user_department();

CREATE OR REPLACE FUNCTION protect_department_organization() RETURNS trigger AS $$
BEGIN
    IF NEW.organization_key IS DISTINCT FROM OLD.organization_key THEN
        RAISE EXCEPTION 'department organization is immutable' USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS departments_organization_guard ON departments;
CREATE TRIGGER departments_organization_guard BEFORE UPDATE OF organization_key
ON departments FOR EACH ROW EXECUTE FUNCTION protect_department_organization();
