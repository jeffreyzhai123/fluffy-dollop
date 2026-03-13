package health

import (
	"context"
	"fmt"

	"github.com/jeffreyzhai123/fluffy-dollop/internal/observability"
	"github.com/redis/go-redis/v9"
)

type RedisChecker struct {
	client *redis.Client
	logger *observability.Logger
}

func NewRedisChecker(client *redis.Client, logger *observability.Logger) *RedisChecker {
	return &RedisChecker{
		client: client,
		logger: logger,
	}
}

func (r *RedisChecker) Check(ctx context.Context) error {
	err := r.client.Ping(ctx).Err()
	if err != nil {
		return fmt.Errorf("redis ping failed: %w", err)
	}
	return nil
}

func (r *RedisChecker) Name() string {
	return "redis"
}
