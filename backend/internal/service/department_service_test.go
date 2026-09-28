package service

import (
	"context"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDepartmentScopeDoesNotExpandWithAllOrMissingGrants(t *testing.T) {
	s := &DepartmentScope{Organizations: []string{OrganizationXunyou}, Departments: []Department{{ID: 7, Organization: OrganizationXunyou}}, Version: "current"}
	require.NoError(t, s.ValidateSelection("all", "all"))
	require.ErrorIs(t, s.ValidateSelection(OrganizationWsdashi, "all"), ErrDepartmentScopeDenied)
	require.ErrorIs(t, s.ValidateSelection(OrganizationXunyou, "8"), ErrDepartmentScopeDenied)
	require.ErrorIs(t, s.ValidateSelection(OrganizationXunyou, "unassigned"), ErrDepartmentScopeDenied)
	require.ErrorIs(t, (&DepartmentScope{}).ValidateSelection("all", "all"), ErrDepartmentScopeDenied)
}

func TestDepartmentSelectionRequiresMatchingOrganization(t *testing.T) {
	for _, department := range []string{"0", "-1", "01", "7 OR 1=1"} {
		_, _, err := NormalizeDepartmentFilter(OrganizationXunyou, department)
		require.Error(t, err, department)
	}
	_, _, err := NormalizeDepartmentFilter("all", "7")
	require.Error(t, err)
	s := &DepartmentScope{Unrestricted: true, Departments: []Department{{ID: 7, Organization: OrganizationXunyou}}}
	require.ErrorIs(t, s.ValidateSelection(OrganizationWsdashi, "7"), ErrDepartmentInvalid)
	org, dept, err := NormalizeDepartmentFilter("", "")
	require.NoError(t, err)
	require.Equal(t, "all", org)
	require.Equal(t, "all", dept)
}

func TestDepartmentOrganizationUsesExactDomain(t *testing.T) {
	require.Equal(t, OrganizationOther, OrganizationForEmail("dev@team.xunyou.com"))
	require.Equal(t, OrganizationWsdashi, OrganizationForEmail("DEV@WSDASHI.COM"))
}

type departmentFilterRecorder struct {
	DepartmentRepository
	filter DepartmentListFilter
}

func (r *departmentFilterRecorder) Members(_ context.Context, f DepartmentListFilter) (*DepartmentMemberList, error) {
	r.filter = f
	return &DepartmentMemberList{}, nil
}

func TestDepartmentMembersValidateBeforeQuery(t *testing.T) {
	for _, f := range []DepartmentListFilter{
		{Page: math.MaxInt, PageSize: 200}, {Page: -1}, {PageSize: 201},
		{UserIDs: []int64{1, 1}}, {UserIDs: []int64{0}}, {UserIDs: make([]int64, 201)},
		{DepartmentID: "01"}, {DepartmentID: "7 OR 1=1"},
		{Organization: "unknown"}, {Status: "deleted"},
	} {
		// No repository: every invalid request must be rejected before invoking it.
		_, err := NewDepartmentService(nil).Members(context.Background(), f)
		require.ErrorIs(t, err, ErrDepartmentInvalid, "%+v", f)
	}
	r := &departmentFilterRecorder{}
	_, err := NewDepartmentService(r).Members(context.Background(), DepartmentListFilter{DepartmentID: "7", Q: " dev "})
	require.NoError(t, err)
	require.Equal(t, DepartmentListFilter{Organization: "all", DepartmentID: "7", Q: "dev", Page: 1, PageSize: 20}, r.filter)
}

func TestDepartmentSubscriptionPermissionNeverGrantsAssignment(t *testing.T) {
	u := &User{Role: RoleSubAdmin, AdminPermissions: []string{AdminPermissionDepartmentSubscriptions}}
	for _, route := range []string{"/api/v1/admin/subscriptions/:id/reset-quota", "/api/v1/admin/subscriptions/reset-daily-filtered"} {
		require.True(t, CanAccessAdminRoute(u, "POST", route))
	}
	for _, route := range []string{"/api/v1/admin/subscriptions/assign", "/api/v1/admin/subscriptions/bulk-action", "/api/v1/admin/subscriptions/:id/extend", "/api/v1/admin/subscriptions/:id/revoke", "/api/v1/admin/subscriptions/:id/restore", "/api/v1/admin/departments"} {
		require.False(t, CanAccessAdminRoute(u, "POST", route), route)
	}
	require.False(t, CanAccessAdminRoute(u, "GET", "/api/v1/admin/usage/search-users"))
	_, err := NormalizeAdminPermissions(RoleSubAdmin, []string{AdminPermissionSubscriptions, AdminPermissionDepartmentSubscriptions})
	require.ErrorContains(t, err, "mutually exclusive")
}
