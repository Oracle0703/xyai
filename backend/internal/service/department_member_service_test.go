package service

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type managedDepartmentRecorder struct {
	DepartmentRepository
	scope               *DepartmentScope
	scopeErr            error
	permission          string
	filter              DepartmentListFilter
	candidateDepartment int64
	query               string
	assignment          DepartmentAssignInput
	update              ManagedMemberUpdateInput
	updateUserID        int64
	updateResult        *ManagedDepartmentMember
	updateErr           error
	statusChanged       bool
	committed           bool
}

func (r *managedDepartmentRecorder) Scope(_ context.Context, permission string) (*DepartmentScope, error) {
	r.permission = permission
	return r.scope, r.scopeErr
}

func (r *managedDepartmentRecorder) ManagedMembers(_ context.Context, filter DepartmentListFilter) (*ManagedDepartmentMemberList, error) {
	r.filter = filter
	return &ManagedDepartmentMemberList{Page: filter.Page, PageSize: filter.PageSize}, nil
}

func (r *managedDepartmentRecorder) ManagedCandidates(_ context.Context, departmentID int64, query string) ([]DepartmentMember, error) {
	r.candidateDepartment, r.query = departmentID, query
	return []DepartmentMember{{ID: 42}}, nil
}

func (r *managedDepartmentRecorder) ManagedAssign(_ context.Context, input DepartmentAssignInput) (int, error) {
	r.assignment = input
	return len(input.Members), nil
}

func (r *managedDepartmentRecorder) UpdateManagedMember(_ context.Context, userID int64, input ManagedMemberUpdateInput) (*ManagedDepartmentMember, bool, error) {
	r.updateUserID, r.update = userID, input
	r.committed = r.updateErr == nil
	return r.updateResult, r.statusChanged, r.updateErr
}

type managedDepartmentCacheRecorder struct {
	APIKeyAuthCacheInvalidator
	onInvalidate func(int64)
}

func (c *managedDepartmentCacheRecorder) InvalidateAuthCacheByUserID(_ context.Context, userID int64) {
	c.onInvalidate(userID)
}

func TestDepartmentManagedScopeRequiresCurrentGrant(t *testing.T) {
	ctx := context.Background()
	readErr := errors.New("scope read failed")
	for _, tt := range []struct {
		name    string
		scope   *DepartmentScope
		repoErr error
		wantErr error
	}{
		{"missing scope", nil, nil, ErrDepartmentScopeDenied},
		{"no grants", &DepartmentScope{}, nil, ErrDepartmentScopeDenied},
		{"granted", &DepartmentScope{Departments: []Department{{ID: 7}}}, nil, nil},
		{"unrestricted admin", &DepartmentScope{Unrestricted: true}, nil, nil},
		{"repository failure", nil, readErr, readErr},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := &managedDepartmentRecorder{scope: tt.scope, scopeErr: tt.repoErr}
			result, err := NewDepartmentService(repo, nil).ManagedScope(ctx)
			require.ErrorIs(t, err, tt.wantErr)
			require.Equal(t, AdminPermissionDepartmentMembers, repo.permission)
			if tt.wantErr != nil {
				require.Nil(t, result)
			} else {
				require.Same(t, tt.scope, result)
			}
		})
	}
	var missing *DepartmentService
	_, err := missing.ManagedScope(ctx)
	require.ErrorIs(t, err, ErrDepartmentScopeDenied)
}

func TestDepartmentManagedMembersAcceptsDepartmentWithoutOrganization(t *testing.T) {
	for _, organization := range []string{"", OrganizationAll} {
		repo := &managedDepartmentRecorder{}
		result, err := NewDepartmentService(repo, nil).ManagedMembers(context.Background(), DepartmentListFilter{
			Organization: organization, DepartmentID: " 7 ", Status: StatusDisabled, Q: " 中文 ",
		})
		require.NoError(t, err)
		require.Equal(t, DepartmentListFilter{Organization: OrganizationAll, DepartmentID: "7", Status: StatusDisabled, Q: "中文", Page: 1, PageSize: 20}, repo.filter)
		require.Equal(t, 1, result.Page)
		require.Equal(t, 20, result.PageSize)
	}
	for _, filter := range []DepartmentListFilter{
		{DepartmentID: "unassigned"}, {DepartmentID: "01"}, {DepartmentID: "-1"},
		{Status: "inactive"}, {Status: "deleted"}, {PageSize: 201},
	} {
		_, err := NewDepartmentService(nil, nil).ManagedMembers(context.Background(), filter)
		require.ErrorIs(t, err, ErrDepartmentInvalid, "%+v", filter)
	}
}

func TestDepartmentManagedCandidatesCountsCharactersAfterTrimming(t *testing.T) {
	for _, input := range []struct {
		department int64
		query      string
	}{{0, "ab"}, {-1, "ab"}, {7, ""}, {7, " a "}, {7, " 名 "}} {
		_, err := NewDepartmentService(nil, nil).ManagedCandidates(context.Background(), input.department, input.query)
		require.ErrorIs(t, err, ErrDepartmentInvalid)
	}
	repo := &managedDepartmentRecorder{}
	result, err := NewDepartmentService(repo, nil).ManagedCandidates(context.Background(), 7, " 中文 ")
	require.NoError(t, err)
	require.Equal(t, int64(7), repo.candidateDepartment)
	require.Equal(t, "中文", repo.query)
	require.Equal(t, []DepartmentMember{{ID: 42}}, result)
}

func TestDepartmentManagedAssignValidatesBatchBeforeRepository(t *testing.T) {
	zero, negative := int64(0), int64(-1)
	for _, input := range []DepartmentAssignInput{
		{},
		{Members: make([]DepartmentMemberChange, 201)},
		{DepartmentID: &zero, Members: []DepartmentMemberChange{{UserID: 1}}},
		{DepartmentID: &negative, Members: []DepartmentMemberChange{{UserID: 1}}},
		{Members: []DepartmentMemberChange{{UserID: 0}}},
		{Members: []DepartmentMemberChange{{UserID: 1}, {UserID: 1}}},
		{Members: []DepartmentMemberChange{{UserID: 1, ExpectedVersion: -1}}},
		{Members: []DepartmentMemberChange{{UserID: 1, ExpectedDepartmentID: &zero}}},
	} {
		_, err := NewDepartmentService(nil, nil).ManagedAssign(context.Background(), input)
		require.ErrorIs(t, err, ErrDepartmentInvalid, "%+v", input)
	}
	repo := &managedDepartmentRecorder{}
	count, err := NewDepartmentService(repo, nil).ManagedAssign(context.Background(), DepartmentAssignInput{
		Members: []DepartmentMemberChange{{UserID: 9, ExpectedVersion: 2}, {UserID: 4, ExpectedVersion: 3}},
	})
	require.NoError(t, err)
	require.Equal(t, 2, count)
	require.Equal(t, []DepartmentMemberChange{{UserID: 4, ExpectedVersion: 3}, {UserID: 9, ExpectedVersion: 2}}, repo.assignment.Members)
}

func TestDepartmentManagedUpdateRejectsInvalidInputBeforeRepository(t *testing.T) {
	departmentID, version, zero, negative := int64(7), int64(0), int64(0), int64(-1)
	str := func(value string) *string { return &value }
	for _, tt := range []struct {
		name   string
		userID int64
		input  ManagedMemberUpdateInput
	}{
		{"missing user", 0, ManagedMemberUpdateInput{ExpectedDepartmentID: &departmentID, ExpectedDepartmentVersion: &version}},
		{"invalid department", 42, ManagedMemberUpdateInput{ExpectedDepartmentID: &zero, ExpectedDepartmentVersion: &version}},
		{"missing version", 42, ManagedMemberUpdateInput{ExpectedDepartmentID: &departmentID}},
		{"invalid version", 42, ManagedMemberUpdateInput{ExpectedDepartmentID: &departmentID, ExpectedDepartmentVersion: &negative}},
		{"ASCII byte limit", 42, ManagedMemberUpdateInput{ExpectedDepartmentID: &departmentID, ExpectedDepartmentVersion: &version, Username: str(strings.Repeat("a", 101))}},
		{"UTF8 byte limit", 42, ManagedMemberUpdateInput{ExpectedDepartmentID: &departmentID, ExpectedDepartmentVersion: &version, Username: str(strings.Repeat("名", 34))}},
		{"note character limit", 42, ManagedMemberUpdateInput{ExpectedDepartmentID: &departmentID, ExpectedDepartmentVersion: &version, DepartmentNote: str(strings.Repeat("备", 1001))}},
		{"unknown status", 42, ManagedMemberUpdateInput{ExpectedDepartmentID: &departmentID, ExpectedDepartmentVersion: &version, Status: str("banned")}},
		{"blank status", 42, ManagedMemberUpdateInput{ExpectedDepartmentID: &departmentID, ExpectedDepartmentVersion: &version, Status: str("")}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewDepartmentService(nil, nil).UpdateManagedMember(context.Background(), tt.userID, tt.input)
			require.ErrorIs(t, err, ErrDepartmentInvalid)
		})
	}
}

func TestDepartmentManagedUpdateAuthorizesBeforeMissingDepartmentError(t *testing.T) {
	version := int64(0)
	for _, tt := range []struct {
		name string
		err  error
	}{
		{"unmanageable target", ErrDepartmentScopeDenied},
		{"manageable target with missing expected department", ErrDepartmentInvalid},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := &managedDepartmentRecorder{updateErr: tt.err}
			result, err := NewDepartmentService(repo, nil).UpdateManagedMember(context.Background(), 42, ManagedMemberUpdateInput{
				ExpectedDepartmentVersion: &version,
			})
			require.ErrorIs(t, err, tt.err)
			require.Nil(t, result)
			require.Equal(t, int64(42), repo.updateUserID, "the repository must decide current target authorization first")
			require.Nil(t, repo.update.ExpectedDepartmentID)
		})
	}
}

func TestDepartmentManagedUpdateNormalizesOnlySubmittedFields(t *testing.T) {
	departmentID, version := int64(7), int64(0)
	for _, username := range []string{strings.Repeat("a", 100), strings.Repeat("名", 33), ""} {
		inputUsername := "  " + username + "  "
		inputNote := "  " + strings.Repeat("备", 1000) + "  "
		repo := &managedDepartmentRecorder{updateResult: &ManagedDepartmentMember{ID: 42}}
		_, err := NewDepartmentService(repo, nil).UpdateManagedMember(context.Background(), 42, ManagedMemberUpdateInput{
			ExpectedDepartmentID: &departmentID, ExpectedDepartmentVersion: &version,
			Username: &inputUsername, DepartmentNote: &inputNote,
		})
		require.NoError(t, err)
		require.Equal(t, username, *repo.update.Username)
		require.Equal(t, strings.Repeat("备", 1000), *repo.update.DepartmentNote)
		require.Nil(t, repo.update.Status, "a profile edit never replays a stale status")
		require.Equal(t, "  "+username+"  ", inputUsername, "normalization does not mutate the caller's snapshot")
	}
}

func TestDepartmentManagedUpdateInvalidatesOnlyCommittedStatusChanges(t *testing.T) {
	departmentID, version, status := int64(7), int64(2), StatusDisabled
	commitErr := errors.New("audit or commit failed")
	for _, tt := range []struct {
		name          string
		statusChanged bool
		err           error
		withCache     bool
		wantCalls     int
	}{
		{"committed change", true, nil, true, 1},
		{"unchanged state", false, nil, true, 0},
		{"failed commit", true, commitErr, true, 0},
		{"nil invalidator", true, nil, false, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := &managedDepartmentRecorder{updateResult: &ManagedDepartmentMember{ID: 42}, updateErr: tt.err, statusChanged: tt.statusChanged}
			var cache APIKeyAuthCacheInvalidator
			var invalidated []int64
			if tt.withCache {
				cache = &managedDepartmentCacheRecorder{onInvalidate: func(userID int64) {
					require.True(t, repo.committed, "cache invalidation must follow a successful repository commit")
					invalidated = append(invalidated, userID)
				}}
			}
			result, err := NewDepartmentService(repo, cache).UpdateManagedMember(context.Background(), 42, ManagedMemberUpdateInput{
				ExpectedDepartmentID: &departmentID, ExpectedDepartmentVersion: &version, Status: &status,
			})
			require.ErrorIs(t, err, tt.err)
			require.Len(t, invalidated, tt.wantCalls)
			if tt.wantCalls > 0 {
				require.Equal(t, int64(42), invalidated[0])
			}
			if tt.err != nil {
				require.Nil(t, result)
			} else {
				require.Same(t, repo.updateResult, result)
			}
		})
	}
}

func TestDepartmentMemberPermissionOnlyGrantsDedicatedRoutes(t *testing.T) {
	user := &User{Role: RoleSubAdmin, AdminPermissions: []string{AdminPermissionDepartmentMembers}}
	routes := []string{
		"GET /api/v1/admin/department-members/scope",
		"GET /api/v1/admin/department-members",
		"GET /api/v1/admin/department-members/candidates",
		"POST /api/v1/admin/department-members/assign",
		"PUT /api/v1/admin/department-members/:id",
	}
	require.ElementsMatch(t, routes, AdminPermissionRouteTemplates(AdminPermissionDepartmentMembers))
	for _, route := range routes {
		method, path, _ := strings.Cut(route, " ")
		require.True(t, CanAccessAdminRoute(user, method, path))
		for _, permission := range []string{AdminPermissionOrganizationUsage, AdminPermissionDepartmentSubscriptions, AdminPermissionSubscriptions} {
			require.False(t, CanAccessAdminRoute(&User{Role: RoleSubAdmin, AdminPermissions: []string{permission}}, method, path), permission)
		}
	}
	for _, route := range []adminRouteRule{
		{http.MethodGet, "/api/v1/admin/users"},
		{http.MethodGet, "/api/v1/admin/departments"},
		{http.MethodPost, "/api/v1/admin/departments/assign-members"},
		{http.MethodPut, "/api/v1/admin/users/:id/department-scope"},
		{http.MethodGet, "/api/v1/admin/usage/organization-report/summary"},
		{http.MethodGet, "/api/v1/admin/subscriptions"},
		{http.MethodPost, "/api/v1/admin/subscriptions/:id/reset-quota"},
		{http.MethodDelete, "/api/v1/admin/department-members/:id"},
	} {
		require.False(t, CanAccessAdminRoute(user, route.method, route.route), route)
	}
	permissions, err := NormalizeAdminPermissions(RoleSubAdmin, []string{AdminPermissionDepartmentMembers})
	require.NoError(t, err)
	require.Equal(t, []string{AdminPermissionDepartmentMembers}, permissions)
	require.Contains(t, AdminPermissionCatalog(), AdminPermissionCatalogItem{Code: AdminPermissionDepartmentMembers, MenuKey: "department_members", Route: "/admin/my-department"})
}
