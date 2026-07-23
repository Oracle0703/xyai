package repository

import (
	"context"
	"crypto/tls"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"

	"github.com/redis/go-redis/v9"
)

// InitRedis 初始化 Redis 客户端
//
// 性能优化说明：
// 原实现使用 go-redis 默认配置，未设置连接池和超时参数：
// 1. 默认连接池大小可能不足以支撑高并发
// 2. 无超时控制可能导致慢操作阻塞
//
// 新实现支持可配置的连接池和超时参数：
// 1. PoolSize: 控制最大并发连接数（默认 128）
// 2. MinIdleConns: 保持最小空闲连接，减少冷启动延迟（默认 10）
// 3. DialTimeout/ReadTimeout/WriteTimeout: 精确控制各阶段超时
func InitRedis(cfg *config.Config) (*redis.Client, error) {
	rdb := redis.NewClient(buildRedisOptions(cfg))
	if cfg.Server.EnableServerTiming {
		rdb.AddHook(serverTimingRedisHook{})
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.Redis.DialTimeoutSeconds)*time.Second)
	defer cancel()
	if err := validateRedisServerVersion(ctx, rdb, ""); err != nil {
		_ = rdb.Close()
		return nil, err
	}
	return rdb, nil
}

// buildRedisOptions 构建 Redis 连接选项
// 从配置文件读取连接池和超时参数，支持生产环境调优
func buildRedisOptions(cfg *config.Config) *redis.Options {
	opts := &redis.Options{
		Addr:         cfg.Redis.Address(),
		Username:     cfg.Redis.Username,
		Password:     cfg.Redis.Password,
		DB:           cfg.Redis.DB,
		DialTimeout:  time.Duration(cfg.Redis.DialTimeoutSeconds) * time.Second,  // 建连超时
		ReadTimeout:  time.Duration(cfg.Redis.ReadTimeoutSeconds) * time.Second,  // 读取超时
		WriteTimeout: time.Duration(cfg.Redis.WriteTimeoutSeconds) * time.Second, // 写入超时
		PoolSize:     cfg.Redis.PoolSize,                                         // 连接池大小
		MinIdleConns: cfg.Redis.MinIdleConns,                                     // 最小空闲连接
	}

	if cfg.Redis.EnableTLS {
		opts.TLSConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
			ServerName: cfg.Redis.Host,
		}
	}

	return opts
}

func validateRedisServerVersion(ctx context.Context, rdb *redis.Client, versionOverride string) error {
	version := strings.TrimSpace(versionOverride)
	if version == "" {
		info, err := rdb.Info(ctx, "server").Result()
		if err != nil {
			return fmt.Errorf("check Redis server version: %w", err)
		}
		return validateRedisServerInfo(ctx, rdb, info)
	}
	if version == "" {
		return fmt.Errorf("check Redis server version: missing redis_version in INFO server")
	}
	if redisMajorVersion(version) < 7 {
		return fmt.Errorf("Redis 7+ is required, current Redis version is %s", version)
	}
	return nil
}

func validateRedisServerInfo(_ context.Context, _ *redis.Client, info string) error {
	version := redisInfoValue(info, "redis_version")
	if version != "" {
		if redisMajorVersion(version) < 7 {
			return fmt.Errorf("Redis 7+ is required, current Redis version is %s", version)
		}
		return nil
	}
	memuraiVersion := redisInfoValue(info, "memurai_version")
	if memuraiVersion != "" {
		return nil
	}
	return fmt.Errorf("check Redis server version: missing redis_version in INFO server")
}

func redisVersionFromInfo(info string) string {
	return redisInfoValue(info, "redis_version")
}

func redisInfoValue(info, name string) string {
	for _, line := range strings.Split(info, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, name+":") {
			return strings.TrimSpace(strings.TrimPrefix(line, name+":"))
		}
	}
	return ""
}

func redisMajorVersion(version string) int {
	version = strings.TrimSpace(version)
	if version == "" {
		return 0
	}
	for i, r := range version {
		if r < '0' || r > '9' {
			if i == 0 {
				return 0
			}
			version = version[:i]
			break
		}
	}
	major, err := strconv.Atoi(version)
	if err != nil {
		return 0
	}
	return major
}
