package config

import (
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

// The rollback documented in the rollup design and ops wiki must actually disable the rollup.
func TestOrganizationUsageRollupEnabledKey(t *testing.T) {
	for _, tc := range []struct {
		name string
		yaml string
		want bool
	}{
		{name: "default", yaml: "", want: true},
		{name: "documented rollback", yaml: "organization_usage:\n  rollup_enabled: false\n", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			viper.Reset()
			defer viper.Reset()
			setDefaults()
			viper.SetConfigType("yaml")
			require.NoError(t, viper.ReadConfig(strings.NewReader(tc.yaml)))
			var cfg Config
			require.NoError(t, viper.Unmarshal(&cfg))
			require.Equal(t, tc.want, cfg.OrganizationUsage.RollupEnabled)
		})
	}

	t.Run("environment", func(t *testing.T) {
		t.Setenv("ORGANIZATION_USAGE_ROLLUP_ENABLED", "false")
		viper.Reset()
		defer viper.Reset()
		setDefaults()
		viper.AutomaticEnv()
		viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
		var cfg Config
		require.NoError(t, viper.Unmarshal(&cfg))
		require.False(t, cfg.OrganizationUsage.RollupEnabled)
	})
}
