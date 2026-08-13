package promptmetrics

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestNewExtensionStartsPublisherOnlyWhenEnabled(t *testing.T) {
	disabled := NewExtension(&config.Config{PromptMetrics: config.PromptMetricsConfig{Enabled: false}}, nil)
	require.NotNil(t, disabled)
	require.Nil(t, disabled.publisher)
	disabled.Stop(time.Second)

	enabled := NewExtension(&config.Config{PromptMetrics: testPromptMetricsConfig()}, nil)
	require.NotNil(t, enabled.publisher)
	require.False(t, enabled.publisher.pool.Stopped())
	require.NotPanics(t, func() {
		enabled.Stop(time.Second)
		enabled.Stop(time.Second)
	})
	require.True(t, enabled.publisher.pool.Stopped())
}
