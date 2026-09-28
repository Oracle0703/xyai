package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type adminDetailReader struct {
	UserRepository
	user *User
	err  error
}

func (r *adminDetailReader) GetByIDWithAdminAccess(context.Context, int64) (*User, error) {
	return r.user, r.err
}

func (r *adminDetailReader) GetLatestUsedAtByUserID(context.Context, int64) (*time.Time, error) {
	return nil, nil
}

func TestAdminUserDetailRequiresVersionedReader(t *testing.T) {
	user := &User{ID: 7, Role: RoleSubAdmin, AdminPermissions: []string{AdminPermissionOrganizationUsage}, AdminAccessVersion: "current-access"}
	r := &adminDetailReader{user: user}
	s := &adminServiceImpl{userRepo: r}
	actual, err := s.GetUser(context.Background(), user.ID)
	require.NoError(t, err)
	require.Equal(t, user, actual)
	r.err = errors.New("authorization snapshot failed")
	_, err = s.GetUser(context.Background(), user.ID)
	require.ErrorIs(t, err, r.err, "do not fall back to an unversioned detail or a second snapshot")
}
