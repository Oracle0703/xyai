package repository

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type departmentPredicates struct {
	clauses []string
	args    []any
}

func (p *departmentPredicates) add(expression string, value any) {
	p.args = append(p.args, value)
	p.clauses = append(p.clauses, fmt.Sprintf(expression, len(p.args)))
}
func (p *departmentPredicates) where() string { return strings.Join(p.clauses, " AND ") }

func departmentMemberPredicates(actor *service.User, activeOnly bool, f service.DepartmentListFilter) departmentPredicates {
	p := departmentPredicates{clauses: []string{"u.deleted_at IS NULL"}}
	if activeOnly {
		p.clauses = append(p.clauses, "u.status='active'")
	}
	if actor.Role != service.RoleAdmin {
		p.add("EXISTS(SELECT 1 FROM department_access_grants dg WHERE dg.user_id=$%d AND dg.department_id=d.id)", actor.ID)
	}
	if f.Organization != "" && f.Organization != "all" {
		p.add(organizationUsageOrganizationExpression("u")+"=$%d", f.Organization)
	}
	if f.DepartmentID == "unassigned" {
		p.clauses = append(p.clauses, "d.id IS NULL")
	} else if f.DepartmentID != "" && f.DepartmentID != "all" {
		p.add("d.id=$%d::bigint", f.DepartmentID)
	}
	if f.Q != "" {
		p.add("u.email ILIKE $%d ESCAPE E'\\\\'", organizationUsageSearchPattern(f.Q))
	}
	if f.Status != "" {
		p.add("u.status=$%d", f.Status)
	}
	if len(f.UserIDs) > 0 {
		p.add("u.id=ANY($%d::bigint[])", pq.Array(f.UserIDs))
	}
	return p
}

func departmentMemberFrom() string {
	return ` FROM users u LEFT JOIN departments d ON d.id=u.department_id AND d.organization_key=` + organizationUsageOrganizationExpression("u")
}

func querySelectedDepartmentMembers(ctx context.Context, q sqlExecutor, actor *service.User, activeOnly bool, f service.DepartmentListFilter) ([]service.DepartmentMember, error) {
	p := departmentMemberPredicates(actor, activeOnly, f)
	statement := `SELECT ` + departmentMemberColumns + organizationUsageOrganizationExpression("u") + `,d.id,COALESCE(d.name,''),u.department_version` + departmentMemberFrom() + ` WHERE ` + p.where() + ` ORDER BY u.id`
	if f.PageSize > 0 {
		p.args = append(p.args, f.PageSize, max(0, f.Page-1)*f.PageSize)
		statement += fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(p.args)-1, len(p.args))
	}
	rows, err := q.QueryContext(ctx, statement, p.args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := []service.DepartmentMember{}
	for rows.Next() {
		var m service.DepartmentMember
		if err := rows.Scan(&m.ID, &m.Email, &m.Username, &m.Status, &m.Organization, &m.DepartmentID, &m.DepartmentName, &m.DepartmentVersion); err != nil {
			return nil, err
		}
		result = append(result, m)
	}
	return result, rows.Err()
}

func departmentSubscriptionUnrestricted(actor *service.User) bool {
	return actor.Role == service.RoleAdmin || service.HasAdminPermission(actor, service.AdminPermissionSubscriptions)
}

func resolveDepartmentQueryScope(ctx context.Context, q sqlExecutor, permission string, selection service.DepartmentScopeQuery) (*service.DepartmentScope, error) {
	permissions := []string{permission}
	if permission == service.AdminPermissionDepartmentSubscriptions {
		permissions = append(permissions, service.AdminPermissionSubscriptions)
	}
	actor, err := loadDepartmentActor(ctx, q, permissions...)
	if err != nil {
		return nil, err
	}
	return departmentQueryScopeForActor(ctx, q, actor, permission, selection)
}

func departmentQueryScopeForActor(ctx context.Context, q sqlExecutor, actor *service.User, permission string, selection service.DepartmentScopeQuery) (*service.DepartmentScope, error) {
	org, dept, err := service.NormalizeDepartmentFilter(selection.Organization, selection.DepartmentID)
	if err != nil {
		return nil, err
	}
	selection.Organization, selection.DepartmentID = org, dept
	unrestricted := actor.Role == service.RoleAdmin || (permission == service.AdminPermissionDepartmentSubscriptions && departmentSubscriptionUnrestricted(actor))
	// Global subscription operations require identity checks, not a user directory scan.
	if unrestricted && permission == service.AdminPermissionDepartmentSubscriptions && dept == "all" && !selection.Versioned && selection.Limit == 0 {
		return &service.DepartmentScope{Unrestricted: true, Actor: actor}, nil
	}
	scope, err := departmentCatalogForActor(ctx, q, actor, permission)
	if err != nil {
		return nil, err
	}
	if err = scope.ValidateSelection(org, dept, ""); err != nil {
		return nil, err
	}
	queryActor := *actor
	if unrestricted {
		queryActor.Role = service.RoleAdmin
	}
	f := service.DepartmentListFilter{Organization: org, DepartmentID: dept, Q: selection.Q, Page: 1, PageSize: selection.Limit}
	if selection.UserID != nil {
		f.UserIDs = []int64{*selection.UserID}
	}
	scope.Members, err = querySelectedDepartmentMembers(ctx, q, &queryActor, true, f)
	if err != nil {
		return nil, err
	}
	if selection.UserID != nil && len(scope.Members) == 0 && !unrestricted {
		return nil, service.ErrDepartmentScopeDenied
	}
	if selection.Limit > 0 {
		return scope, nil
	}
	type departmentVersion struct {
		ID                 int64
		Organization, Name string
	}
	type memberVersion struct {
		ID                  int64
		Email, Organization string
		DepartmentID        *int64
		Version             int64
	}
	departments := []departmentVersion{}
	for _, d := range scope.Departments {
		if org != "all" && d.Organization != org {
			continue
		}
		if dept == "unassigned" || (dept != "all" && strconv.FormatInt(d.ID, 10) != dept) {
			continue
		}
		departments = append(departments, departmentVersion{d.ID, d.Organization, d.Name})
	}
	sort.Slice(departments, func(i, j int) bool { return departments[i].ID < departments[j].ID })
	members := make([]memberVersion, 0, len(scope.Members))
	for _, m := range scope.Members {
		members = append(members, memberVersion{m.ID, m.Email, m.Organization, m.DepartmentID, m.DepartmentVersion})
	}
	platform := strings.ToLower(strings.TrimSpace(selection.Platform))
	if platform == "" {
		platform = "all"
	}
	scope.Version = service.HashDepartmentScope([]any{actor.ID, actor.Role, permission, unrestricted, org, dept, strings.ToLower(strings.TrimSpace(selection.Q)), platform, selection.UserID, selection.GroupID, selection.Status, departments, members})
	return scope, nil
}

func (r *departmentRepository) QueryScope(ctx context.Context, permission string, selection service.DepartmentScopeQuery) (*service.DepartmentScope, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	scope, err := resolveDepartmentQueryScope(ctx, tx, permission, selection)
	if err != nil {
		return nil, err
	}
	return scope, tx.Commit()
}
