package config

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Validate validates the entire configuration and returns all errors found
func Validate(cfg *Config) error {
	var errs []error

	// Validate each section
	if err := validateServer(&cfg.Server); err != nil {
		errs = append(errs, fmt.Errorf("server config: %w", err))
	}

	if err := validateRedis(&cfg.Redis); err != nil {
		errs = append(errs, fmt.Errorf("redis config: %w", err))
	}

	if err := validateQueue(&cfg.Queue); err != nil {
		errs = append(errs, fmt.Errorf("queue config: %w", err))
	}

	if err := validatePostgres(&cfg.Postgres); err != nil {
		errs = append(errs, fmt.Errorf("postgres config: %w", err))
	}

	if err := validateIngestion(&cfg.Ingestion); err != nil {
		errs = append(errs, fmt.Errorf("ingestion config: %w", err))
	}

	if err := validateWorker(&cfg.Worker); err != nil {
		errs = append(errs, fmt.Errorf("worker config: %w", err))
	}

	if err := validateObservability(&cfg.Observability); err != nil {
		errs = append(errs, fmt.Errorf("observability config: %w", err))
	}

	if err := validateThrottler(&cfg.Ingestion.Throttler); err != nil {
		errs = append(errs, err)
	}

	if err := validateRegistry(&cfg.Registry); err != nil {
		errs = append(errs, err)
	}

	// Return aggregated errors
	if len(errs) > 0 {
		return formatErrors(errs)
	}

	return nil
}

func validateServer(cfg *ServerConfig) error {
	var errs []error

	if cfg.Port == "" {
		errs = append(errs, fmt.Errorf("port is required"))
	} else if port, err := strconv.Atoi(cfg.Port); err != nil || port < 1 || port > 65535 {
		errs = append(errs, fmt.Errorf("port must be between 1 and 65535, got: %s", cfg.Port))
	}

	if cfg.ReadTimeout <= 0 {
		errs = append(errs, fmt.Errorf("read timeout must be positive"))
	}

	if cfg.WriteTimeout <= 0 {
		errs = append(errs, fmt.Errorf("write timeout must be positive"))
	}

	if cfg.ShutdownTimeout <= 0 {
		errs = append(errs, fmt.Errorf("shutdown timeout must be positive"))
	}

	return combineErrors(errs)
}

func validateRedis(cfg *RedisClientConfig) error {
	var errs []error

	if cfg.Host == "" {
		errs = append(errs, fmt.Errorf("host is required"))
	}

	if cfg.Port == "" {
		errs = append(errs, fmt.Errorf("port is required"))
	} else if port, err := strconv.Atoi(cfg.Port); err != nil || port < 1 || port > 65535 {
		errs = append(errs, fmt.Errorf("port must be between 1 and 65535, got: %s", cfg.Port))
	}

	if cfg.DB < 0 || cfg.DB > 15 {
		errs = append(errs, fmt.Errorf("db must be between 0 and 15, got: %d", cfg.DB))
	}

	if cfg.PoolSize <= 0 {
		errs = append(errs, fmt.Errorf("pool size must be positive"))
	}

	return combineErrors(errs)
}

func validateQueue(cfg *QueueConfig) error {
	var errs []error

	if cfg.BaseStreamName == "" {
		errs = append(errs, fmt.Errorf("stream name is required"))
	}

	if cfg.BaseConsumerGroup == "" {
		errs = append(errs, fmt.Errorf("consumer group is required"))
	}

	if cfg.RedisMaxRetries <= 0 {
		errs = append(errs, fmt.Errorf("max retries must be positive"))
	}

	if cfg.MaxLen <= 0 {
		errs = append(errs, fmt.Errorf("max length must be positive"))
	}

	if cfg.DLQStreamName == "" {
		errs = append(errs, fmt.Errorf("DLQ stream name is required"))
	}

	if cfg.DLQMaxLen <= 0 {
		errs = append(errs, fmt.Errorf("DLQ max length must be positive"))
	}

	if cfg.DLQStreamName == cfg.BaseStreamName {
		errs = append(errs, fmt.Errorf("DLQ stream name must be different from main stream"))
	}

	if cfg.PartitionCount <= 0 {
		errs = append(errs, fmt.Errorf("partition count must be positive"))
	}

	if cfg.PartitionID < 0 || cfg.PartitionID >= cfg.PartitionCount {
		errs = append(errs, fmt.Errorf("partition ID %d out of range [0, %d)", cfg.PartitionID, cfg.PartitionCount))
	}

	return combineErrors(errs)
}

func validatePostgres(cfg *PostgresClientConfig) error {
	var errs []error

	if cfg.Host == "" {
		errs = append(errs, fmt.Errorf("host is required"))
	}

	if cfg.Port == "" {
		errs = append(errs, fmt.Errorf("port is required"))
	} else if port, err := strconv.Atoi(cfg.Port); err != nil || port < 1 || port > 65535 {
		errs = append(errs, fmt.Errorf("port must be between 1 and 65535, got: %s", cfg.Port))
	}

	if cfg.Database == "" {
		errs = append(errs, fmt.Errorf("database name is required"))
	}

	if cfg.User == "" {
		errs = append(errs, fmt.Errorf("user is required"))
	}

	validSSLModes := []string{"disable", "require", "verify-ca", "verify-full"}
	if !contains(validSSLModes, cfg.SSLMode) {
		errs = append(errs, fmt.Errorf("ssl mode must be one of %v, got: %s", validSSLModes, cfg.SSLMode))
	}

	if cfg.MaxConns <= 0 {
		errs = append(errs, fmt.Errorf("max connections must be positive"))
	}

	if cfg.MinConns < 0 {
		errs = append(errs, fmt.Errorf("min connections cannot be negative"))
	}

	if cfg.MinConns > cfg.MaxConns {
		errs = append(errs, fmt.Errorf("min connections (%d) cannot exceed max connections (%d)", cfg.MinConns, cfg.MaxConns))
	}

	return combineErrors(errs)
}

func validateWorker(cfg *WorkerConfig) error {
	var errs []error

	if cfg.Concurrency <= 0 {
		errs = append(errs, fmt.Errorf("concurrency must be positive"))
	} else if cfg.Concurrency > 1000 {
		errs = append(errs, fmt.Errorf("concurrency cannot exceed 1000, got: %d", cfg.Concurrency))
	}

	if cfg.PollInterval <= 0 {
		errs = append(errs, fmt.Errorf("poll interval must be positive"))
	}

	if cfg.BatchSize <= 0 {
		errs = append(errs, fmt.Errorf("batch size must be positive"))
	}

	if cfg.ProcessingTimeout <= 0 {
		errs = append(errs, fmt.Errorf("processing timeout must be positive"))
	}

	if cfg.JobMaxRetries < 0 {
		errs = append(errs, fmt.Errorf("max retries cannot be negative"))
	}

	if cfg.BackoffMinDelay <= 0 {
		errs = append(errs, fmt.Errorf("backoff min delay must be positive"))
	}

	if cfg.BackoffMaxDelay <= 0 {
		errs = append(errs, fmt.Errorf("backoff max delay must be positive"))
	}

	if cfg.BackoffMinDelay > cfg.BackoffMaxDelay {
		errs = append(errs, fmt.Errorf("backoff min delay (%s) cannot exceed max delay (%s)",
			cfg.BackoffMinDelay, cfg.BackoffMaxDelay))
	}

	if cfg.HealthPort == "" {
		errs = append(errs, fmt.Errorf("health port is required"))
	}

	// Reclaim validation
	if cfg.ReclaimInterval <= 0 {
		errs = append(errs, fmt.Errorf("reclaim interval must be positive"))
	}

	if cfg.ReclaimIdleTime <= 0 {
		errs = append(errs, fmt.Errorf("reclaim idle time must be positive"))
	}

	if cfg.ReclaimIdleTime < cfg.ReclaimInterval {
		errs = append(errs, fmt.Errorf("reclaim idle time (%s) must be >= reclaim interval (%s), ideally 2-3x",
			cfg.ReclaimIdleTime, cfg.ReclaimInterval))
	}

	if cfg.ReclaimBatchSize <= 0 {
		errs = append(errs, fmt.Errorf("reclaim batch size must be positive"))
	}

	if cfg.DedupTTL <= 0 {
		errs = append(errs, fmt.Errorf("deduplication TTL must be positive"))
	}

	return combineErrors(errs)
}

func validateIngestion(cfg *IngestionConfig) error {
	var errs []error

	if cfg.MaxQueueDepth < 0 {
		errs = append(errs, fmt.Errorf("max queue depth cannot be negative"))
	}

	return combineErrors(errs)
}

func validateObservability(cfg *ObservabilityConfig) error {
	var errs []error

	validLogLevels := []string{"debug", "info", "warn", "error"}
	if !contains(validLogLevels, strings.ToLower(cfg.LogLevel)) {
		errs = append(errs, fmt.Errorf("log level must be one of %v, got: %s", validLogLevels, cfg.LogLevel))
	}

	validLogFormats := []string{"json", "text"}
	if !contains(validLogFormats, strings.ToLower(cfg.LogFormat)) {
		errs = append(errs, fmt.Errorf("log format must be one of %v, got: %s", validLogFormats, cfg.LogFormat))
	}

	if cfg.MetricsEnabled {
		if cfg.MetricsPort == "" {
			errs = append(errs, fmt.Errorf("metrics port is required when metrics are enabled"))
		} else if port, err := strconv.Atoi(cfg.MetricsPort); err != nil || port < 1 || port > 65535 {
			errs = append(errs, fmt.Errorf("metrics port must be between 1 and 65535, got: %s", cfg.MetricsPort))
		}
	}

	if cfg.TracingEnabled {
		if cfg.TracingSampleRate < 0.0 || cfg.TracingSampleRate > 1.0 {
			errs = append(errs, fmt.Errorf("tracing sample rate must be between 0.0 and 1.0, got: %.2f", cfg.TracingSampleRate))
		}
	}

	if cfg.HealthCheckTimeout <= 0 {
		errs = append(errs, fmt.Errorf("health check timeout must be positive"))
	}

	return combineErrors(errs)
}

func validateThrottler(cfg *ThrottlerConfig) error {
	if !cfg.Enabled {
		return nil // skip validation if throttler is disabled
	}

	var errs []error

	if cfg.SoftDepthLimit <= 0 {
		errs = append(errs, fmt.Errorf("throttler soft depth limit must be positive"))
	}
	if cfg.HardDepthLimit <= 0 {
		errs = append(errs, fmt.Errorf("throttler hard depth limit must be positive"))
	}
	if cfg.HardDepthLimit <= cfg.SoftDepthLimit {
		errs = append(errs, fmt.Errorf("throttler hard depth limit must be greater than soft depth limit"))
	}
	if cfg.MaxMessageAge <= 0 {
		errs = append(errs, fmt.Errorf("throttler max message age must be positive"))
	}
	if cfg.CriticalMessageAge <= cfg.MaxMessageAge {
		errs = append(errs, fmt.Errorf("throttler critical message age must be greater than max message age"))
	}
	if cfg.RateLimitScore <= 0 {
		errs = append(errs, fmt.Errorf("throttler rate limit score must be positive"))
	}
	if cfg.CircuitOpenScore <= cfg.RateLimitScore {
		errs = append(errs, fmt.Errorf("throttler circuit open score must be greater than rate limit score"))
	}
	if cfg.MetricsUpdateInterval <= 0 {
		errs = append(errs, fmt.Errorf("throttler metrics update interval must be positive"))
	}
	if cfg.EstimatedDrainRate <= 0 {
		errs = append(errs, fmt.Errorf("throttler estimated drain rate must be positive"))
	}
	if cfg.MinRetryAfter <= 0 {
		errs = append(errs, fmt.Errorf("throttler min retry after must be positive"))
	}
	if cfg.MaxRetryAfter <= cfg.MinRetryAfter {
		errs = append(errs, fmt.Errorf("throttler max retry after must be greater than min retry after"))
	}

	return combineErrors(errs)
}

func validateRegistry(cfg *RegistryConfig) error {
	var errs []error

	if cfg.HeartbeatInterval <= 0 {
		errs = append(errs, fmt.Errorf("registry heartbeat interval must be positive"))
	}
	if cfg.DeadThreshold <= cfg.HeartbeatInterval {
		errs = append(errs, fmt.Errorf("registry dead threshold must be greater than heartbeat interval"))
	}
	if cfg.PollInterval <= 0 {
		errs = append(errs, fmt.Errorf("registry poll interval must be positive"))
	}
	if cfg.ControlPlanePoll <= 0 {
		errs = append(errs, fmt.Errorf("registry control plane poll interval must be positive"))
	}
	if cfg.LockTTL <= cfg.ControlPlanePoll {
		errs = append(errs, fmt.Errorf("registry lock TTL must be greater than control plane poll interval"))
	}

	return combineErrors(errs)
}

// combineErrors combines multiple errors into a single error
// Returns nil if slice is empty
func combineErrors(errs []error) error {
	if len(errs) == 0 {
		return nil
	}

	if len(errs) == 1 {
		return errs[0]
	}

	// Multiple errors - combine them
	var sb strings.Builder
	for i, err := range errs {
		if i > 0 {
			sb.WriteString("; ")
		}
		sb.WriteString(err.Error())
	}
	return errors.New(sb.String())
}

// formatErrors formats multiple top-level errors with nice formatting
func formatErrors(errs []error) error {
	var sb strings.Builder
	sb.WriteString("configuration validation failed:\n")
	for _, err := range errs {
		sb.WriteString("  - ")
		sb.WriteString(err.Error())
		sb.WriteString("\n")
	}
	return errors.New(sb.String())
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
