//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeGroupOrganizationScopeInput(t *testing.T) {
	keys := func(values ...string) *[]string { return &values }
	ids, organizations, err := normalizeGroupOrganizationScopeInput(GroupOrganizationScopeInput{GroupIDs: []int64{3, 1, 3}, OrganizationKeys: keys(" wsdashi", "xunyou", "wsdashi")})
	require.NoError(t, err)
	require.Equal(t, []int64{1, 3}, ids)
	require.Equal(t, []string{"wsdashi", "xunyou"}, organizations)
	_, organizations, err = normalizeGroupOrganizationScopeInput(GroupOrganizationScopeInput{GroupIDs: []int64{1}, OrganizationKeys: keys()})
	require.NoError(t, err)
	require.Empty(t, organizations, "[] clears the scope")

	tooMany := make([]int64, MaxGroupOrganizationScopeBatch+1)
	for i := range tooMany {
		tooMany[i] = int64(i + 1)
	}
	for name, input := range map[string]GroupOrganizationScopeInput{
		"nil organizations": {GroupIDs: []int64{1}},
		"other":             {GroupIDs: []int64{1}, OrganizationKeys: keys(OrganizationOther)},
		"unknown":           {GroupIDs: []int64{1}, OrganizationKeys: keys("acme")},
		"no groups":         {OrganizationKeys: keys("xunyou")},
		"non-positive id":   {GroupIDs: []int64{-1}, OrganizationKeys: keys("xunyou")},
		"too many groups":   {GroupIDs: tooMany, OrganizationKeys: keys("xunyou")},
	} {
		_, _, err = normalizeGroupOrganizationScopeInput(input)
		require.ErrorIs(t, err, ErrGroupOrganizationScopeInvalid, name)
	}
}

func TestSubscriptionWriteAccessNeedsOwnerAndGroup(t *testing.T) {
	access := &SubscriptionWriteAccess{Users: map[int64]bool{1: true}, Groups: map[int64]bool{7: true}}
	require.True(t, access.Allows(1, 7))
	require.False(t, access.Allows(2, 7))
	require.False(t, access.Allows(1, 8))
	require.False(t, (*SubscriptionWriteAccess)(nil).Allows(1, 7))
}

func TestGroupOrganizationScopesRejectSubAdmins(t *testing.T) {
	svc := &SubscriptionService{}
	ctx := WithSubAdminAccountRestrictions(context.Background())
	_, err := svc.ListGroupOrganizationScopes(ctx)
	require.ErrorIs(t, err, ErrDepartmentScopeDenied)
	_, err = svc.SetGroupOrganizationScopes(ctx, GroupOrganizationScopeInput{GroupIDs: []int64{1}, OrganizationKeys: &[]string{}})
	require.ErrorIs(t, err, ErrDepartmentScopeDenied)
}
