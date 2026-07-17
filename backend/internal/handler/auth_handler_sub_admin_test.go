//go:build unit

package handler

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestCanRefreshInBackendMode(t *testing.T) {
	t.Run("allows sub admin with at least one current permission", func(t *testing.T) {
		result := &service.TokenPairWithUser{
			UserRole:         service.RoleSubAdmin,
			AdminPermissions: []string{service.AdminPermissionUsage},
		}

		require.True(t, canRefreshInBackendMode(result))
	})

	t.Run("denies sub admin after all permissions are revoked", func(t *testing.T) {
		result := &service.TokenPairWithUser{UserRole: service.RoleSubAdmin}

		require.False(t, canRefreshInBackendMode(result))
	})

	t.Run("keeps full admin access", func(t *testing.T) {
		result := &service.TokenPairWithUser{UserRole: service.RoleAdmin}

		require.True(t, canRefreshInBackendMode(result))
	})
}
