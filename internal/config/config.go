package config

import (
	"fmt"
	"time"
)

// Config holds all application configuration
type Config struct {
	Environment string

	// Service specific configs
	Ingestion IngestionConfig
	Worker    WorkerConfig
	Registry  RegistryConfig
	Throttler ThrottlerConfig

	// Infrastructure configs
	Server        ServerConfig
	Redis         RedisClientConfig
	Queue         QueueConfig
	Postgres      PostgresClientConfig
	Observability ObservabilityConfig
}

// IngestionConfig holds configuration for the ingestion service
type IngestionConfig struct {
	EnableCircuitBreaker bool
	MaxQueueDepth        int64
	Throttler            ThrottlerConfig
}

type ThrottlerConfig struct {
	Enabled bool

	// Thresholds
	SoftDepthLimit     int64
	HardDepthLimit     int64
	MaxMessageAge      time.Duration
	CriticalMessageAge time.Duration

	// Overload score thresholds
	RateLimitScore   float64 // e.g., 1.0
	CircuitOpenScore float64 // e.g., 1.5

	// Update interval
	MetricsUpdateInterval time.Duration

	// Throughput estimation (messages/second)
	EstimatedDrainRate float64

	// Retry-After bounds
	MinRetryAfter time.Duration
	MaxRetryAfter time.Duration
}

// RegistryConfig holds service discovery
type RegistryConfig struct {
	HeartbeatInterval time.Duration // 10s
	DeadThreshold     time.Duration // 30s
	PollInterval      time.Duration // 15s
	ControlPlanePoll  time.Duration // 15s
	LockTTL           time.Duration // 60s
}

// WorkerConfig holds worker-specific configuration
type WorkerConfig struct {
	Concurrency       int
	PollInterval      time.Duration
	BatchSize         int
	ProcessingTimeout time.Duration
	JobMaxRetries     int
	BackoffMinDelay   time.Duration
	BackoffMaxDelay   time.Duration
	HealthPort        string
	ReclaimInterval   time.Duration
	ReclaimIdleTime   time.Duration
	ReclaimBatchSize  int
	DedupTTL          time.Duration
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	Host            string
	Port            string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
}

// QueueConfig holds Redis stream configuration
type QueueConfig struct {
	BaseStreamName    string
	BaseConsumerGroup string
	RedisMaxRetries   int
	MaxLen            int64

	DLQStreamName string
	DLQMaxLen     int64

	PartitionCount int
	PartitionID    int
}

// RedisClientConfig holds Redis connection configuration
type RedisClientConfig struct {
	Host            string
	Port            string
	Password        string
	DB              int
	PoolSize        int
	MinIdleConns    int
	ConnMaxIdleTime time.Duration
}

// PostgresConfig holds PostgreSQL connection configuration
type PostgresClientConfig struct {
	Host            string
	Port            string
	Database        string
	User            string
	Password        string
	SSLMode         string
	MaxConns        int
	MinConns        int
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
}

// ObservabilityConfig holds logging and metrics configuration
type ObservabilityConfig struct {
	LogLevel           string
	LogFormat          string
	MetricsEnabled     bool
	MetricsPort        string
	TracingEnabled     bool
	TracingSampleRate  float64
	HealthCheckTimeout time.Duration
}

// ServerAddr returns the full server address
func (c *Config) ServerAddr() string {
	return fmt.Sprintf("%s:%s", c.Server.Host, c.Server.Port)
}

// RedisAddr returns the full Redis address
func (c *Config) RedisAddr() string {
	return fmt.Sprintf("%s:%s", c.Redis.Host, c.Redis.Port)
}

// PostgresDSN returns the PostgreSQL connection string
func (c *Config) PostgresDSN() string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		c.Postgres.Host,
		c.Postgres.Port,
		c.Postgres.User,
		c.Postgres.Password,
		c.Postgres.Database,
		c.Postgres.SSLMode,
	)
}

// MetricsAddr returns the metrics endpoint address
func (c *Config) MetricsAddr() string {
	return fmt.Sprintf(":%s", c.Observability.MetricsPort)
}
