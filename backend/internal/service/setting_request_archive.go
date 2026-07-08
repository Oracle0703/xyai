package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

type cachedRequestArchiveRuntimeConfig struct {
	cfg       config.GatewayRequestArchiveConfig
	expiresAt int64
}

const requestArchiveRuntimeCacheTTL = 5 * time.Second
const requestArchiveRuntimeErrorTTL = time.Second
const requestArchiveRuntimeDBTimeout = 2 * time.Second
const requestArchiveRuntimeCacheKey = "request_archive_runtime_config"

// 后台自定义请求体截断上限的合法区间: 过小会让所有归档行都被截断,
// 过大则单文件膨胀失控。
const minRequestArchiveBodyBytes = int64(64 * 1024)
const maxRequestArchiveBodyBytes = int64(512 * 1024 * 1024)

func (s *SettingService) defaultRequestArchiveSettings() *RequestArchiveSettings {
	cfg := config.GatewayRequestArchiveConfig{}
	if s != nil && s.cfg != nil {
		cfg = s.cfg.Gateway.RequestArchive
	}
	if strings.TrimSpace(cfg.Dir) == "" {
		cfg.Dir = "data/request-archive"
	}
	if cfg.MaxRequestBodyBytes <= 0 {
		cfg.MaxRequestBodyBytes = int64(64 * 1024)
	}
	if cfg.MaxResponseBodyBytes <= 0 {
		cfg.MaxResponseBodyBytes = int64(64 * 1024)
	}
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = 1024
	}
	return &RequestArchiveSettings{
		Enabled:              cfg.Enabled,
		CaptureResponse:      cfg.CaptureResponse,
		Dir:                  cfg.Dir,
		DefaultDir:           cfg.Dir,
		MaxRequestBodyBytes:  cfg.MaxRequestBodyBytes,
		MaxResponseBodyBytes: cfg.MaxResponseBodyBytes,
		QueueSize:            cfg.QueueSize,
	}
}

func (s *SettingService) getRequestArchiveSettingsUncached(ctx context.Context) (*RequestArchiveSettings, error) {
	if s == nil || s.settingRepo == nil {
		return nil, fmt.Errorf("setting service is not configured")
	}
	settings := s.defaultRequestArchiveSettings()
	value, err := s.settingRepo.GetValue(ctx, SettingKeyRequestArchiveSettings)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return settings, nil
		}
		return nil, fmt.Errorf("get request archive settings: %w", err)
	}
	if strings.TrimSpace(value) == "" {
		return settings, nil
	}
	var stored persistedRequestArchiveSettings
	if err := json.Unmarshal([]byte(value), &stored); err != nil {
		slog.Warn("request archive settings unmarshal failed, falling back to config defaults", "error", err)
		return settings, nil
	}
	settings.Enabled = stored.Enabled
	settings.CaptureResponse = stored.CaptureResponse
	if dir := strings.TrimSpace(stored.Dir); dir != "" {
		settings.Dir = dir
		settings.DirCustomized = true
	}
	if stored.MaxRequestBodyBytes > 0 {
		settings.MaxRequestBodyBytes = stored.MaxRequestBodyBytes
	}
	return settings, nil
}

func (s *SettingService) requestArchiveSettingsToConfig(settings *RequestArchiveSettings) config.GatewayRequestArchiveConfig {
	if settings == nil {
		settings = s.defaultRequestArchiveSettings()
	}
	return config.GatewayRequestArchiveConfig{
		Enabled:              settings.Enabled,
		Dir:                  settings.Dir,
		MaxRequestBodyBytes:  settings.MaxRequestBodyBytes,
		CaptureResponse:      settings.CaptureResponse,
		MaxResponseBodyBytes: settings.MaxResponseBodyBytes,
		QueueSize:            settings.QueueSize,
	}
}

func (s *SettingService) storeRequestArchiveRuntimeConfig(settings *RequestArchiveSettings, ttl time.Duration) config.GatewayRequestArchiveConfig {
	cfg := s.requestArchiveSettingsToConfig(settings)
	s.requestArchiveRuntimeCache.Store(&cachedRequestArchiveRuntimeConfig{
		cfg:       cfg,
		expiresAt: time.Now().Add(ttl).UnixNano(),
	})
	return cfg
}

func (s *SettingService) GetRequestArchiveSettings(ctx context.Context) (*RequestArchiveSettings, error) {
	settings, err := s.getRequestArchiveSettingsUncached(ctx)
	if err != nil {
		return nil, err
	}
	s.storeRequestArchiveRuntimeConfig(settings, requestArchiveRuntimeCacheTTL)
	return settings, nil
}

func (s *SettingService) SetRequestArchiveSettings(ctx context.Context, settings *RequestArchiveSettings) error {
	if s == nil || s.settingRepo == nil {
		return fmt.Errorf("setting service is not configured")
	}
	if settings == nil {
		return fmt.Errorf("settings cannot be nil")
	}
	customDir := strings.TrimSpace(settings.Dir)
	if customDir != "" {
		customDir = filepath.Clean(customDir)
		if requestArchiveDirEquals(customDir, s.defaultRequestArchiveSettings().DefaultDir) {
			customDir = ""
		}
	}
	if customDir != "" {
		if err := ValidateRequestArchiveDir(customDir); err != nil {
			return err
		}
	}
	customMaxRequestBodyBytes := settings.MaxRequestBodyBytes
	if customMaxRequestBodyBytes <= 0 || customMaxRequestBodyBytes == s.defaultRequestArchiveSettings().MaxRequestBodyBytes {
		customMaxRequestBodyBytes = 0
	}
	if customMaxRequestBodyBytes != 0 && (customMaxRequestBodyBytes < minRequestArchiveBodyBytes || customMaxRequestBodyBytes > maxRequestArchiveBodyBytes) {
		return infraerrors.BadRequest("REQUEST_ARCHIVE_MAX_REQUEST_BODY_INVALID",
			fmt.Sprintf("max request body bytes must be between %d and %d, got %d", minRequestArchiveBodyBytes, maxRequestArchiveBodyBytes, customMaxRequestBodyBytes))
	}
	stored := persistedRequestArchiveSettings{
		Enabled:             settings.Enabled,
		CaptureResponse:     settings.CaptureResponse,
		Dir:                 customDir,
		MaxRequestBodyBytes: customMaxRequestBodyBytes,
	}
	data, err := json.Marshal(stored)
	if err != nil {
		return fmt.Errorf("marshal request archive settings: %w", err)
	}
	if err := s.settingRepo.Set(ctx, SettingKeyRequestArchiveSettings, string(data)); err != nil {
		return fmt.Errorf("save request archive settings: %w", err)
	}
	merged := s.defaultRequestArchiveSettings()
	merged.Enabled = settings.Enabled
	merged.CaptureResponse = settings.CaptureResponse
	if customDir != "" {
		merged.Dir = customDir
		merged.DirCustomized = true
	}
	if customMaxRequestBodyBytes > 0 {
		merged.MaxRequestBodyBytes = customMaxRequestBodyBytes
	}
	s.storeRequestArchiveRuntimeConfig(merged, requestArchiveRuntimeCacheTTL)
	return nil
}

func (s *SettingService) GetRequestArchiveRuntimeConfig(ctx context.Context) config.GatewayRequestArchiveConfig {
	if s == nil {
		return config.GatewayRequestArchiveConfig{}
	}
	if s.settingRepo == nil {
		return s.requestArchiveSettingsToConfig(s.defaultRequestArchiveSettings())
	}
	now := time.Now().UnixNano()
	if cached, ok := s.requestArchiveRuntimeCache.Load().(*cachedRequestArchiveRuntimeConfig); ok && cached != nil && now < cached.expiresAt {
		return cached.cfg
	}
	result, err, _ := s.requestArchiveRuntimeSF.Do(requestArchiveRuntimeCacheKey, func() (any, error) {
		if cached, ok := s.requestArchiveRuntimeCache.Load().(*cachedRequestArchiveRuntimeConfig); ok && cached != nil && time.Now().UnixNano() < cached.expiresAt {
			return cached.cfg, nil
		}
		dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), requestArchiveRuntimeDBTimeout)
		defer cancel()
		settings, loadErr := s.getRequestArchiveSettingsUncached(dbCtx)
		if loadErr != nil {
			if cached, ok := s.requestArchiveRuntimeCache.Load().(*cachedRequestArchiveRuntimeConfig); ok && cached != nil {
				s.requestArchiveRuntimeCache.Store(&cachedRequestArchiveRuntimeConfig{
					cfg:       cached.cfg,
					expiresAt: time.Now().Add(requestArchiveRuntimeErrorTTL).UnixNano(),
				})
				return cached.cfg, nil
			}
			settings = s.defaultRequestArchiveSettings()
			s.storeRequestArchiveRuntimeConfig(settings, requestArchiveRuntimeErrorTTL)
			return s.requestArchiveSettingsToConfig(settings), nil
		}
		return s.storeRequestArchiveRuntimeConfig(settings, requestArchiveRuntimeCacheTTL), nil
	})
	if err != nil {
		return s.requestArchiveSettingsToConfig(s.defaultRequestArchiveSettings())
	}
	if cfg, ok := result.(config.GatewayRequestArchiveConfig); ok {
		return cfg
	}
	return s.requestArchiveSettingsToConfig(s.defaultRequestArchiveSettings())
}
