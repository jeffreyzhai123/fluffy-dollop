package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Load reads configuration from environment variables with defaults
func Load() (*Config, error) {
	// Start with defaults
	cfg := GetDefaults()

	// Override with environment variables
	cfg.Environment = GetEnv("ENVIRONMENT", cfg.Environment)
	loadIngestionConfig(&cfg.Ingestion)
	loadWorkerConfig(&cfg.Worker)
	loadServerConfig(&cfg.Server)
	loadRedisConfig(&cfg.Redis)
	loadQueueConfig(&cfg.Queue)
	loadPostgresConfig(&cfg.Postgres)
	loadObservabilityConfig(&cfg.Observability)
	loadRegistryConfig(&cfg.Registry)
	loadThrottlerConfig(&cfg.Throttler)

	// Validate the final configuration
	if err := Validate(cfg); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

func loadIngestionConfig(cfg *IngestionConfig) {
	cfg.EnableCircuitBreaker = GetBoolEnv("INGESTION_ENABLE_CIRCUIT_BREAKER", cfg.EnableCircuitBreaker)
	cfg.MaxQueueDepth = GetInt64Env("INGESTION_MAX_QUEUE_DEPTH", cfg.MaxQueueDepth)

	// Throttler settings
	cfg.Throttler.Enabled = GetBoolEnv("INGESTION_THROTTLER_ENABLED", cfg.Throttler.Enabled)
	cfg.Throttler.SoftDepthLimit = GetInt64Env("INGESTION_THROTTLER_SOFT_DEPTH", cfg.Throttler.SoftDepthLimit)
	cfg.Throttler.HardDepthLimit = GetInt64Env("INGESTION_THROTTLER_HARD_DEPTH", cfg.Throttler.HardDepthLimit)
	cfg.Throttler.MaxMessageAge = GetDurationEnv("INGESTION_THROTTLER_MAX_AGE", cfg.Throttler.MaxMessageAge)
	cfg.Throttler.CriticalMessageAge = GetDurationEnv("INGESTION_THROTTLER_CRITICAL_AGE", cfg.Throttler.CriticalMessageAge)
	cfg.Throttler.RateLimitScore = GetFloat64Env("INGESTION_THROTTLER_RATE_LIMIT_SCORE", cfg.Throttler.RateLimitScore)
	cfg.Throttler.CircuitOpenScore = GetFloat64Env("INGESTION_THROTTLER_CIRCUIT_SCORE", cfg.Throttler.CircuitOpenScore)
	cfg.Throttler.MetricsUpdateInterval = GetDurationEnv("INGESTION_THROTTLER_UPDATE_INTERVAL", cfg.Throttler.MetricsUpdateInterval)
	cfg.Throttler.EstimatedDrainRate = GetFloat64Env("INGESTION_THROTTLER_DRAIN_RATE", cfg.Throttler.EstimatedDrainRate)
	cfg.Throttler.MinRetryAfter = GetDurationEnv("INGESTION_THROTTLER_MIN_RETRY", cfg.Throttler.MinRetryAfter)
	cfg.Throttler.MaxRetryAfter = GetDurationEnv("INGESTION_THROTTLER_MAX_RETRY", cfg.Throttler.MaxRetryAfter)
}

func loadWorkerConfig(cfg *WorkerConfig) {
	cfg.Concurrency = GetIntEnv("WORKER_CONCURRENCY", cfg.Concurrency)
	cfg.PollInterval = GetDurationEnv("WORKER_POLL_INTERVAL", cfg.PollInterval)
	cfg.BatchSize = GetIntEnv("WORKER_BATCH_SIZE", cfg.BatchSize)
	cfg.ProcessingTimeout = GetDurationEnv("WORKER_PROCESSING_TIMEOUT", cfg.ProcessingTimeout)
	cfg.JobMaxRetries = GetIntEnv("WORKER_MAX_RETRIES", cfg.JobMaxRetries)
	cfg.BackoffMinDelay = GetDurationEnv("WORKER_BACKOFF_MIN_DELAY", cfg.BackoffMinDelay)
	cfg.BackoffMaxDelay = GetDurationEnv("WORKER_BACKOFF_MAX_DELAY", cfg.BackoffMaxDelay)
	cfg.HealthPort = GetEnv("WORKER_HEALTH_PORT", cfg.HealthPort)
	cfg.ReclaimInterval = GetDurationEnv("WORKER_RECLAIM_INTERVAL", cfg.ReclaimInterval)
	cfg.ReclaimIdleTime = GetDurationEnv("WORKER_RECLAIM_IDLE_TIME", cfg.ReclaimIdleTime)
	cfg.ReclaimBatchSize = GetIntEnv("WORKER_RECLAIM_BATCH_SIZE", cfg.ReclaimBatchSize)
	cfg.DedupTTL = GetDurationEnv("WORKER_DEDUP_TTL", cfg.DedupTTL)
}

func loadServerConfig(cfg *ServerConfig) {
	cfg.Host = GetEnv("SERVER_HOST", cfg.Host)
	cfg.Port = GetEnv("SERVER_PORT", cfg.Port)
	cfg.ReadTimeout = GetDurationEnv("SERVER_READ_TIMEOUT", cfg.ReadTimeout)
	cfg.WriteTimeout = GetDurationEnv("SERVER_WRITE_TIMEOUT", cfg.WriteTimeout)
	cfg.IdleTimeout = GetDurationEnv("SERVER_IDLE_TIMEOUT", cfg.IdleTimeout)
	cfg.ShutdownTimeout = GetDurationEnv("SERVER_SHUTDOWN_TIMEOUT", cfg.ShutdownTimeout)
}

func loadRedisConfig(cfg *RedisClientConfig) {
	cfg.Host = GetEnv("REDIS_HOST", cfg.Host)
	cfg.Port = GetEnv("REDIS_PORT", cfg.Port)
	cfg.Password = GetEnv("REDIS_PASSWORD", cfg.Password)
	cfg.DB = GetIntEnv("REDIS_DB", cfg.DB)
	cfg.PoolSize = GetIntEnv("REDIS_POOL_SIZE", cfg.PoolSize)
	cfg.MinIdleConns = GetIntEnv("REDIS_MIN_IDLE_CONNS", cfg.MinIdleConns)
	cfg.ConnMaxIdleTime = GetDurationEnv("REDIS_CONN_MAX_IDLE_TIME", cfg.ConnMaxIdleTime)
}

func loadQueueConfig(cfg *QueueConfig) {
	cfg.BaseStreamName = GetEnv("QUEUE_STREAM_NAME", cfg.BaseStreamName)
	cfg.BaseConsumerGroup = GetEnv("QUEUE_CONSUMER_GROUP", cfg.BaseConsumerGroup)
	cfg.RedisMaxRetries = GetIntEnv("QUEUE_REDIS_MAX_RETRIES", cfg.RedisMaxRetries)
	cfg.MaxLen = GetInt64Env("QUEUE_MAX_LEN", cfg.MaxLen)
	cfg.DLQStreamName = GetEnv("QUEUE_DLQ_STREAM_NAME", cfg.DLQStreamName)
	cfg.DLQMaxLen = GetInt64Env("QUEUE_DLQ_MAX_LEN", cfg.DLQMaxLen)
	cfg.PartitionCount = GetIntEnv("QUEUE_PARTITION_COUNT", cfg.PartitionCount)
	cfg.PartitionID = GetIntEnv("QUEUE_PARTITION_ID", cfg.PartitionID)
}

func loadPostgresConfig(cfg *PostgresClientConfig) {
	cfg.Host = GetEnv("POSTGRES_HOST", cfg.Host)
	cfg.Port = GetEnv("POSTGRES_PORT", cfg.Port)
	cfg.Database = GetEnv("POSTGRES_DB", cfg.Database)
	cfg.User = GetEnv("POSTGRES_USER", cfg.User)
	cfg.Password = GetEnv("POSTGRES_PASSWORD", cfg.Password)
	cfg.SSLMode = GetEnv("POSTGRES_SSLMODE", cfg.SSLMode)
	cfg.MaxConns = GetIntEnv("POSTGRES_MAX_CONNS", cfg.MaxConns)
	cfg.MinConns = GetIntEnv("POSTGRES_MIN_CONNS", cfg.MinConns)
	cfg.MaxConnLifetime = GetDurationEnv("POSTGRES_MAX_CONN_LIFETIME", cfg.MaxConnLifetime)
	cfg.MaxConnIdleTime = GetDurationEnv("POSTGRES_MAX_CONN_IDLE_TIME", cfg.MaxConnIdleTime)
}

func loadObservabilityConfig(cfg *ObservabilityConfig) {
	cfg.LogLevel = GetEnv("LOG_LEVEL", cfg.LogLevel)
	cfg.LogFormat = GetEnv("LOG_FORMAT", cfg.LogFormat)
	cfg.MetricsEnabled = GetBoolEnv("METRICS_ENABLED", cfg.MetricsEnabled)
	cfg.MetricsPort = GetEnv("METRICS_PORT", cfg.MetricsPort)
	cfg.TracingEnabled = GetBoolEnv("TRACING_ENABLED", cfg.TracingEnabled)
	cfg.TracingSampleRate = GetFloat64Env("TRACING_SAMPLE_RATE", cfg.TracingSampleRate)
	cfg.HealthCheckTimeout = GetDurationEnv("HEALTH_CHECK_TIMEOUT", cfg.HealthCheckTimeout)
}

func loadThrottlerConfig(cfg *ThrottlerConfig) {
	cfg.Enabled = GetBoolEnv("THROTTLER_ENABLED", cfg.Enabled)
	cfg.SoftDepthLimit = GetInt64Env("THROTTLER_SOFT_DEPTH_LIMIT", cfg.SoftDepthLimit)
	cfg.HardDepthLimit = GetInt64Env("THROTTLER_HARD_DEPTH_LIMIT", cfg.HardDepthLimit)
	cfg.MaxMessageAge = GetDurationEnv("THROTTLER_MAX_MESSAGE_AGE", cfg.MaxMessageAge)
	cfg.CriticalMessageAge = GetDurationEnv("THROTTLER_CRITICAL_MESSAGE_AGE", cfg.CriticalMessageAge)
	cfg.RateLimitScore = GetFloat64Env("THROTTLER_RATE_LIMIT_SCORE", cfg.RateLimitScore)
	cfg.CircuitOpenScore = GetFloat64Env("THROTTLER_CIRCUIT_OPEN_SCORE", cfg.CircuitOpenScore)
	cfg.MetricsUpdateInterval = GetDurationEnv("THROTTLER_METRICS_UPDATE_INTERVAL", cfg.MetricsUpdateInterval)
	cfg.EstimatedDrainRate = GetFloat64Env("THROTTLER_ESTIMATED_DRAIN_RATE", cfg.EstimatedDrainRate)
	cfg.MinRetryAfter = GetDurationEnv("THROTTLER_MIN_RETRY_AFTER", cfg.MinRetryAfter)
	cfg.MaxRetryAfter = GetDurationEnv("THROTTLER_MAX_RETRY_AFTER", cfg.MaxRetryAfter)
}

func loadRegistryConfig(cfg *RegistryConfig) {
	cfg.HeartbeatInterval = GetDurationEnv("REGISTRY_HEARTBEAT_INTERVAL", cfg.HeartbeatInterval)
	cfg.DeadThreshold = GetDurationEnv("REGISTRY_DEAD_THRESHOLD", cfg.DeadThreshold)
	cfg.PollInterval = GetDurationEnv("REGISTRY_POLL_INTERVAL", cfg.PollInterval)
	cfg.ControlPlanePoll = GetDurationEnv("REGISTRY_CONTROL_PLANE_POLL", cfg.ControlPlanePoll)
	cfg.LockTTL = GetDurationEnv("REGISTRY_LOCK_TTL", cfg.LockTTL)
}

// Helper functions for reading environment variables
func GetEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func GetIntEnv(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func GetInt64Env(key string, defaultValue int64) int64 {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.ParseInt(value, 10, 64); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func GetBoolEnv(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolVal, err := strconv.ParseBool(value); err == nil {
			return boolVal
		}
	}
	return defaultValue
}

func GetFloat64Env(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if floatVal, err := strconv.ParseFloat(value, 64); err == nil {
			return floatVal
		}
	}
	return defaultValue
}

func GetDurationEnv(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}
