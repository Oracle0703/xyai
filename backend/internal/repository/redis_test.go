package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestBuildRedisOptions(t *testing.T) {
	cfg := &config.Config{
		Redis: config.RedisConfig{
			Host:                "localhost",
			Port:                6379,
			Username:            "app-user",
			Password:            "secret",
			DB:                  2,
			DialTimeoutSeconds:  5,
			ReadTimeoutSeconds:  3,
			WriteTimeoutSeconds: 4,
			PoolSize:            100,
			MinIdleConns:        10,
		},
	}

	opts := buildRedisOptions(cfg)
	require.Equal(t, "localhost:6379", opts.Addr)
	require.Equal(t, "app-user", opts.Username)
	require.Equal(t, "secret", opts.Password)
	require.Equal(t, 2, opts.DB)
	require.Equal(t, 5*time.Second, opts.DialTimeout)
	require.Equal(t, 3*time.Second, opts.ReadTimeout)
	require.Equal(t, 4*time.Second, opts.WriteTimeout)
	require.Equal(t, 100, opts.PoolSize)
	require.Equal(t, 10, opts.MinIdleConns)
	require.Nil(t, opts.TLSConfig)

	// Test case with TLS enabled
	cfgTLS := &config.Config{
		Redis: config.RedisConfig{
			Host:      "localhost",
			EnableTLS: true,
		},
	}
	optsTLS := buildRedisOptions(cfgTLS)
	require.NotNil(t, optsTLS.TLSConfig)
	require.Equal(t, "localhost", optsTLS.TLSConfig.ServerName)
}

func TestValidateRedisServerVersionRejectsRedis3(t *testing.T) {
	err := validateRedisServerVersion(context.Background(), redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"}), "3.0.504")
	require.Error(t, err)
	require.Contains(t, err.Error(), "Redis 7+ is required")
	require.Contains(t, err.Error(), "3.0.504")
}

func TestValidateRedisServerVersionAllowsRedis7(t *testing.T) {
	err := validateRedisServerVersion(context.Background(), redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"}), "7.2.4")
	require.NoError(t, err)
}

func TestValidateRedisServerVersionAllowsMemuraiRedisCompatibility(t *testing.T) {
	err := validateRedisServerVersion(context.Background(), redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"}), "7.2.0-memurai")
	require.NoError(t, err)
}

func TestValidateRedisServerVersionAllowsMemuraiInfo(t *testing.T) {
	err := validateRedisServerInfo(context.Background(), redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"}), "memurai_version:4.2.2\r\n")
	require.NoError(t, err)
}
