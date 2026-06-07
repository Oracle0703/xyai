//go:build unit

package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
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

func TestSetRequestArchiveSettingsPersistsCustomDir(t *testing.T) {
	repo := newMockSettingRepo()
	svc := NewSettingService(repo, &config.Config{})
	customDir := filepath.Join(t.TempDir(), "archive-on-big-disk")

	err := svc.SetRequestArchiveSettings(context.Background(), &RequestArchiveSettings{
		Enabled: true,
		Dir:     customDir,
	})
	require.NoError(t, err)

	settings, err := svc.GetRequestArchiveSettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, customDir, settings.Dir)
	require.True(t, settings.DirCustomized)
	require.Equal(t, "data/request-archive", settings.DefaultDir)
	// 运行态配置与写入端同源, 必须跟随自定义目录。
	require.Equal(t, customDir, svc.GetRequestArchiveRuntimeConfig(context.Background()).Dir)
	// 校验目录已被创建(MkdirAll + 写探针)。
	info, err := os.Stat(customDir)
	require.NoError(t, err)
	require.True(t, info.IsDir())
}

func TestSetRequestArchiveSettingsEmptyDirRestoresDefault(t *testing.T) {
	repo := newMockSettingRepo()
	svc := NewSettingService(repo, &config.Config{})
	customDir := t.TempDir()

	require.NoError(t, svc.SetRequestArchiveSettings(context.Background(), &RequestArchiveSettings{Dir: customDir}))
	require.NoError(t, svc.SetRequestArchiveSettings(context.Background(), &RequestArchiveSettings{Dir: ""}))

	settings, err := svc.GetRequestArchiveSettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, "data/request-archive", settings.Dir)
	require.False(t, settings.DirCustomized)
	require.Equal(t, "data/request-archive", svc.GetRequestArchiveRuntimeConfig(context.Background()).Dir)
}

func TestTokenAnalysisIndexerFollowsRuntimeArchiveDir(t *testing.T) {
	// config 默认目录留空目录, 后台自定义目录里放 JSONL:
	// 索引器必须跟随运行态目录(与写入端同源), 而不是 config 值。
	configDir := t.TempDir()
	customDir := t.TempDir()
	line := `{"archive_id":"rt1","event":"request","timestamp":"2026-06-07T01:02:03Z","method":"POST","endpoint":"/v1/chat/completions","user_id":7,"api_key_id":9,"model":"gpt-4.1","body":"{\"model\":\"gpt-4.1\",\"messages\":[{\"role\":\"user\",\"content\":\"hello\"}]}","body_size":64,"body_sha256":"hash-rt"}` + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(customDir, "2026-06-07.jsonl"), []byte(line), 0o600))

	settingRepo := newMockSettingRepo()
	cfg := &config.Config{
		Gateway:       config.GatewayConfig{RequestArchive: config.GatewayRequestArchiveConfig{Dir: configDir}},
		TokenAnalysis: config.TokenAnalysisConfig{IndexEnabled: true, IndexBatchSize: 1000, MaxPreviewChars: 300, UsageMatchWindowSeconds: 10},
	}
	settingSvc := NewSettingService(settingRepo, cfg)
	require.NoError(t, settingSvc.SetRequestArchiveSettings(context.Background(), &RequestArchiveSettings{Dir: customDir}))

	repo := &tokenAnalysisRepoStub{}
	svc := NewTokenAnalysisService(repo, cfg, settingSvc)

	result, err := svc.IndexRange(context.Background(), TokenAnalysisIndexRequest{StartDate: "2026-06-07", EndDate: "2026-06-07"})
	require.NoError(t, err)
	require.Equal(t, int64(1), result.IndexedRows)
	require.Len(t, repo.upserts, 1)
	require.Equal(t, "rt1", repo.upserts[0].ArchiveID)
}

func TestTokenAnalysisListArchiveFilesFollowsRuntimeDir(t *testing.T) {
	configDir := t.TempDir()
	customDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(customDir, "2026-01-01.jsonl"), []byte("x"), 0o600))

	settingRepo := newMockSettingRepo()
	cfg := &config.Config{Gateway: config.GatewayConfig{RequestArchive: config.GatewayRequestArchiveConfig{Dir: configDir}}}
	settingSvc := NewSettingService(settingRepo, cfg)
	require.NoError(t, settingSvc.SetRequestArchiveSettings(context.Background(), &RequestArchiveSettings{Dir: customDir}))

	svc := NewTokenAnalysisService(&tokenAnalysisRepoStub{}, cfg, settingSvc)

	files, err := svc.ListArchiveFiles(context.Background())
	require.NoError(t, err)
	require.Len(t, files, 1)
	require.Equal(t, "2026-01-01.jsonl", files[0].Name)
}

func TestSetRequestArchiveSettingsRejectsInvalidDir(t *testing.T) {
	repo := newMockSettingRepo()
	svc := NewSettingService(repo, &config.Config{})

	// 相对路径: 进程工作目录不可靠, 必须拒绝。
	err := svc.SetRequestArchiveSettings(context.Background(), &RequestArchiveSettings{Dir: "relative/archive"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "absolute")

	// 路径指向已存在的文件。
	file := filepath.Join(t.TempDir(), "occupied.txt")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o600))
	err = svc.SetRequestArchiveSettings(context.Background(), &RequestArchiveSettings{Dir: file})
	require.Error(t, err)
	require.Contains(t, err.Error(), "file")

	// 磁盘/卷不存在(仅 Windows 可稳定构造盘符场景)。
	if runtime.GOOS == "windows" {
		err = svc.SetRequestArchiveSettings(context.Background(), &RequestArchiveSettings{Dir: `Q:\no-such-volume\archive`})
		require.Error(t, err)
	}

	// 校验失败不得污染持久化与运行态。
	settings, getErr := svc.GetRequestArchiveSettings(context.Background())
	require.NoError(t, getErr)
	require.False(t, settings.DirCustomized)
	require.Equal(t, "data/request-archive", settings.Dir)
}
