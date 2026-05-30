//go:build unit

package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestGetRequestArchiveSettingsDefaultsFromConfig(t *testing.T) {
	repo := newMockSettingRepo()
	svc := NewSettingService(repo, &config.Config{
		Gateway: config.GatewayConfig{
			RequestArchive: config.GatewayRequestArchiveConfig{
				Enabled:              true,
				CaptureResponse:      true,
				Dir:                  "data/custom-archive",
				MaxRequestBodyBytes:  123,
				MaxResponseBodyBytes: 456,
				QueueSize:            7,
			},
		},
	})

	settings, err := svc.GetRequestArchiveSettings(context.Background())
	require.NoError(t, err)
	require.True(t, settings.Enabled)
	require.True(t, settings.CaptureResponse)
	require.Equal(t, "data/custom-archive", settings.Dir)
	require.Equal(t, int64(123), settings.MaxRequestBodyBytes)
	require.Equal(t, int64(456), settings.MaxResponseBodyBytes)
	require.Equal(t, 7, settings.QueueSize)
}

func TestGetRequestArchiveSettingsUsesPersistedSwitchesOnly(t *testing.T) {
	repo := newMockSettingRepo()
	stored, err := json.Marshal(persistedRequestArchiveSettings{Enabled: true, CaptureResponse: true})
	require.NoError(t, err)
	repo.data[SettingKeyRequestArchiveSettings] = string(stored)
	svc := NewSettingService(repo, &config.Config{
		Gateway: config.GatewayConfig{
			RequestArchive: config.GatewayRequestArchiveConfig{
				Enabled:              false,
				CaptureResponse:      false,
				Dir:                  "data/from-config",
				MaxRequestBodyBytes:  1024,
				MaxResponseBodyBytes: 2048,
				QueueSize:            33,
			},
		},
	})

	settings, err := svc.GetRequestArchiveSettings(context.Background())
	require.NoError(t, err)
	require.True(t, settings.Enabled)
	require.True(t, settings.CaptureResponse)
	require.Equal(t, "data/from-config", settings.Dir)
	require.Equal(t, int64(1024), settings.MaxRequestBodyBytes)
	require.Equal(t, int64(2048), settings.MaxResponseBodyBytes)
	require.Equal(t, 33, settings.QueueSize)
}

func TestSetRequestArchiveSettingsUpdatesRuntimeCache(t *testing.T) {
	repo := newMockSettingRepo()
	svc := NewSettingService(repo, &config.Config{})

	err := svc.SetRequestArchiveSettings(context.Background(), &RequestArchiveSettings{
		Enabled:         true,
		CaptureResponse: true,
	})
	require.NoError(t, err)

	runtimeCfg := svc.GetRequestArchiveRuntimeConfig(context.Background())
	require.True(t, runtimeCfg.Enabled)
	require.True(t, runtimeCfg.CaptureResponse)
	require.Equal(t, "data/request-archive", runtimeCfg.Dir)
	require.Equal(t, 1024, runtimeCfg.QueueSize)
}
