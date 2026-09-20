-- The identity of an arbitrary database email writer is not necessarily known.
-- Record the subject and transition without inventing an authenticated actor.
CREATE OR REPLACE FUNCTION audit_department_email_change() RETURNS trigger AS $$
BEGIN
    IF OLD.department_id IS NOT NULL AND NEW.department_id IS NULL
        AND OLD.email IS DISTINCT FROM NEW.email THEN
        INSERT INTO audit_logs (action, auth_method, method, status_code, extra)
        VALUES ('department.email_organization_changed', 'database_guard', 'INTERNAL', 200,
            jsonb_build_object('user_id', NEW.id, 'before_department_id', OLD.department_id,
                'after_department_id', NULL, 'department_version', NEW.department_version));
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS users_department_email_audit ON users;
CREATE TRIGGER users_department_email_audit AFTER UPDATE OF email
ON users FOR EACH ROW EXECUTE FUNCTION audit_department_email_change();
