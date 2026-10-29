-- Department managers can only re-enable users disabled by a department manager.
ALTER TABLE users ADD COLUMN disabled_source VARCHAR(32);
ALTER TABLE users ADD COLUMN department_note TEXT NOT NULL DEFAULT '';

UPDATE users SET disabled_source = 'other' WHERE status = 'disabled';

ALTER TABLE users ADD CONSTRAINT users_disabled_source_check CHECK (
    (status = 'disabled' AND disabled_source IS NOT NULL
        AND disabled_source IN ('department_manager', 'other'))
    OR (status <> 'disabled' AND disabled_source IS NULL)
);
ALTER TABLE users ADD CONSTRAINT users_department_note_length_check
    CHECK (char_length(department_note) <= 1000);

CREATE OR REPLACE FUNCTION maintain_user_disabled_source() RETURNS trigger AS $$
BEGIN
    IF NEW.status <> 'disabled' THEN
        NEW.disabled_source := NULL;
        RETURN NEW;
    END IF;
    IF NEW.disabled_source IS NOT NULL
        AND NEW.disabled_source NOT IN ('department_manager', 'other') THEN
        RAISE EXCEPTION 'invalid disabled source' USING ERRCODE = '23514';
    END IF;
    NEW.disabled_source := COALESCE(NEW.disabled_source, 'other');
    IF TG_OP = 'UPDATE' THEN
        IF OLD.status = 'disabled' AND OLD.disabled_source = 'other'
            AND NEW.disabled_source = 'department_manager' THEN
            RAISE EXCEPTION 'system or administrator disable cannot become manager disable'
                USING ERRCODE = '23514';
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Explicit admin/system re-disables must also set disabled_source='other'.
-- A trigger cannot distinguish an omitted column from its inherited old value.
CREATE TRIGGER users_disabled_source_guard
BEFORE INSERT OR UPDATE OF status, disabled_source ON users
FOR EACH ROW EXECUTE FUNCTION maintain_user_disabled_source();

CREATE OR REPLACE FUNCTION clear_user_department_note() RETURNS trigger AS $$
BEGIN
    IF NEW.department_id IS DISTINCT FROM OLD.department_id THEN
        NEW.department_note := '';
        IF OLD.department_note <> '' THEN
            INSERT INTO audit_logs (action, auth_method, method, status_code, extra)
            VALUES ('department.note_cleared', 'database_guard', 'INTERNAL', 200,
                jsonb_build_object('user_id', NEW.id,
                    'before_department_id', OLD.department_id,
                    'after_department_id', NEW.department_id,
                    'department_note', OLD.department_note));
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- PostgreSQL runs same-kind triggers in name order: this follows
-- users_department_guard and observes its cross-organization email cleanup.
CREATE TRIGGER users_department_note_guard
BEFORE UPDATE OF email, department_id ON users
FOR EACH ROW EXECUTE FUNCTION clear_user_department_note();
