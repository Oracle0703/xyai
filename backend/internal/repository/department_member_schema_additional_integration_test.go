//go:build integration

package repository

import (
	"database/sql"
	"strconv"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestDepartmentMemberSchemaIntegration_MigrationBackfillsExistingDisabledUsers(t *testing.T) {
	f := newDepartmentMemberFixture(t)
	migrationSQL, err := migrations.FS.ReadFile("261_department_member_management.sql")
	require.NoError(t, err)
	tx, err := integrationDB.BeginTx(f.ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	schemaName := f.prefix + "migration"
	_, err = tx.ExecContext(f.ctx, `CREATE SCHEMA `+pq.QuoteIdentifier(schemaName))
	require.NoError(t, err)
	_, err = tx.ExecContext(f.ctx, `SET LOCAL search_path TO `+pq.QuoteIdentifier(schemaName))
	require.NoError(t, err)
	_, err = tx.ExecContext(f.ctx, `
CREATE TABLE users (
    id BIGINT PRIMARY KEY,
    email VARCHAR(255) NOT NULL,
    status VARCHAR(20) NOT NULL,
    department_id BIGINT
);
CREATE TABLE audit_logs (
    action TEXT,
    auth_method TEXT,
    method TEXT,
    status_code INT,
    extra JSONB
);
INSERT INTO users(id,email,status,department_id) VALUES
    (1,'legacy-disabled@xunyou.com','disabled',NULL),
    (2,'legacy-member@xunyou.com','disabled',42),
    (3,'legacy-active@xunyou.com','active',42);
`)
	require.NoError(t, err)
	_, err = tx.ExecContext(f.ctx, string(migrationSQL))
	require.NoError(t, err)

	for _, expected := range []struct {
		id     int64
		status string
		source string
	}{
		{1, "disabled", "other"},
		{2, "disabled", "other"},
		{3, "active", ""},
	} {
		var status, note string
		var source sql.NullString
		require.NoError(t, tx.QueryRowContext(f.ctx, `SELECT status,disabled_source,department_note FROM users WHERE id=$1`, expected.id).
			Scan(&status, &source, &note))
		require.Equal(t, expected.status, status)
		require.Equal(t, expected.source, source.String)
		require.Equal(t, expected.source != "", source.Valid)
		require.Empty(t, note)
	}
	var departmentID int64
	require.NoError(t, tx.QueryRowContext(f.ctx, `SELECT department_id FROM users WHERE id=2`).Scan(&departmentID))
	require.EqualValues(t, 42, departmentID, "the migration does not reassign existing members")
	require.NoError(t, tx.Rollback())
	var schemaExists bool
	require.NoError(t, integrationDB.QueryRowContext(f.ctx, `SELECT EXISTS(SELECT 1 FROM pg_namespace WHERE nspname=$1)`, schemaName).Scan(&schemaExists))
	require.False(t, schemaExists, "the isolated migration fixture leaves no schema behind")
}

func TestDepartmentMemberSchemaIntegration_SourceConstraintSQLStates(t *testing.T) {
	f := newDepartmentMemberFixture(t)
	id := f.user("source_constraints", "xunyou.com", service.RoleUser, nil)

	err := f.exec(`UPDATE users SET status='disabled',disabled_source='invalid' WHERE id=$1`, id)
	var pgErr *pq.Error
	require.ErrorAs(t, err, &pgErr)
	require.Equal(t, pq.ErrorCode("23514"), pgErr.Code)
	f.requireStatus(id, "active", "", "an invalid source must not partially disable the user")

	f.mustExec(`UPDATE users SET status='disabled',disabled_source=NULL WHERE id=$1`, id)
	f.requireStatus(id, "disabled", "other", "an omitted source is never manager-enable-able")
	err = f.exec(`UPDATE users SET disabled_source='department_manager' WHERE id=$1`, id)
	pgErr = nil
	require.ErrorAs(t, err, &pgErr)
	require.Equal(t, pq.ErrorCode("23514"), pgErr.Code)
	f.requireStatus(id, "disabled", "other")

	f.mustExec(`UPDATE users SET disabled_source=NULL WHERE id=$1`, id)
	f.requireStatus(id, "disabled", "other", "setting NULL must not bypass the protected source")
}

func TestDepartmentMemberSchemaIntegration_CheckRejectsDisabledNullWithoutTrigger(t *testing.T) {
	f := newDepartmentMemberFixture(t)
	id := f.user("check_null", "xunyou.com", service.RoleUser, nil)

	// Rollback restores the trigger even if a require assertion aborts the test.
	// Nothing is committed and other sessions cannot observe the disabled trigger.
	tx, err := integrationDB.BeginTx(f.ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(f.ctx, `SET LOCAL lock_timeout='5s'`)
	require.NoError(t, err)
	_, err = tx.ExecContext(f.ctx, `ALTER TABLE users DISABLE TRIGGER users_disabled_source_guard`)
	require.NoError(t, err)
	_, err = tx.ExecContext(f.ctx, `UPDATE users SET status='disabled',disabled_source=NULL WHERE id=$1`, id)
	var pgErr *pq.Error
	require.ErrorAs(t, err, &pgErr)
	require.Equal(t, pq.ErrorCode("23514"), pgErr.Code)
	require.Equal(t, "users_disabled_source_check", pgErr.Constraint)
	require.NoError(t, tx.Rollback())
	f.requireStatus(id, "active", "")

	// This fresh statement also verifies the trigger was restored by rollback.
	f.mustExec(`UPDATE users SET status='disabled' WHERE id=$1`, id)
	f.requireStatus(id, "disabled", "other")
}

func TestDepartmentMemberSchemaIntegration_NoteClearAuditMetadata(t *testing.T) {
	f := newDepartmentMemberFixture(t)
	first := f.department(service.OrganizationXunyou)
	second := f.department(service.OrganizationXunyou)
	id := f.user("audit_metadata", "xunyou.com", service.RoleUser, nil)
	f.mustExec(`UPDATE users SET department_id=$1 WHERE id=$2`, first, id)
	f.mustExec(`UPDATE users SET department_note=$1 WHERE id=$2`, "旧部门备注", id)

	f.mustExec(`UPDATE users SET department_id=department_id WHERE id=$1`, id)
	require.Equal(t, "旧部门备注", f.note(id), "a same-department assignment preserves the note")
	require.Empty(t, f.noteClearedAudits(id))

	// Even a writer that supplies a replacement note cannot carry it across a transfer.
	f.mustExec(`UPDATE users SET department_id=$1,department_note='carry across' WHERE id=$2`, second, id)
	require.Empty(t, f.note(id))
	var authMethod, method, userID, before, after, oldNote string
	var statusCode int
	var actorID sql.NullInt64
	require.NoError(t, integrationDB.QueryRowContext(f.ctx, `
SELECT auth_method,method,status_code,actor_user_id,
       extra->>'user_id',extra->>'before_department_id',extra->>'after_department_id',extra->>'department_note'
FROM audit_logs WHERE action='department.note_cleared' AND extra->>'user_id'=$1`,
		strconv.FormatInt(id, 10)).Scan(&authMethod, &method, &statusCode, &actorID, &userID, &before, &after, &oldNote))
	require.Equal(t, "database_guard", authMethod)
	require.Equal(t, "INTERNAL", method)
	require.Equal(t, 200, statusCode)
	require.False(t, actorID.Valid, "the database guard must not invent an authenticated actor")
	require.Equal(t, strconv.FormatInt(id, 10), userID)
	require.Equal(t, strconv.FormatInt(first, 10), before)
	require.Equal(t, strconv.FormatInt(second, 10), after)
	require.Equal(t, "旧部门备注", oldNote)
	require.Len(t, f.noteClearedAudits(id), 1)
}

func TestDepartmentMemberSchemaIntegration_NoteAuditFailureRollsBackMembership(t *testing.T) {
	f := newDepartmentMemberFixture(t)
	first := f.department(service.OrganizationXunyou)
	second := f.department(service.OrganizationXunyou)
	id := f.user("audit_rollback", "xunyou.com", service.RoleUser, nil)
	f.mustExec(`UPDATE users SET department_id=$1 WHERE id=$2`, first, id)
	f.mustExec(`UPDATE users SET department_note='must survive' WHERE id=$1`, id)
	var version int64
	require.NoError(t, integrationDB.QueryRowContext(f.ctx, `SELECT department_version FROM users WHERE id=$1`, id).Scan(&version))

	// The uncommitted constraint is local to this transaction and is rolled back
	// together with the failed transfer; it never becomes a shared schema change.
	tx, err := integrationDB.BeginTx(f.ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(f.ctx, `SET LOCAL lock_timeout='5s'`)
	require.NoError(t, err)
	constraintName := f.prefix + "reject_note_audit"
	_, err = tx.ExecContext(f.ctx, `ALTER TABLE audit_logs ADD CONSTRAINT `+pq.QuoteIdentifier(constraintName)+
		` CHECK (action <> 'department.note_cleared') NOT VALID`)
	require.NoError(t, err)
	_, err = tx.ExecContext(f.ctx, `UPDATE users SET department_id=$1 WHERE id=$2`, second, id)
	var pgErr *pq.Error
	require.ErrorAs(t, err, &pgErr)
	require.Equal(t, pq.ErrorCode("23514"), pgErr.Code)
	require.Equal(t, constraintName, pgErr.Constraint)
	require.NoError(t, tx.Rollback())

	var currentDepartment, currentVersion int64
	require.NoError(t, integrationDB.QueryRowContext(f.ctx, `SELECT department_id,department_version FROM users WHERE id=$1`, id).
		Scan(&currentDepartment, &currentVersion))
	require.Equal(t, first, currentDepartment)
	require.Equal(t, version, currentVersion, "the failed audit also rolls back the membership version")
	require.Equal(t, "must survive", f.note(id))
	require.Empty(t, f.noteClearedAudits(id))
	var constraintExists bool
	require.NoError(t, integrationDB.QueryRowContext(f.ctx, `SELECT EXISTS(SELECT 1 FROM pg_constraint WHERE conname=$1)`, constraintName).Scan(&constraintExists))
	require.False(t, constraintExists)

	f.mustExec(`UPDATE users SET department_id=$1 WHERE id=$2`, second, id)
	require.Empty(t, f.note(id))
	require.Len(t, f.noteClearedAudits(id), 1, "normal auditing works after the temporary failure is removed")
}

func TestDepartmentMemberSchemaIntegration_UserRepositoryReturnsPersistedGuardFields(t *testing.T) {
	f := newDepartmentMemberFixture(t)
	departmentID := f.department(service.OrganizationXunyou)
	id := f.user("returned_fields", "xunyou.com", service.RoleUser, nil)
	f.mustExec(`UPDATE users SET department_id=$1 WHERE id=$2`, departmentID, id)
	f.mustExec(`UPDATE users SET department_note='before email change' WHERE id=$1`, id)
	repo := NewUserRepository(integrationEntClient, integrationDB)
	current, err := repo.GetByID(f.ctx, id)
	require.NoError(t, err)
	require.Equal(t, "before email change", current.DepartmentNote)
	require.Nil(t, current.DisabledSource)

	f.mustExec(`UPDATE users SET status='disabled',disabled_source='department_manager' WHERE id=$1`, id)
	current.Username = "profile-only"
	require.NoError(t, repo.Update(f.ctx, current, service.UserUpdateFields{Username: true}))
	require.NotNil(t, current.DisabledSource)
	require.Equal(t, "department_manager", *current.DisabledSource, "the returned source reflects the database, not the stale input")
	current.Status = service.StatusDisabled
	require.NoError(t, repo.Update(f.ctx, current, service.UserUpdateFields{Status: true}))
	require.NotNil(t, current.DisabledSource)
	require.Equal(t, "other", *current.DisabledSource, "the response reflects the explicit admin/system source")
	current.Status = service.StatusActive
	require.NoError(t, repo.Update(f.ctx, current, service.UserUpdateFields{Status: true}))
	require.Nil(t, current.DisabledSource, "the response reflects the trigger clearing the source")

	previousVersion := current.DepartmentVersion
	current.Email = f.prefix + "returned_fields@wsdashi.com"
	require.NoError(t, repo.Update(f.ctx, current, service.UserUpdateFields{Email: true}))
	require.Nil(t, current.DepartmentID)
	require.Equal(t, previousVersion+1, current.DepartmentVersion)
	require.Empty(t, current.DepartmentNote, "the update response must not disclose the old department's note")
	stored, err := repo.GetByID(f.ctx, id)
	require.NoError(t, err)
	require.Equal(t, current.DepartmentID, stored.DepartmentID)
	require.Equal(t, current.DepartmentVersion, stored.DepartmentVersion)
	require.Equal(t, current.DepartmentNote, stored.DepartmentNote)
	require.Len(t, f.noteClearedAudits(id), 1)
}
