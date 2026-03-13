package config

import "time"

// Default configuration values
const (
	DefaultEnvironment = "development"

	// Ingestion defaults
	DefaultEnableCircuitBreaker = true
	DefaultMaxQueueDepth        = 1000

	// Throttler defaults
	DefaultThrottlerEnabled               = true
	DefaultThrottlerSoftDepthLimit        = 100
	DefaultThrottlerHardDepthLimit        = 5000
	DefaultThrottlerMaxMessageAge         = 30 * time.Second
	DefaultThrottlerCriticalMessageAge    = 60 * time.Second
	DefaultThrottlerRateLimitScore        = 1.0
	DefaultThrottlerCircuitOpenScore      = 1.5
	DefaultThrottlerMetricsUpdateInterval = 500 * time.Millisecond
	DefaultThrottlerEstimatedDrainRate    = 100.0 // messages per second
	DefaultThrottlerMinRetryAfter         = 1 * time.Second
	DefaultThrottlerMaxRetryAfter         = 30 * time.Second

	// Registry defaults
	DefaultRegistryHeartbeatInterval = 10 * time.Second
	DefaultRegistryDeadThreshold     = 30 * time.Second
	DefaultRegistryPollInterval      = 15 * time.Second
	DefaultRegistryControlPlanePoll  = 15 * time.Second
	DefaultRegistryLockTTL           = 60 * time.Second

	// Worker defaults
	DefaultWorkerConcurrency      = 2
	DefaultWorkerPollInterval     = 100 * time.Millisecond
	DefaultWorkerBatchSize        = 10
	DefaultWorkerProcessTimeout   = 30 * time.Second
	DefaultWorkerMaxRetries       = 3
	DefaultWorkerBackoffMinDelay  = 500 * time.Millisecond
	DefaultWorkerBackoffMaxDelay  = 5 * time.Second
	DefaultWorkerHealthPort       = "8081"
	DefaultWorkerReclaimInterval  = 30 * time.Second
	DefaultWorkerReclaimIdleTime  = 60 * time.Second
	DefaultWorkerReclaimBatchSize = 100
	DefaultWorkerDedupTTL         = 1 * time.Hour

	// Server defaults
	DefaultServerHost      = "0.0.0.0"
	DefaultServerPort      = "8080"
	DefaultReadTimeout     = 10 * time.Second
	DefaultWriteTimeout    = 10 * time.Second
	DefaultIdleTimeout     = 30 * time.Second
	DefaultShutdownTimeout = 30 * time.Second

	// Redis defaults
	DefaultRedisHost         = "localhost"
	DefaultRedisPort         = "6379"
	DefaultRedisDB           = 0
	DefaultRedisPoolSize     = 10
	DefaultRedisMinIdleConns = 5
	DefaultRedisConnMaxIdle  = 5 * time.Minute

	// Queue defaults
	DefaultQueueBaseStreamName    = "logs:ingest"
	DefaultQueueBaseConsumerGroup = "log-workers"
	DefaultQueueRedisMaxRetries   = 3
	DefaultQueueMaxLen            = 10000
	DefaultQueueDLQStreamName     = "logs:dlq"
	DefaultQueueDLQMaxLen         = 1000
	DefaultQueuePartitionCount    = 2
	DefaultQueueParitionID        = 0

	// Postgres defaults
	DefaultPostgresHost        = "localhost"
	DefaultPostgresPort        = "5432"
	DefaultPostgresDB          = "logdb"
	DefaultPostgresUser        = "postgres"
	DefaultPostgresPassword    = "postgres"
	DefaultPostgresSSLMode     = "disable"
	DefaultPostgresMaxConns    = 25
	DefaultPostgresMinConns    = 5
	DefaultPostgresMaxConnLife = 1 * time.Hour
	DefaultPostgresMaxConnIdle = 30 * time.Minute

	// Observability defaults
	DefaultLogLevel           = "info"
	DefaultLogFormat          = "json"
	DefaultMetricsEnabled     = true
	DefaultMetricsPort        = "9090"
	DefaultTracingEnabled     = false
	DefaultTracingSampleRate  = 0.1
	DefaultHealthCheckTimeout = 5 * time.Second
)

// GetDefaults returns a Config with all default values
func GetDefaults() *Config {
	return &Config{
		Environment: DefaultEnvironment,
		Server: ServerConfig{
			Host:            DefaultServerHost,
			Port:            DefaultServerPort,
			ReadTimeout:     DefaultReadTimeout,
			WriteTimeout:    DefaultWriteTimeout,
			IdleTimeout:     DefaultIdleTimeout,
			ShutdownTimeout: DefaultShutdownTimeout,
		},
		Redis: RedisClientConfig{
			Host:            DefaultRedisHost,
			Port:            DefaultRedisPort,
			Password:        "",
			DB:              DefaultRedisDB,
			PoolSize:        DefaultRedisPoolSize,
			MinIdleConns:    DefaultRedisMinIdleConns,
			ConnMaxIdleTime: DefaultRedisConnMaxIdle,
		},
		Queue: QueueConfig{
			BaseStreamName:    DefaultQueueBaseStreamName,
			BaseConsumerGroup: DefaultQueueBaseConsumerGroup,
			RedisMaxRetries:   DefaultQueueRedisMaxRetries,
			MaxLen:            DefaultQueueMaxLen,

			DLQStreamName: DefaultQueueDLQStreamName,
			DLQMaxLen:     DefaultQueueDLQMaxLen,

			PartitionCount: DefaultQueuePartitionCount,
			PartitionID:    DefaultQueueParitionID,
		},
		Postgres: PostgresClientConfig{
			Host:            DefaultPostgresHost,
			Port:            DefaultPostgresPort,
			Database:        DefaultPostgresDB,
			User:            DefaultPostgresUser,
			Password:        DefaultPostgresPassword,
			SSLMode:         DefaultPostgresSSLMode,
			MaxConns:        DefaultPostgresMaxConns,
			MinConns:        DefaultPostgresMinConns,
			MaxConnLifetime: DefaultPostgresMaxConnLife,
			MaxConnIdleTime: DefaultPostgresMaxConnIdle,
		},
		Ingestion: IngestionConfig{
			EnableCircuitBreaker: DefaultEnableCircuitBreaker,
			MaxQueueDepth:        DefaultMaxQueueDepth,
			Throttler: ThrottlerConfig{
				Enabled:               DefaultThrottlerEnabled,
				SoftDepthLimit:        DefaultThrottlerSoftDepthLimit,
				HardDepthLimit:        DefaultThrottlerHardDepthLimit,
				MaxMessageAge:         DefaultThrottlerMaxMessageAge,
				CriticalMessageAge:    DefaultThrottlerCriticalMessageAge,
				RateLimitScore:        DefaultThrottlerRateLimitScore,
				CircuitOpenScore:      DefaultThrottlerCircuitOpenScore,
				MetricsUpdateInterval: DefaultThrottlerMetricsUpdateInterval,
				EstimatedDrainRate:    DefaultThrottlerEstimatedDrainRate,
				MinRetryAfter:         DefaultThrottlerMinRetryAfter,
				MaxRetryAfter:         DefaultThrottlerMaxRetryAfter,
			},
		},
		Worker: WorkerConfig{
			Concurrency:       DefaultWorkerConcurrency,
			PollInterval:      DefaultWorkerPollInterval,
			BatchSize:         DefaultWorkerBatchSize,
			ProcessingTimeout: DefaultWorkerProcessTimeout,
			JobMaxRetries:     DefaultWorkerMaxRetries,
			BackoffMinDelay:   DefaultWorkerBackoffMinDelay,
			BackoffMaxDelay:   DefaultWorkerBackoffMaxDelay,
			HealthPort:        DefaultWorkerHealthPort,
			ReclaimInterval:   DefaultWorkerReclaimInterval,
			ReclaimIdleTime:   DefaultWorkerReclaimIdleTime,
			ReclaimBatchSize:  DefaultWorkerBatchSize,
			DedupTTL:          DefaultWorkerDedupTTL,
		},
		Observability: ObservabilityConfig{
			LogLevel:           DefaultLogLevel,
			LogFormat:          DefaultLogFormat,
			MetricsEnabled:     DefaultMetricsEnabled,
			MetricsPort:        DefaultMetricsPort,
			TracingEnabled:     DefaultTracingEnabled,
			TracingSampleRate:  DefaultTracingSampleRate,
			HealthCheckTimeout: DefaultHealthCheckTimeout,
		},
		Throttler: ThrottlerConfig{
			Enabled:               DefaultThrottlerEnabled,
			SoftDepthLimit:        DefaultThrottlerSoftDepthLimit,
			HardDepthLimit:        DefaultThrottlerHardDepthLimit,
			MaxMessageAge:         DefaultThrottlerMaxMessageAge,
			CriticalMessageAge:    DefaultThrottlerCriticalMessageAge,
			RateLimitScore:        DefaultThrottlerRateLimitScore,
			CircuitOpenScore:      DefaultThrottlerCircuitOpenScore,
			MetricsUpdateInterval: DefaultThrottlerMetricsUpdateInterval,
			EstimatedDrainRate:    DefaultThrottlerEstimatedDrainRate,
			MinRetryAfter:         DefaultThrottlerMinRetryAfter,
			MaxRetryAfter:         DefaultThrottlerMaxRetryAfter,
		},
		Registry: RegistryConfig{
			HeartbeatInterval: DefaultRegistryHeartbeatInterval,
			DeadThreshold:     DefaultRegistryDeadThreshold,
			PollInterval:      DefaultRegistryPollInterval,
			ControlPlanePoll:  DefaultRegistryControlPlanePoll,
			LockTTL:           DefaultRegistryLockTTL,
		},
	}
}
