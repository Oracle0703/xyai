//go:build integration

package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// Locked acceptance tests for docs/features/department-member-management-design-cn.md §3 and §3.1.
// Implementations must make these pass without weakening or deleting assertions.

type departmentMemberFixture struct {
	t             *testing.T
	ctx           context.Context
	prefix        string
	userIDs       []int64
	departmentIDs []int64
}

func newDepartmentMemberFixture(t *testing.T) *departmentMemberFixture {
	t.Helper()
	f := &departmentMemberFixture{t: t, ctx: context.Background(), prefix: "dept_member_" + uuid.NewString()[:8] + "_"}
	t.Cleanup(f.cleanup)
	return f
}

func (f *departmentMemberFixture) cleanup() {
	ids := pq.Array(f.userIDs)
	for _, statement := range []string{
		`DELETE FROM department_access_grants WHERE user_id=ANY($1) OR created_by=ANY($1)`,
		`UPDATE users SET department_id=NULL WHERE id=ANY($1)`,
		`DELETE FROM audit_logs WHERE actor_user_id=ANY($1) OR extra->>'user_id' IN (SELECT unnest($1::bigint[])::text)`,
		`DELETE FROM users WHERE id=ANY($1)`,
	} {
		_, err := integrationDB.ExecContext(f.ctx, statement, ids)
		require.NoError(f.t, err)
	}
	_, err := integrationDB.ExecContext(f.ctx, `DELETE FROM departments WHERE id=ANY($1)`, pq.Array(f.departmentIDs))
	require.NoError(f.t, err)
}

func (f *departmentMemberFixture) user(name, domain, role string, permissions []string) int64 {
	f.t.Helper()
	if permissions == nil {
		permissions = []string{}
	}
	raw, err := json.Marshal(permissions)
	require.NoError(f.t, err)
	var id int64
	require.NoError(f.t, integrationDB.QueryRowContext(f.ctx, `INSERT INTO users(email,password_hash,role,status,admin_permissions) VALUES($1,'test',$2,'active',$3) RETURNING id`,
		f.prefix+name+"@"+domain, role, string(raw)).Scan(&id))
	f.userIDs = append(f.userIDs, id)
	return id
}

func (f *departmentMemberFixture) department(org string) int64 {
	f.t.Helper()
	var id int64
	require.NoError(f.t, integrationDB.QueryRowContext(f.ctx, `INSERT INTO departments(organization_key,name) VALUES($1,$2) RETURNING id`,
		org, f.prefix+uuid.NewString()[:8]).Scan(&id))
	f.departmentIDs = append(f.departmentIDs, id)
	return id
}

func (f *departmentMemberFixture) exec(query string, args ...any) error {
	_, err := integrationDB.ExecContext(f.ctx, query, args...)
	return err
}

func (f *departmentMemberFixture) mustExec(query string, args ...any) {
	f.t.Helper()
	require.NoError(f.t, f.exec(query, args...))
}

// requireStatus checks the persisted status and disabled_source; an empty source means NULL.
func (f *departmentMemberFixture) requireStatus(id int64, status, source string, msgAndArgs ...any) {
	f.t.Helper()
	var gotStatus string
	var gotSource sql.NullString
	require.NoError(f.t, integrationDB.QueryRowContext(f.ctx, `SELECT status,disabled_source FROM users WHERE id=$1`, id).Scan(&gotStatus, &gotSource))
	require.Equal(f.t, status, gotStatus, msgAndArgs...)
	require.Equal(f.t, source, gotSource.String, msgAndArgs...)
	require.Equal(f.t, source != "", gotSource.Valid, msgAndArgs...)
}

func (f *departmentMemberFixture) note(id int64) string {
	f.t.Helper()
	var note string
	require.NoError(f.t, integrationDB.QueryRowContext(f.ctx, `SELECT department_note FROM users WHERE id=$1`, id).Scan(&note))
	return note
}

type departmentNoteClearedAudit struct {
	before, after sql.NullString
	note          string
}

func (f *departmentMemberFixture) noteClearedAudits(id int64) []departmentNoteClearedAudit {
	f.t.Helper()
	rows, err := integrationDB.QueryContext(f.ctx, `SELECT extra->>'before_department_id',extra->>'after_department_id',extra->>'department_note'
FROM audit_logs WHERE action='department.note_cleared' AND extra->>'user_id'=$1 ORDER BY id`, strconv.FormatInt(id, 10))
	require.NoError(f.t, err)
	defer func() { _ = rows.Close() }()
	var out []departmentNoteClearedAudit
	for rows.Next() {
		var item departmentNoteClearedAudit
		require.NoError(f.t, rows.Scan(&item.before, &item.after, &item.note))
		out = append(out, item)
	}
	require.NoError(f.t, rows.Err())
	return out
}

func TestDepartmentMemberSchemaIntegration_DisabledSourceTrigger(t *testing.T) {
	f := newDepartmentMemberFixture(t)
	id := f.user("source", "xunyou.com", service.RoleUser, nil)

	f.mustExec(`UPDATE users SET status='disabled' WHERE id=$1`, id)
	f.requireStatus(id, "disabled", "other", "a disable that does not name its source must not be enable-able by managers")
	f.mustExec(`UPDATE users SET status='active' WHERE id=$1`, id)
	f.requireStatus(id, "active", "", "enabling clears the source")

	f.mustExec(`UPDATE users SET status='disabled', disabled_source='department_manager' WHERE id=$1`, id)
	f.requireStatus(id, "disabled", "department_manager")
	f.mustExec(`UPDATE users SET username='renamed', notes='admin note', balance=balance+1 WHERE id=$1`, id)
	f.requireStatus(id, "disabled", "department_manager", "profile and billing writes must not change the source")

	f.mustExec(`UPDATE users SET disabled_source='other' WHERE id=$1`, id)
	f.requireStatus(id, "disabled", "other", "department_manager may be upgraded to other")
	require.Error(t, f.exec(`UPDATE users SET disabled_source='department_manager' WHERE id=$1`, id), "other must never be downgraded")
	f.requireStatus(id, "disabled", "other")

	f.mustExec(`UPDATE users SET status='active' WHERE id=$1`, id)
	f.mustExec(`UPDATE users SET disabled_source='other' WHERE id=$1`, id)
	f.requireStatus(id, "active", "", "a non-disabled user never carries a source")
	require.Error(t, f.exec(`UPDATE users SET status='disabled', disabled_source='bogus' WHERE id=$1`, id))
	f.requireStatus(id, "active", "")

	var inserted int64
	require.NoError(t, integrationDB.QueryRowContext(f.ctx, `INSERT INTO users(email,password_hash,role,status) VALUES($1,'test','user','disabled') RETURNING id`,
		f.prefix+"inserted@xunyou.com").Scan(&inserted))
	f.userIDs = append(f.userIDs, inserted)
	f.requireStatus(inserted, "disabled", "other", "users inserted as disabled are not manager-disabled")

	var inconsistent int
	require.NoError(t, integrationDB.QueryRowContext(f.ctx, `SELECT COUNT(*) FROM users WHERE (status='disabled') <> (disabled_source IS NOT NULL)`).Scan(&inconsistent))
	require.Zero(t, inconsistent, "migration backfill and constraints keep status and source consistent for every row")
}

// R1: an explicit admin/system disable through the generic repository always records other,
// even when the user is already disabled by a manager; profile writes from stale snapshots keep the source.
func TestDepartmentMemberSchemaIntegration_UserRepositoryExplicitDisableWritesOther(t *testing.T) {
	f := newDepartmentMemberFixture(t)
	id := f.user("r1", "xunyou.com", service.RoleUser, nil)
	repo := NewUserRepository(integrationEntClient, integrationDB)

	stale, err := repo.GetByID(f.ctx, id)
	require.NoError(t, err)
	f.mustExec(`UPDATE users SET status='disabled', disabled_source='department_manager' WHERE id=$1`, id)

	stale.Username = "stale-rename"
	require.NoError(t, repo.Update(f.ctx, stale, service.UserUpdateFields{Username: true}))
	f.requireStatus(id, "disabled", "department_manager", "a profile-only update must not touch status or source")

	current, err := repo.GetByID(f.ctx, id)
	require.NoError(t, err)
	current.Status = service.StatusDisabled
	require.NoError(t, repo.Update(f.ctx, current, service.UserUpdateFields{Status: true}))
	f.requireStatus(id, "disabled", "other", "re-disabling an already manager-disabled user must take it out of managers' hands")

	current.Status = service.StatusActive
	require.NoError(t, repo.Update(f.ctx, current, service.UserUpdateFields{Status: true}))
	f.requireStatus(id, "active", "")
	current.Status = service.StatusDisabled
	require.NoError(t, repo.Update(f.ctx, current, service.UserUpdateFields{Status: true}))
	f.requireStatus(id, "disabled", "other")
}

func TestDepartmentMemberSchemaIntegration_DepartmentNote(t *testing.T) {
	f := newDepartmentMemberFixture(t)
	first := f.department(service.OrganizationXunyou)
	second := f.department(service.OrganizationXunyou)
	id := f.user("note", "xunyou.com", service.RoleUser, nil)
	f.mustExec(`UPDATE users SET department_id=$1 WHERE id=$2`, first, id)
	f.mustExec(`UPDATE users SET department_note='first note' WHERE id=$1`, id)

	f.mustExec(`UPDATE users SET username='renamed', status='disabled' WHERE id=$1`, id)
	f.mustExec(`UPDATE users SET status='active', role='sub_admin' WHERE id=$1`, id)
	f.mustExec(`UPDATE users SET role='user' WHERE id=$1`, id)
	require.Equal(t, "first note", f.note(id), "only membership changes clear the note")
	require.Empty(t, f.noteClearedAudits(id))

	f.mustExec(`UPDATE users SET department_id=$1 WHERE id=$2`, second, id)
	require.Empty(t, f.note(id), "transfers clear the note")
	audits := f.noteClearedAudits(id)
	require.Len(t, audits, 1)
	require.Equal(t, strconv.FormatInt(first, 10), audits[0].before.String)
	require.Equal(t, strconv.FormatInt(second, 10), audits[0].after.String)
	require.Equal(t, "first note", audits[0].note, "the audit keeps the cleared note")

	f.mustExec(`UPDATE users SET department_id=NULL WHERE id=$1`, id)
	require.Len(t, f.noteClearedAudits(id), 1, "clearing an empty note is not audited")

	f.mustExec(`UPDATE users SET department_id=$1 WHERE id=$2`, first, id)
	f.mustExec(`UPDATE users SET department_note='second note' WHERE id=$1`, id)
	f.mustExec(`UPDATE users SET email=$1 WHERE id=$2`, f.prefix+"note@wsdashi.com", id)
	var department sql.NullInt64
	require.NoError(t, integrationDB.QueryRowContext(f.ctx, `SELECT department_id FROM users WHERE id=$1`, id).Scan(&department))
	require.False(t, department.Valid, "cross-organization email change clears membership")
	require.Empty(t, f.note(id), "a membership cleared by the email guard also clears the note")
	audits = f.noteClearedAudits(id)
	require.Len(t, audits, 2)
	require.Equal(t, strconv.FormatInt(first, 10), audits[1].before.String)
	require.False(t, audits[1].after.Valid)
	require.Equal(t, "second note", audits[1].note)

	f.mustExec(`UPDATE users SET department_note=$1 WHERE id=$2`, strings.Repeat("备", 1000), id)
	require.Error(t, f.exec(`UPDATE users SET department_note=$1 WHERE id=$2`, strings.Repeat("备", 1001), id), "the database enforces 1000 characters")
}
