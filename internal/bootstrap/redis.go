package bootstrap

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"

	"github.com/jeffreyzhai123/fluffy-dollop/internal/config"
	"github.com/jeffreyzhai123/fluffy-dollop/internal/observability"
)

// SetupRedis creates and verifies a Redis client connection
func SetupRedis(ctx context.Context, cfg *config.Config, logger *observability.Logger) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         cfg.RedisAddr(),
		Password:     cfg.Redis.Password,
		DB:           cfg.Redis.DB,
		PoolSize:     cfg.Redis.PoolSize,
		MinIdleConns: cfg.Redis.MinIdleConns,
	})

	// Verify connection
	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		return nil, fmt.Errorf("redis connection failed: %w", err)
	}

	logger.Info("redis connected successfully",
		"addr", cfg.RedisAddr(),
		"db", cfg.Redis.DB,
		"pool_size", cfg.Redis.PoolSize,
	)

	return client, nil
}
