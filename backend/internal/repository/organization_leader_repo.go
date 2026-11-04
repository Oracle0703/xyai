package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

// SetOrganizationAccess is the only writer of organization grants and
// admin.organization_manage (design §5.1). Only full administrators call it.
func (r *departmentRepository) SetOrganizationAccess(ctx context.Context, userID int64, in service.OrganizationAccessInput) (*service.DepartmentAccess, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if err = lockDepartmentUsers(ctx, tx, []int64{service.DepartmentActorID(ctx), userID}); err != nil {
		return nil, err
	}
	actor, err := loadDepartmentActor(ctx, tx, "")
	if err != nil {
		return nil, err
	}
	previous, err := loadAdminAccess(ctx, tx, userID)
	if err != nil {
		return nil, err
	}
	if previous.Version != in.ExpectedVersion {
		return nil, service.ErrAdminAccessChanged
	}
	if previous.Role != service.RoleSubAdmin {
		return nil, service.ErrDepartmentInvalid
	}
	organizations := *in.Organizations
	permissions := []string{}
	if len(organizations) > 0 {
		// D8: an organization leader holds no department grants; replacing them
		// must be confirmed explicitly, never done silently.
		if len(previous.DepartmentIDs) > 0 && !in.ReplaceDepartmentGrants {
			return nil, service.ErrOrganizationDepartmentGrantsConfirmation
		}
		for _, p := range previous.Permissions {
			if p == service.AdminPermissionSubscriptions {
				if !in.ReplaceGlobalSubscriptions {
					return nil, service.ErrDepartmentGlobalConfirmation
				}
				continue
			}
			permissions = append(permissions, p)
		}
		permissions = append(permissions, service.AdminPermissionOrganizationManage)
		permissions = append(permissions, service.OrganizationLeaderDepartmentPermissions...)
		if _, err = tx.ExecContext(ctx, `DELETE FROM department_access_grants WHERE user_id=$1`, userID); err != nil {
			return nil, err
		}
	} else {
		// Revoking keeps the department capabilities without any scope, the
		// existing legal state from which an administrator can grant departments.
		for _, p := range previous.Permissions {
			if p != service.AdminPermissionOrganizationManage {
				permissions = append(permissions, p)
			}
		}
	}
	permissions, err = service.NormalizeAdminPermissions(service.RoleSubAdmin, permissions)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(permissions)
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE users SET admin_permissions=$1,updated_at=NOW() WHERE id=$2`, string(encoded), userID); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM organization_access_grants WHERE user_id=$1 AND NOT (organization_key=ANY($2::text[]))`, userID, pq.Array(organizations)); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO organization_access_grants(user_id,organization_key,created_by)
SELECT $1,key,$3 FROM unnest($2::text[]) AS key ON CONFLICT DO NOTHING`, userID, pq.Array(organizations), actor.ID); err != nil {
		return nil, departmentPersistenceError(err)
	}
	result, err := loadDepartmentAccess(ctx, tx, userID)
	if err != nil {
		return nil, err
	}
	if err = departmentAudit(ctx, tx, actor, "organization.grant_scope", map[string]any{"before": previous, "after": result}); err != nil {
		return nil, err
	}
	return result, tx.Commit()
}

// organizationLeaderActor loads an organization leader and narrows an optional
// organization filter to the leader's grants. Full administrators use the
// existing department-scope endpoints instead.
func organizationLeaderActor(ctx context.Context, q sqlExecutor, organization string) (*service.User, []string, error) {
	actor, err := loadDepartmentActor(ctx, q, service.AdminPermissionOrganizationManage)
	if err != nil {
		return nil, nil, err
	}
	if !departmentActorIsOrganizationLeader(actor) {
		return nil, nil, service.ErrDepartmentScopeDenied
	}
	if organization == "" || organization == service.OrganizationAll {
		return actor, actor.OrganizationGrants, nil
	}
	if !slices.Contains(actor.OrganizationGrants, organization) {
		return nil, nil, service.ErrDepartmentScopeDenied
	}
	return actor, []string{organization}, nil
}

func (r *departmentRepository) OrganizationLeaders(ctx context.Context, f service.DepartmentListFilter) (*service.OrganizationLeaderList, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	actor, organizations, err := organizationLeaderActor(ctx, tx, f.Organization)
	if err != nil {
		return nil, err
	}
	p := departmentPredicates{clauses: []string{"u.deleted_at IS NULL", "u.role='sub_admin'"}}
	p.add("u.id<>$%d", actor.ID)
	p.add(`(`+organizationUsageOrganizationExpression("u")+`=ANY($%[1]d::text[]) OR EXISTS(SELECT 1 FROM department_access_grants lg JOIN departments ld ON ld.id=lg.department_id
 WHERE lg.user_id=u.id AND ld.organization_key=ANY($%[1]d::text[])))`, pq.Array(organizations))
	if f.Q != "" {
		p.add(`(u.email ILIKE $%[1]d ESCAPE E'\\' OR u.username ILIKE $%[1]d ESCAPE E'\\')`, organizationUsageSearchPattern(f.Q))
	}
	result := &service.OrganizationLeaderList{Items: []service.OrganizationLeader{}, Page: f.Page, PageSize: f.PageSize}
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users u WHERE `+p.where(), p.args...).Scan(&result.Total); err != nil {
		return nil, err
	}
	args := append([]any{}, p.args...)
	args = append(args, pq.Array(actor.OrganizationGrants))
	managed := managedLeaderPredicate("u", len(args))
	args = append(args, f.PageSize, (f.Page-1)*f.PageSize)
	rows, err := tx.QueryContext(ctx, `SELECT u.id,u.email,u.username,u.status,`+organizationUsageOrganizationExpression("u")+`,(`+managed+`),u.admin_permissions
 FROM users u WHERE `+p.where()+fmt.Sprintf(` ORDER BY u.id LIMIT $%d OFFSET $%d`, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, err
	}
	ids := []int64{}
	positions := map[int64]int{}
	for rows.Next() {
		leader := service.OrganizationLeader{Departments: []service.OrganizationLeaderDepartment{}, Permissions: []string{}}
		var permissions []byte
		if err = rows.Scan(&leader.ID, &leader.Email, &leader.Username, &leader.Status, &leader.Organization, &leader.Managed, &permissions); err != nil {
			_ = rows.Close()
			return nil, err
		}
		// Other capabilities of a protected leader are not disclosed.
		if leader.Managed {
			if err = json.Unmarshal(permissions, &leader.Permissions); err != nil {
				_ = rows.Close()
				return nil, err
			}
		}
		positions[leader.ID] = len(result.Items)
		ids = append(ids, leader.ID)
		result.Items = append(result.Items, leader)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return nil, err
	}
	departments, err := organizationLeaderDepartments(ctx, tx, ids, organizations)
	if err != nil {
		return nil, err
	}
	for id, items := range departments {
		result.Items[positions[id]].Departments = items
	}
	return result, tx.Commit()
}

// organizationLeaderDepartments returns grants limited to the given
// organizations, so grants elsewhere are never disclosed.
func organizationLeaderDepartments(ctx context.Context, q sqlExecutor, userIDs []int64, organizations []string) (map[int64][]service.OrganizationLeaderDepartment, error) {
	result := map[int64][]service.OrganizationLeaderDepartment{}
	if len(userIDs) == 0 {
		return result, nil
	}
	rows, err := q.QueryContext(ctx, `SELECT g.user_id,d.id,d.organization_key,d.name,d.status FROM department_access_grants g JOIN departments d ON d.id=g.department_id
 WHERE g.user_id=ANY($1::bigint[]) AND d.organization_key=ANY($2::text[]) ORDER BY g.user_id,d.organization_key,d.sort_order,d.id`, pq.Array(userIDs), pq.Array(organizations))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var userID int64
		var d service.OrganizationLeaderDepartment
		if err = rows.Scan(&userID, &d.ID, &d.Organization, &d.Name, &d.Status); err != nil {
			return nil, err
		}
		result[userID] = append(result[userID], d)
	}
	return result, rows.Err()
}

// OrganizationLeader is the edit snapshot: every grant of a managed leader and
// the version from the same transaction (design §4.1, §14 K3).
func (r *departmentRepository) OrganizationLeader(ctx context.Context, userID int64) (*service.OrganizationLeader, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	actor, _, err := organizationLeaderActor(ctx, tx, "")
	if err != nil {
		return nil, err
	}
	if userID == actor.ID {
		return nil, service.ErrOrganizationLeaderProtected
	}
	leader := service.OrganizationLeader{Managed: true}
	var managed bool
	err = scanSingleRow(ctx, tx, `SELECT u.id,u.email,u.username,u.status,`+organizationUsageOrganizationExpression("u")+`,(u.role='sub_admin' AND `+managedLeaderPredicate("u", 2)+`)
 FROM users u WHERE u.id=$1 AND u.deleted_at IS NULL`, []any{userID, pq.Array(actor.OrganizationGrants)},
		&leader.ID, &leader.Email, &leader.Username, &leader.Status, &leader.Organization, &managed)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !managed) {
		return nil, service.ErrOrganizationLeaderProtected
	}
	if err != nil {
		return nil, err
	}
	access, err := loadAdminAccess(ctx, tx, userID)
	if err != nil {
		return nil, err
	}
	departments, err := organizationLeaderDepartments(ctx, tx, []int64{userID}, actor.OrganizationGrants)
	if err != nil {
		return nil, err
	}
	leader.Departments = departments[userID]
	if leader.Departments == nil {
		leader.Departments = []service.OrganizationLeaderDepartment{}
	}
	leader.Permissions, leader.Version = access.Permissions, access.Version
	return &leader, tx.Commit()
}

func (r *departmentRepository) OrganizationLeaderCandidates(ctx context.Context, organization, search string) ([]service.OrganizationLeaderCandidate, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	_, organizations, err := organizationLeaderActor(ctx, tx, organization)
	if err != nil {
		return nil, err
	}
	p := departmentPredicates{clauses: []string{"u.deleted_at IS NULL", "u.role='user'", "u.status='active'"}}
	p.add(organizationUsageOrganizationExpression("u")+"=ANY($%d::text[])", pq.Array(organizations))
	p.add(`(u.email ILIKE $%[1]d ESCAPE E'\\' OR u.username ILIKE $%[1]d ESCAPE E'\\')`, organizationUsageSearchPattern(search))
	rows, err := tx.QueryContext(ctx, `SELECT u.id,u.email,u.username,`+organizationUsageOrganizationExpression("u")+`,d.id,d.name`+departmentMemberFrom()+
		` WHERE `+p.where()+` ORDER BY u.email,u.id LIMIT 20`, p.args...)
	if err != nil {
		return nil, err
	}
	result := []service.OrganizationLeaderCandidate{}
	for rows.Next() {
		var candidate service.OrganizationLeaderCandidate
		if err = rows.Scan(&candidate.ID, &candidate.Email, &candidate.Username, &candidate.Organization, &candidate.DepartmentID, &candidate.DepartmentName); err != nil {
			_ = rows.Close()
			return nil, err
		}
		result = append(result, candidate)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return nil, err
	}
	for i := range result {
		access, err := loadAdminAccess(ctx, tx, result[i].ID)
		if err != nil {
			return nil, err
		}
		result[i].Version = access.Version
	}
	return result, tx.Commit()
}

// UpdateOrganizationLeader appoints, updates or revokes a department leader
// (design §4.1). Authorization (403) is decided under the row locks before the
// version check (409).
func (r *departmentRepository) UpdateOrganizationLeader(ctx context.Context, userID int64, in service.OrganizationLeaderInput) (*service.DepartmentAccess, bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err = lockDepartmentUsers(ctx, tx, []int64{service.DepartmentActorID(ctx), userID}); err != nil {
		return nil, false, err
	}
	actor, _, err := organizationLeaderActor(ctx, tx, "")
	if err != nil {
		return nil, false, err
	}
	if userID == actor.ID {
		return nil, false, service.ErrOrganizationLeaderProtected
	}
	var role string
	var candidate, managed bool
	err = scanSingleRow(ctx, tx, `SELECT u.role,
 u.role='user' AND u.status='active' AND `+organizationUsageOrganizationExpression("u")+`=ANY($2::text[]),
 u.role='sub_admin' AND `+managedLeaderPredicate("u", 2)+`
 FROM users u WHERE u.id=$1 AND u.deleted_at IS NULL`, []any{userID, pq.Array(actor.OrganizationGrants)}, &role, &candidate, &managed)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !candidate && !managed) {
		return nil, false, service.ErrOrganizationLeaderProtected
	}
	if err != nil {
		return nil, false, err
	}
	ids := *in.DepartmentIDs
	permissions := in.Permissions()
	for _, p := range permissions {
		if !service.HasAdminPermission(actor, p) {
			return nil, false, service.ErrDepartmentScopeDenied
		}
	}
	previous, err := loadAdminAccess(ctx, tx, userID)
	if err != nil {
		return nil, false, err
	}
	old := map[int64]bool{}
	for _, id := range previous.DepartmentIDs {
		old[id] = true
	}
	added := []int64{}
	if len(ids) > 0 {
		// Every requested department must belong to the leader's organizations;
		// unknown and foreign IDs are indistinguishable to the caller.
		rows, err := tx.QueryContext(ctx, `SELECT id,organization_key FROM departments WHERE id=ANY($1::bigint[])`, pq.Array(ids))
		if err != nil {
			return nil, false, err
		}
		found := 0
		for rows.Next() {
			var id int64
			var organization string
			if err = rows.Scan(&id, &organization); err != nil {
				_ = rows.Close()
				return nil, false, err
			}
			if !slices.Contains(actor.OrganizationGrants, organization) {
				_ = rows.Close()
				return nil, false, service.ErrDepartmentScopeDenied
			}
			found++
			if !old[id] {
				added = append(added, id)
			}
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return nil, false, err
		}
		if found != len(ids) {
			return nil, false, service.ErrDepartmentScopeDenied
		}
	} else if role == service.RoleUser {
		return nil, false, service.ErrDepartmentInvalid
	}
	if previous.Version != *in.ExpectedVersion {
		return nil, false, service.ErrAdminAccessChanged
	}
	// Only new grants require an active department; retained grants may stay on
	// an inactive one. FOR SHARE (not FOR UPDATE) keeps department edits, which
	// update the row directly, free of lock upgrades (review R5).
	if len(added) > 0 {
		rows, err := tx.QueryContext(ctx, `SELECT status FROM departments WHERE id=ANY($1::bigint[]) ORDER BY id FOR SHARE`, pq.Array(added))
		if err != nil {
			return nil, false, err
		}
		for rows.Next() {
			var status string
			if err = rows.Scan(&status); err == nil && status != service.StatusActive {
				err = service.ErrDepartmentInactive
			}
			if err != nil {
				_ = rows.Close()
				return nil, false, err
			}
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return nil, false, err
		}
	}
	newRole, action := service.RoleSubAdmin, "organization.leader_update"
	if len(ids) == 0 {
		newRole, action = service.RoleUser, "organization.leader_revoke"
	} else if role == service.RoleUser {
		action = "organization.leader_appoint"
	}
	encoded, err := json.Marshal(permissions)
	if err != nil {
		return nil, false, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE users SET role=$1,admin_permissions=$2,updated_at=NOW() WHERE id=$3`, newRole, string(encoded), userID); err != nil {
		return nil, false, err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM department_access_grants WHERE user_id=$1 AND NOT (department_id=ANY($2::bigint[]))`, userID, pq.Array(ids)); err != nil {
		return nil, false, err
	}
	if len(added) > 0 {
		if _, err = tx.ExecContext(ctx, `INSERT INTO department_access_grants(user_id,department_id,created_by) SELECT $1,id,$3 FROM unnest($2::bigint[]) AS id`, userID, pq.Array(added), actor.ID); err != nil {
			return nil, false, departmentPersistenceError(err)
		}
	}
	after, err := loadAdminAccess(ctx, tx, userID)
	if err != nil {
		return nil, false, err
	}
	changed := after.Version != previous.Version
	if changed {
		if err = departmentAudit(ctx, tx, actor, action, map[string]any{"user_id": userID, "before": previous, "after": after,
			"before_role": previous.Role, "after_role": after.Role, "actor_scope": "organization"}); err != nil {
			return nil, false, err
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, false, departmentPersistenceError(err)
	}
	return after, changed, nil
}

// authorizeOrganizationFilter rejects an organization filter outside an
// organization leader's grants; "all" means all of the leader's organizations.
func authorizeOrganizationFilter(actor *service.User, organization string) error {
	if actor.Role == service.RoleAdmin || organization == "" || organization == service.OrganizationAll {
		return nil
	}
	if !slices.Contains(actor.OrganizationGrants, organization) {
		return service.ErrDepartmentScopeDenied
	}
	return nil
}

// authorizeDepartmentOrganization denies unknown departments and departments of
// other organizations alike, so a leader cannot probe for their existence.
func authorizeDepartmentOrganization(ctx context.Context, q sqlExecutor, actor *service.User, departmentID int64) error {
	var organization string
	err := scanSingleRow(ctx, q, `SELECT organization_key FROM departments WHERE id=$1`, []any{departmentID}, &organization)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !slices.Contains(actor.OrganizationGrants, organization)) {
		return service.ErrDepartmentScopeDenied
	}
	return err
}

// authorizeOrganizationTransfer checks every member before any version check:
// an organization leader moves ordinary users and managed leaders of the own
// organizations, never itself (design §4.3).
// organizationTransferTargets returns which of ids an organization leader may move
// between departments: users of its organizations who are plain users or leaders it
// manages, never itself. Assign enforces it and Members marks rows with it.
func organizationTransferTargets(ctx context.Context, q sqlExecutor, actor *service.User, ids []int64) (map[int64]bool, error) {
	rows, err := q.QueryContext(ctx, `SELECT u.id FROM users u WHERE u.id=ANY($1::bigint[]) AND u.id<>$3 AND u.deleted_at IS NULL
 AND `+organizationUsageOrganizationExpression("u")+`=ANY($2::text[])
 AND (u.role='user' OR (u.role='sub_admin' AND `+managedLeaderPredicate("u", 2)+`))`, pq.Array(ids), pq.Array(actor.OrganizationGrants), actor.ID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	allowed := make(map[int64]bool, len(ids))
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		allowed[id] = true
	}
	return allowed, rows.Err()
}

func authorizeOrganizationTransfer(ctx context.Context, q sqlExecutor, actor *service.User, members []service.DepartmentMemberChange) error {
	ids := make([]int64, 0, len(members))
	for _, m := range members {
		ids = append(ids, m.UserID)
	}
	allowed, err := organizationTransferTargets(ctx, q, actor, ids)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if !allowed[id] {
			return service.ErrDepartmentScopeDenied
		}
	}
	return nil
}

// departmentMemberRows marks the rows the actor may move: admins may move anyone, an
// organization leader exactly the rows authorizeOrganizationTransfer accepts.
func departmentMemberRows(ctx context.Context, q sqlExecutor, actor *service.User, members []service.DepartmentMember) ([]service.DepartmentMemberRow, error) {
	admin := actor.Role == service.RoleAdmin
	allowed := map[int64]bool{}
	if !admin && len(members) > 0 {
		ids := make([]int64, 0, len(members))
		for _, m := range members {
			ids = append(ids, m.ID)
		}
		var err error
		if allowed, err = organizationTransferTargets(ctx, q, actor, ids); err != nil {
			return nil, err
		}
	}
	rows := make([]service.DepartmentMemberRow, 0, len(members))
	for _, m := range members {
		rows = append(rows, service.DepartmentMemberRow{DepartmentMember: m, Manageable: admin || allowed[m.ID]})
	}
	return rows, nil
}

func withActorScope(actor *service.User, details map[string]any) map[string]any {
	if departmentActorIsOrganizationLeader(actor) {
		details["actor_scope"] = "organization"
	}
	return details
}
