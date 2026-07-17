//go:build unit

package repository

import (
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUserEntityToServiceMapsAdminPermissions(t *testing.T) {
	entity := &dbent.User{
		Role:             service.RoleSubAdmin,
		AdminPermissions: []string{service.AdminPermissionUsage},
	}

	user := userEntityToService(entity)

	require.Equal(t, service.RoleSubAdmin, user.Role)
	require.Equal(t, []string{service.AdminPermissionUsage}, user.AdminPermissions)
}
