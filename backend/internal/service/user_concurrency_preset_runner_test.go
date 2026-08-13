package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestUserConcurrencyPresetRunnerStartAndStopAreIdempotent(t *testing.T) {
	svc := NewUserConcurrencyPresetService(nil, nil, nil, &config.Config{})
	runner := ProvideUserConcurrencyPresetRunner(svc)

	firstCron := runner.cron
	runner.Start()

	require.NotNil(t, firstCron)
	require.Same(t, firstCron, runner.cron)
	require.Len(t, runner.cron.Entries(), 1)
	require.NotPanics(t, func() {
		runner.Stop()
		runner.Stop()
	})
}

func TestUserConcurrencyPresetRunnerWithoutServiceDoesNotStart(t *testing.T) {
	runner := NewUserConcurrencyPresetRunner(nil)

	runner.Start()

	require.Nil(t, runner.cron)
}
