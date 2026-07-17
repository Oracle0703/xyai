package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSubAdminPermissionsMigration(t *testing.T) {
	content, err := FS.ReadFile("177_add_sub_admin_permissions.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS admin_permissions JSONB NOT NULL DEFAULT '[]'::jsonb")
	require.NotContains(t, sql, "UPDATE users SET role")
}
