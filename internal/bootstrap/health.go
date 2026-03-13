package bootstrap

import (
	"github.com/redis/go-redis/v9"

	"github.com/jeffreyzhai123/fluffy-dollop/internal/config"
	"github.com/jeffreyzhai123/fluffy-dollop/internal/health"
	"github.com/jeffreyzhai123/fluffy-dollop/internal/observability"
)

// SetupHealthChecks creates and registers common health checks
func SetupHealthChecks(redisClient *redis.Client, cfg *config.Config, logger *observability.Logger) *health.Aggregator {
	healthAgg := health.NewAggregator(cfg.Observability.HealthCheckTimeout, logger)
	healthAgg.Register(health.NewRedisChecker(redisClient, logger))

	// TODO: Add postgres when ready
	// if pgPool != nil {
	//     healthAgg.Register(health.NewPostgresChecker(pgPool, logger))
	// }

	logger.Debug("health checks registered")

	return healthAgg
}
