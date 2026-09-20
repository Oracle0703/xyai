package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDepartmentScopeDoesNotExpandWithAllOrMissingGrants(t *testing.T) {
	s := &DepartmentScope{Organizations: []string{OrganizationXunyou}, Departments: []Department{{ID: 7, Organization: OrganizationXunyou}}, Version: "current"}
	require.NoError(t, s.ValidateSelection("all", "all", ""))
	require.ErrorIs(t, s.ValidateSelection(OrganizationWsdashi, "all", ""), ErrDepartmentScopeDenied)
	require.ErrorIs(t, s.ValidateSelection(OrganizationXunyou, "8", ""), ErrDepartmentScopeDenied)
	require.ErrorIs(t, s.ValidateSelection(OrganizationXunyou, "unassigned", ""), ErrDepartmentScopeDenied)
	require.ErrorIs(t, s.ValidateSelection(OrganizationXunyou, "7", "stale"), ErrDepartmentScopeChanged)
	require.ErrorIs(t, (&DepartmentScope{}).ValidateSelection("all", "all", ""), ErrDepartmentScopeDenied)
}

func TestDepartmentSelectionRequiresMatchingOrganization(t *testing.T) {
	for _, department := range []string{"0", "-1", "01", "7 OR 1=1"} {
		_, _, err := NormalizeDepartmentFilter(OrganizationXunyou, department)
		require.Error(t, err, department)
	}
	_, _, err := NormalizeDepartmentFilter("all", "7")
	require.Error(t, err)
	s := &DepartmentScope{Unrestricted: true, Departments: []Department{{ID: 7, Organization: OrganizationXunyou}}}
	require.ErrorIs(t, s.ValidateSelection(OrganizationWsdashi, "7", ""), ErrDepartmentInvalid)
	org, dept, err := NormalizeDepartmentFilter("", "")
	require.NoError(t, err)
	require.Equal(t, "all", org)
	require.Equal(t, "all", dept)
}

func TestDepartmentMembershipIsIndependentOfPlatformSubscriptions(t *testing.T) {
	id := int64(7)
	s := &DepartmentScope{Members: []DepartmentMember{
		{ID: 1, Email: "alpha@xunyou.com", Organization: OrganizationXunyou, DepartmentID: &id},
		{ID: 2, Email: "zero@xunyou.com", Organization: OrganizationXunyou, DepartmentID: &id},
		{ID: 3, Email: "unassigned@xunyou.com", Organization: OrganizationXunyou},
	}}
	require.Len(t, s.SelectedMembers(OrganizationXunyou, "7", ""), 2)
	require.Equal(t, int64(3), s.SelectedMembers(OrganizationXunyou, "unassigned", "")[0].ID)
	require.Len(t, s.SelectedMembers(OrganizationXunyou, "7", "ALPHA"), 1)
	require.Equal(t, OrganizationOther, OrganizationForEmail("dev@team.xunyou.com"))
	require.Equal(t, OrganizationWsdashi, OrganizationForEmail("DEV@WSDASHI.COM"))
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
