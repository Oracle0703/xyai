package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type departmentRepository struct{ db *sql.DB }

func NewDepartmentRepository(db *sql.DB) service.DepartmentRepository {
	return &departmentRepository{db: db}
}

func departmentPersistenceError(err error) error {
	var pe *pq.Error
	if errors.As(err, &pe) {
		switch pe.Code {
		case "23505":
			return service.ErrDepartmentDuplicate
		case "23503", "23514":
			return service.ErrDepartmentInvalid
		case "40001", "40P01":
			return service.ErrDepartmentConflict
		}
	}
	return err
}

func loadDepartmentActor(ctx context.Context, q sqlExecutor, allowedPermissions ...string) (*service.User, error) {
	id := service.DepartmentActorID(ctx)
	if id <= 0 {
		return nil, service.ErrDepartmentScopeDenied
	}
	var actor service.User
	var permissions []byte
	err := scanSingleRow(ctx, q, `SELECT id,email,role,admin_permissions,status FROM users WHERE id=$1 AND deleted_at IS NULL`, []any{id}, &actor.ID, &actor.Email, &actor.Role, &permissions, &actor.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrDepartmentScopeDenied
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(permissions, &actor.AdminPermissions); err != nil {
		return nil, err
	}
	// The validated Admin API Key is an independent full-admin credential.
	// Its audit user is not the source of its authorization.
	if service.DepartmentActorIsAdminAPIKey(ctx) {
		actor.Role = service.RoleAdmin
		actor.Status = service.StatusActive
	}
	allowed := actor.Role == service.RoleAdmin
	for _, permission := range allowedPermissions {
		if permission != "" && service.HasAdminPermission(&actor, permission) {
			allowed = true
		}
	}
	if actor.Status != service.StatusActive || !allowed {
		return nil, service.ErrDepartmentScopeDenied
	}
	if actor.Role == service.RoleSubAdmin && service.HasAdminPermission(&actor, service.AdminPermissionDepartmentSubscriptions) && service.HasAdminPermission(&actor, service.AdminPermissionSubscriptions) {
		return nil, service.ErrDepartmentScopeDenied
	}
	return &actor, nil
}

// Lock all users in the same order. Grant and membership writers share these
// locks with subscription resets so their authorization cannot change mid-write.
func lockDepartmentUsers(ctx context.Context, q sqlExecutor, userIDs []int64) error {
	rows, err := q.QueryContext(ctx, `SELECT id FROM users WHERE id=ANY($1) AND deleted_at IS NULL ORDER BY id FOR UPDATE`, pq.Array(userIDs))
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	count := 0
	for rows.Next() {
		count++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	unique := map[int64]bool{}
	for _, id := range userIDs {
		unique[id] = true
	}
	if count != len(unique) {
		return service.ErrDepartmentScopeDenied
	}
	return nil
}

func departmentAudit(ctx context.Context, q sqlExecutor, actor *service.User, action string, details map[string]any) error {
	entry := &service.AuditLog{Action: action, Method: "INTERNAL", StatusCode: 200, Extra: details}
	if actor != nil {
		entry.ActorUserID, entry.ActorEmail, entry.ActorRole = &actor.ID, actor.Email, actor.Role
	}
	args := auditLogInsertValues(entry)
	placeholders := make([]string, len(args))
	for i := range args {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
	}
	_, err := q.ExecContext(ctx, `INSERT INTO audit_logs (`+auditLogInsertColumns+`) VALUES (`+strings.Join(placeholders, ",")+`)`, args...)
	return err
}

const departmentColumns = `d.id,d.organization_key,d.name,d.status,d.sort_order,d.version,d.created_at,d.updated_at`

func scanDepartment(rows interface{ Scan(...any) error }) (service.Department, error) {
	d := service.Department{Managers: []service.DepartmentManager{}}
	err := rows.Scan(&d.ID, &d.Organization, &d.Name, &d.Status, &d.SortOrder, &d.Version, &d.CreatedAt, &d.UpdatedAt)
	return d, err
}

func queryDepartments(ctx context.Context, q sqlExecutor, actor *service.User) ([]service.Department, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+departmentColumns+` FROM departments d
WHERE ($1 OR EXISTS (SELECT 1 FROM department_access_grants g WHERE g.department_id=d.id AND g.user_id=$2))
ORDER BY d.organization_key,d.sort_order,d.id`, actor.Role == service.RoleAdmin, actor.ID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := []service.Department{}
	for rows.Next() {
		d, err := scanDepartment(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, d)
	}
	return items, rows.Err()
}

func (r *departmentRepository) List(ctx context.Context, f service.DepartmentListFilter) (*service.DepartmentList, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = loadDepartmentActor(ctx, tx, ""); err != nil {
		return nil, err
	}
	p := departmentPredicates{clauses: []string{"TRUE"}}
	if f.Organization != "all" {
		p.add("d.organization_key=$%d", f.Organization)
	}
	if f.Status != "" {
		p.add("d.status=$%d", f.Status)
	}
	if f.Q != "" {
		p.add("d.name ILIKE $%d ESCAPE E'\\\\'", organizationUsageSearchPattern(f.Q))
	}
	result := &service.DepartmentList{Items: []service.Department{}, Page: f.Page, PageSize: f.PageSize}
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM departments d WHERE `+p.where(), p.args...).Scan(&result.Total); err != nil {
		return nil, err
	}
	p.args = append(p.args, f.PageSize, (f.Page-1)*f.PageSize)
	statement := `WITH page AS MATERIALIZED (SELECT ` + departmentColumns + ` FROM departments d WHERE ` + p.where() + fmt.Sprintf(` ORDER BY d.organization_key,d.sort_order,d.id LIMIT $%d OFFSET $%d)`, len(p.args)-1, len(p.args)) + `
 SELECT ` + departmentColumns + `,COUNT(u.id),COUNT(u.id) FILTER(WHERE u.status='active') FROM page d
 LEFT JOIN users u ON u.department_id=d.id AND u.deleted_at IS NULL AND ` + organizationUsageOrganizationExpression("u") + `=d.organization_key
 GROUP BY ` + departmentColumns + ` ORDER BY d.organization_key,d.sort_order,d.id`
	rows, err := tx.QueryContext(ctx, statement, p.args...)
	if err != nil {
		return nil, err
	}
	ids := []int64{}
	positions := map[int64]int{}
	for rows.Next() {
		d := service.Department{Managers: []service.DepartmentManager{}}
		if err = rows.Scan(&d.ID, &d.Organization, &d.Name, &d.Status, &d.SortOrder, &d.Version, &d.CreatedAt, &d.UpdatedAt, &d.MemberCount, &d.ActiveMemberCount); err != nil {
			_ = rows.Close()
			return nil, err
		}
		positions[d.ID] = len(result.Items)
		ids = append(ids, d.ID)
		result.Items = append(result.Items, d)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return nil, err
	}
	if len(ids) > 0 {
		rows, err = tx.QueryContext(ctx, `SELECT g.department_id,u.id,u.email FROM department_access_grants g JOIN users u ON u.id=g.user_id WHERE g.department_id=ANY($1::bigint[]) AND u.deleted_at IS NULL AND u.role='sub_admin' ORDER BY g.department_id,u.id`, pq.Array(ids))
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id int64
			var manager service.DepartmentManager
			if err = rows.Scan(&id, &manager.ID, &manager.Email); err != nil {
				_ = rows.Close()
				return nil, err
			}
			i := positions[id]
			result.Items[i].Managers = append(result.Items[i].Managers, manager)
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return nil, err
		}
	}
	return result, tx.Commit()
}

func (r *departmentRepository) Save(ctx context.Context, id int64, in service.DepartmentSaveInput) (*service.Department, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if err = lockDepartmentUsers(ctx, tx, []int64{service.DepartmentActorID(ctx)}); err != nil {
		return nil, err
	}
	actor, err := loadDepartmentActor(ctx, tx, "")
	if err != nil {
		return nil, err
	}
	if id == 0 {
		err = tx.QueryRowContext(ctx, `INSERT INTO departments(organization_key,name,status,sort_order) VALUES($1,$2,$3,$4) RETURNING id`, in.Organization, in.Name, in.Status, in.SortOrder).Scan(&id)
	} else {
		var result sql.Result
		result, err = tx.ExecContext(ctx, `UPDATE departments SET name=$1,status=$2,sort_order=$3,version=version+1,updated_at=NOW()
WHERE id=$4 AND organization_key=$5 AND version=$6`, in.Name, in.Status, in.SortOrder, id, in.Organization, *in.ExpectedVersion)
		if err == nil {
			n, e := result.RowsAffected()
			if e != nil {
				return nil, e
			}
			if n != 1 {
				return nil, service.ErrDepartmentConflict
			}
		}
	}
	if err != nil {
		return nil, departmentPersistenceError(err)
	}
	if err = departmentAudit(ctx, tx, actor, "department.save", map[string]any{"department_id": id, "input": in}); err != nil {
		return nil, err
	}
	d, err := scanDepartment(tx.QueryRowContext(ctx, `SELECT `+departmentColumns+` FROM departments d WHERE id=$1`, id))
	if err != nil {
		return nil, err
	}
	return &d, tx.Commit()
}

const departmentMemberColumns = `u.id,u.email,u.username,u.status,` // organization expression is appended below.

func (r *departmentRepository) Members(ctx context.Context, f service.DepartmentListFilter) (*service.DepartmentMemberList, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	actor, err := loadDepartmentActor(ctx, tx, "")
	if err != nil {
		return nil, err
	}
	p := departmentMemberPredicates(actor, false, f)
	result := &service.DepartmentMemberList{Items: []service.DepartmentMember{}, Page: f.Page, PageSize: f.PageSize}
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*)`+departmentMemberFrom()+` WHERE `+p.where(), p.args...).Scan(&result.Total); err != nil {
		return nil, err
	}
	result.Items, err = querySelectedDepartmentMembers(ctx, tx, actor, false, f)
	if err != nil {
		return nil, err
	}
	return result, tx.Commit()
}

func sameDepartment(a, b *int64) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

func (r *departmentRepository) Assign(ctx context.Context, in service.DepartmentAssignInput) (int, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	ids := []int64{service.DepartmentActorID(ctx)}
	for _, m := range in.Members {
		ids = append(ids, m.UserID)
	}
	if err = lockDepartmentUsers(ctx, tx, ids); err != nil {
		return 0, err
	}
	actor, err := loadDepartmentActor(ctx, tx, "")
	if err != nil {
		return 0, err
	}
	var organization, status string
	if in.DepartmentID != nil {
		err = tx.QueryRowContext(ctx, `SELECT organization_key,status FROM departments WHERE id=$1 FOR SHARE`, *in.DepartmentID).Scan(&organization, &status)
		if errors.Is(err, sql.ErrNoRows) {
			return 0, service.ErrDepartmentNotFound
		}
		if err != nil {
			return 0, err
		}
	}
	changes := []map[string]any{}
	for _, m := range in.Members {
		var email string
		var current *int64
		var version int64
		if err = tx.QueryRowContext(ctx, `SELECT email,department_id,department_version FROM users WHERE id=$1`, m.UserID).Scan(&email, &current, &version); err != nil {
			return 0, err
		}
		if in.DepartmentID != nil && service.OrganizationForEmail(email) != organization {
			return 0, service.ErrDepartmentMemberOrganization
		}
		if sameDepartment(current, in.DepartmentID) {
			continue
		}
		if in.DepartmentID != nil && status != "active" {
			return 0, service.ErrDepartmentInactive
		}
		if !sameDepartment(current, m.ExpectedDepartmentID) || version != m.ExpectedVersion {
			return 0, service.ErrDepartmentConflict
		}
		if _, err = tx.ExecContext(ctx, `UPDATE users SET department_id=$1,updated_at=NOW() WHERE id=$2`, in.DepartmentID, m.UserID); err != nil {
			return 0, departmentPersistenceError(err)
		}
		changes = append(changes, map[string]any{"user_id": m.UserID, "before": current, "after": in.DepartmentID})
	}
	if len(changes) > 0 {
		if err = departmentAudit(ctx, tx, actor, "department.assign_members", map[string]any{"changes": changes}); err != nil {
			return 0, err
		}
	}
	return len(changes), tx.Commit()
}

func loadDepartmentAccess(ctx context.Context, q sqlExecutor, userID int64) (*service.DepartmentAccess, error) {
	access, err := loadAdminAccess(ctx, q, userID)
	if err == nil && access.Role != service.RoleSubAdmin {
		return nil, service.ErrDepartmentInvalid
	}
	return access, err
}

func loadAdminAccess(ctx context.Context, q sqlExecutor, userID int64) (*service.DepartmentAccess, error) {
	var role string
	var raw []byte
	err := scanSingleRow(ctx, q, `SELECT role,admin_permissions FROM users WHERE id=$1 AND deleted_at IS NULL`, []any{userID}, &role, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrDepartmentInvalid
	}
	if err != nil {
		return nil, err
	}
	result := &service.DepartmentAccess{Role: role, UserID: userID, DepartmentIDs: []int64{}, Permissions: []string{}}
	if err = json.Unmarshal(raw, &result.Permissions); err != nil {
		return nil, err
	}
	sort.Strings(result.Permissions)
	rows, err := q.QueryContext(ctx, `SELECT department_id FROM department_access_grants WHERE user_id=$1 ORDER BY department_id`, userID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			_ = rows.Close()
			return nil, err
		}
		result.DepartmentIDs = append(result.DepartmentIDs, id)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return nil, err
	}
	result.Version = service.HashDepartmentScope([]any{result.UserID, result.Role, result.DepartmentIDs, result.Permissions})
	return result, nil
}

func (r *departmentRepository) GetAccess(ctx context.Context, userID int64) (*service.DepartmentAccess, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = loadDepartmentActor(ctx, tx, ""); err != nil {
		return nil, err
	}
	result, err := loadDepartmentAccess(ctx, tx, userID)
	if err != nil {
		return nil, err
	}
	return result, tx.Commit()
}

func (r *departmentRepository) SetAccess(ctx context.Context, userID int64, in service.DepartmentAccessInput) (*service.DepartmentAccess, error) {
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
	oldIDs := map[int64]bool{}
	for _, id := range previous.DepartmentIDs {
		oldIDs[id] = true
	}
	addedIDs := []int64{}
	for _, id := range in.DepartmentIDs {
		if !oldIDs[id] {
			addedIDs = append(addedIDs, id)
		}
	}
	// Retained grants already reference existing departments and may remain inactive.
	// Only additions need department status locks, ordered to match membership writes.
	if len(addedIDs) > 0 {
		rows, err := tx.QueryContext(ctx, `SELECT status FROM departments WHERE id=ANY($1) ORDER BY id FOR SHARE`, pq.Array(addedIDs))
		if err != nil {
			return nil, err
		}
		count := 0
		for rows.Next() {
			var status string
			if err = rows.Scan(&status); err != nil {
				break
			}
			if status != "active" {
				err = service.ErrDepartmentInactive
				break
			}
			count++
		}
		readErr := rows.Err()
		_ = rows.Close()
		if err != nil {
			return nil, err
		}
		if readErr != nil {
			return nil, readErr
		}
		if count != len(addedIDs) {
			return nil, service.ErrDepartmentInvalid
		}
	}
	permissions := []string{}
	for _, p := range previous.Permissions {
		if p == service.AdminPermissionOrganizationUsage || p == service.AdminPermissionDepartmentSubscriptions {
			continue
		}
		if p == service.AdminPermissionSubscriptions && in.ResetQuota {
			if !in.ReplaceGlobalSubscriptions {
				return nil, service.ErrDepartmentGlobalConfirmation
			}
			continue
		}
		permissions = append(permissions, p)
	}
	if in.Report {
		permissions = append(permissions, service.AdminPermissionOrganizationUsage)
	}
	if in.ResetQuota {
		permissions = append(permissions, service.AdminPermissionDepartmentSubscriptions)
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
	if _, err = tx.ExecContext(ctx, `DELETE FROM department_access_grants WHERE user_id=$1 AND NOT (department_id=ANY($2))`, userID, pq.Array(in.DepartmentIDs)); err != nil {
		return nil, err
	}
	if len(addedIDs) > 0 {
		if _, err = tx.ExecContext(ctx, `INSERT INTO department_access_grants(user_id,department_id,created_by) SELECT $1,id,$3 FROM unnest($2::bigint[]) AS id`, userID, pq.Array(addedIDs), actor.ID); err != nil {
			return nil, err
		}
	}
	result, err := loadDepartmentAccess(ctx, tx, userID)
	if err != nil {
		return nil, err
	}
	if err = departmentAudit(ctx, tx, actor, "department.grant_scope", map[string]any{"before": previous, "after": result}); err != nil {
		return nil, err
	}
	return result, tx.Commit()
}

// resolveDepartmentScope must use the same executor/transaction as report reads.
func resolveDepartmentScope(ctx context.Context, q sqlExecutor, permission string) (*service.DepartmentScope, error) {
	permissions := []string{permission}
	if permission == service.AdminPermissionDepartmentSubscriptions {
		permissions = append(permissions, service.AdminPermissionSubscriptions)
	}
	actor, err := loadDepartmentActor(ctx, q, permissions...)
	if err != nil {
		return nil, err
	}
	return departmentCatalogForActor(ctx, q, actor, permission)
}

func departmentCatalogForActor(ctx context.Context, q sqlExecutor, actor *service.User, permission string) (*service.DepartmentScope, error) {
	queryActor := *actor
	if permission == service.AdminPermissionDepartmentSubscriptions && service.HasAdminPermission(actor, service.AdminPermissionSubscriptions) {
		queryActor.Role = service.RoleAdmin
	}
	departments, err := queryDepartments(ctx, q, &queryActor)
	if err != nil {
		return nil, err
	}
	result := &service.DepartmentScope{Unrestricted: queryActor.Role == service.RoleAdmin, Organizations: []string{}, Departments: departments, Actor: actor, DefaultOrganization: "all", DefaultDepartment: "all"}
	for _, org := range []string{service.OrganizationXunyou, service.OrganizationWsdashi, service.OrganizationOther} {
		found := result.Unrestricted
		for _, d := range departments {
			if d.Organization == org {
				found = true
			}
		}
		if found {
			result.Organizations = append(result.Organizations, org)
		}
	}
	if !result.Unrestricted {
		if len(result.Organizations) == 1 {
			result.DefaultOrganization = result.Organizations[0]
		}
		if len(departments) == 1 {
			result.DefaultDepartment = strconv.FormatInt(departments[0].ID, 10)
		}
	}
	result.CatalogVersion = service.HashDepartmentScope([]any{actor.ID, actor.Role, actor.AdminPermissions, departments})
	return result, nil
}

func (r *departmentRepository) Scope(ctx context.Context, permission string) (*service.DepartmentScope, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := resolveDepartmentScope(ctx, tx, permission)
	if err != nil {
		return nil, err
	}
	return result, tx.Commit()
}

func (r *departmentRepository) SubscriptionGroups(ctx context.Context, search string) ([]service.DepartmentGroupOption, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	scope, err := resolveDepartmentScope(ctx, tx, service.AdminPermissionDepartmentSubscriptions)
	if err != nil {
		return nil, err
	}
	if err = scope.ValidateSelection("all", "all"); err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT g.id,g.name FROM groups g
WHERE ($1 OR EXISTS(SELECT 1 FROM user_subscriptions s JOIN users u ON u.id=s.user_id JOIN departments d ON d.id=u.department_id AND d.organization_key=`+organizationUsageOrganizationExpression("u")+` WHERE s.group_id=g.id AND u.deleted_at IS NULL AND u.status='active' AND EXISTS(SELECT 1 FROM department_access_grants dg WHERE dg.user_id=$2 AND dg.department_id=d.id)))
AND ($3='' OR g.name ILIKE $3 ESCAPE E'\\') ORDER BY g.name,g.id LIMIT 1000`, scope.Unrestricted, service.DepartmentActorID(ctx), organizationUsageSearchPattern(search))
	if err != nil {
		return nil, err
	}
	result := []service.DepartmentGroupOption{}
	for rows.Next() {
		var option service.DepartmentGroupOption
		if err = rows.Scan(&option.ID, &option.Name); err != nil {
			_ = rows.Close()
			return nil, err
		}
		result = append(result, option)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return nil, err
	}
	return result, tx.Commit()
}
