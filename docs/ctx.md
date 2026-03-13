## Project Structure

fluffy-dollop/
├── build/                  # Dockerfiles
│   ├── ingestion/          
│   ├── worker/             
├── cmd/                    # Binary executables 
│   ├── ingestion/
│   │   └── main.go         # Ingestion server entry point
│   └── worker/
│       └── main.go         # Worker server entry point
├── internal/
│   ├── bootstrap/          # Dependency injection & setup 
│   │   ├── consumer.go
│   │   ├── health.go   
│   │   ├── producer.go      
│   │   ├── redis.go  
│   │   └── throttler.go  
│   ├── config/             # Configuration management         
│   │   ├── config.go
│   │   ├── default.go
│   │   ├── loader.go
│   │   └── validator.go 
│   ├── health/             # Health checks
│   │   ├── aggregator.go   
│   │   ├── checker.go      
│   │   ├── http.go         
│   │   ├── postgres.go
│   │   └── redis.go
│   ├── ingestion/          # HTTP ingestion layer
│   │   ├── adaptive_throttler.go
│   │   ├── handler.go
│   │   ├── middleware.go   
│   │   ├── response.go   
│   │   ├── router.go  
│   │   ├── server.go 
│   │   ├── service.go     
│   │   └── throttler.go   
│   ├── observability/      # Logging           
│   │   ├── logger.go       
│   │   ├── metrics.go      
│   │   └── tracing.go
│   ├── queue/              # Redis queue opterations
│   │   ├── consumer.go       
│   │   ├── dlq.go      
│   │   ├── interfaces.go    
│   │   ├── message.go   
│   │   ├── producer.go        
│   │   └── types.go        # JobEnvelope, DLQEntry 
│   ├── worker/             # Job processing worker
│   │   ├── interfaces.go          
│   │   ├── processor.go
│   │   ├── server.go
│   │   └── worker.go                
│   └── storage/
│   │   └── postgres/ 
├── monitoring/         
│   ├── grafana/
│   └── prometheus/
├── pkg/
│   └── models/           # Shared data models (LogEvent - API contract)
├── infra/
│   ├── nginx/
│   ├── grafana/
│   ├── prometheus/
│   ├── postgres/
│   │   └── init.sql
│   │   └── migriations/
│   └── redis/
│       └── redis.conf
├── scripts/            
├── tests/  
│   ├── e2e/            
│   ├── fixtures/       
│   ├── integration/
│   │   ├── concurrency_test.go
│   │   ├── happy_path_test.go
│   │   ├── redis_failure_test.go
│   │   ├── retry_test.go
│   │   └── suite_test.go
│   └── testutil/         
│       ├── env.go
│       ├── health.go
│       ├── naming.go
│       ├── processor.go
│       ├── redis.go
│       └── wait.go
├── docker-compose.yml
├── .env
├── .env.test
├── go.work
├── go.mod
└── README.md

## Project Summary & Goals
This project implements a distributed log ingestion and processing system designed to collect structured log events, process them asynchronously, and persist aggregated metrics with strong operational visibility. The system is intentionally scoped to demonstrate core distributed systems concepts—reliability, delivery guarantees, backpressure, and observability—without the operational overhead of a full-scale logging platform.

The primary goals are:

* Reliable ingestion of JSON log events via an HTTP API
* Asynchronous processing using a distributed job queue
* At-least-once delivery semantics with bounded retries
* Aggregation-oriented storage rather than raw log retention
* Clear observability into system behavior under load and failure

## Data Flow
1. A client sends a JSON log event to the ingestion API.
2. The ingestion service applies rate limiting and validates the payload.
3. A job envelope is created and appended to a Redis Stream.
4. Worker processes claim jobs from the stream via a consumer group.
5. Workers aggregate log events into time-bucketed metrics.
6. Aggregated results are written to PostgreSQL in a single transaction.
7. Upon successful commit, the job is acknowledged in Redis.
8. Failed jobs are retried or sent to a Dead Letter Queue.

## Project Components

Ingestion

Queue

Worker

Observability

Tests

Bootstrap

Config

Health

## Claude Output

types.go 

```
type JobEnvelope struct {
    JobID      string           `json:"job_id"`
    Data       *models.LogEvent `json:"data"`
    EnqueuedAt int64            `json:"enqueued_at"`
    RetryCount int              `json:"retry_count"`
    MaxRetries int              `json:"max_retries"`
    EventType  string           `json:"event_type"`
    LastError  string           `json:"last_error,omitempty"`
}

type DLQEntry struct {
    OriginalStreamID string       `json:"original_stream_id"`
    JobData          *JobEnvelope `json:"job_data"`
    FailureReason    string       `json:"failure_reason"`
    SentToDLQAt      int64        `json:"sent_to_dlq_at"`
}
```

interfaces.go
```
type ProducerInterface interface {
    Enqueue(ctx context.Context, logEvent *models.LogEvent) (string, error)
    EnqueueBatch(ctx context.Context, logEvents []*models.LogEvent) ([]string, error)
    StreamLength(ctx context.Context) (int64, error)
    Ping(ctx context.Context) error
}

type ConsumerInterface interface {
    Consume(ctx context.Context, blockDuration time.Duration) (*JobEnvelope, string, error)
    Ack(ctx context.Context, messageID string) error
    Retry(ctx context.Context, envelope *JobEnvelope, messageID string, lastError error) error
    ReclaimStale(ctx context.Context, idleTime time.Duration, maxBatch int) (int, error)
    GetPendingInfo(ctx context.Context) (count int64, oldestAge int64, err error)
}

type DLQInterface interface {
    Send(ctx context.Context, entry *DLQEntry) (string, error)
    Get(ctx context.Context, messageID string) (*DLQEntry, error)
    List(ctx context.Context, start, end string, count int64) ([]*DLQEntry, error)
    Count(ctx context.Context) (int64, error)
    Delete(ctx context.Context, messageID string) error
    Requeue(ctx context.Context, messageID string, mainStreamName string, maxLen int64) error
}
```

producer.go
```
type Producer struct {
    client     *redis.Client
    streamName string
    maxLen     int64
    logger     *observability.Logger
}

func NewProducer(client *redis.Client, streamName string, maxLen int64, logger *observability.Logger) *Producer
func (p *Producer) Enqueue(ctx context.Context, logEvent *models.LogEvent) (string, error)
func (p *Producer) EnqueueBatch(ctx context.Context, logEvents []*models.LogEvent) ([]string, error)
func (p *Producer) StreamLength(ctx context.Context) (int64, error)
func (p *Producer) Ping(ctx context.Context) error
```

consumer.go
```
type Consumer struct {
    client        *redis.Client
    streamName    string
    consumerGroup string
    consumerName  string
    logger        *observability.Logger
    dlq           *DLQ
}

func NewConsumer(client *redis.Client, streamName, consumerGroup, consumerName string, logger *observability.Logger, dlq *DLQ) *Consumer
func (c *Consumer) Consume(ctx context.Context, blockDuration time.Duration) (*JobEnvelope, string, error)
func (c *Consumer) Ack(ctx context.Context, messageID string) error
func (c *Consumer) Retry(ctx context.Context, envelope *JobEnvelope, messageID string, lastError error) error
func (c *Consumer) ReclaimStale(ctx context.Context, idleTime time.Duration, maxBatch int) (int, error)
func (c *Consumer) GetPendingInfo(ctx context.Context) (count int64, oldestAge int64, err error)
```

**Notes** 
Increments RetryCount
If RetryCount > MaxRetries → Send to DLQ, then ACK
Otherwise → Re-enqueue with incremented count, then ACK original

dlq.go
```
type DLQ struct {
    client     *redis.Client
    streamName string
    maxLen     int64
    logger     *observability.Logger
}

func NewDLQ(client *redis.Client, streamName string, maxLen int64, logger *observability.Logger) *DLQ
func (d *DLQ) Send(ctx context.Context, entry *DLQEntry) (string, error)
func (d *DLQ) Get(ctx context.Context, messageID string) (*DLQEntry, error)
func (d *DLQ) List(ctx context.Context, start, end string, count int64) ([]*DLQEntry, error)
func (d *DLQ) Count(ctx context.Context) (int64, error)
func (d *DLQ) Delete(ctx context.Context, messageID string) error
func (d *DLQ) Requeue(ctx context.Context, messageID string, mainStreamName string, maxLen int64) error
```

worker.go
```
type Worker struct {
    consumer     queue.ConsumerInterface
    processor    Processor
    config       *config.WorkerConfig
    logger       *observability.Logger
    sem          *semaphore.Weighted
    backoff      *exponentialBackoff
    healthServer HealthServerInterface
    activeJobs   atomic.Int64
    ready        chan struct{}
    readyOnce    sync.Once
}

func NewWorker(consumer queue.ConsumerInterface, processor Processor, cfg *config.Config, logger *observability.Logger, healthServer HealthServerInterface) *Worker
func (w *Worker) Start(ctx context.Context) error
func (w *Worker) Ready() <-chan struct{}
func (w *Worker) ActiveJobs() int64
```

**Notes**
Semaphore for concurrency control
Ready signal via channel (no brittle sleeps)
Two goroutines: pollLoop (consume messages) + reclaimLoop (XAUTOCLAIM stale messages)
Context-based shutdown
Exponential backoff on errors

interfaces.go
```
type Processor interface {
    Process(ctx context.Context, envelope *queue.JobEnvelope) error
}

type HealthServerInterface interface {
    StartAsync()
    Close() error
}
```

service.go
```
type Service struct {
    producer  queue.ProducerInterface
    throttler ThrottlerInterface
    config    *config.IngestionConfig
    logger    *observability.Logger
}

func NewService(producer queue.ProducerInterface, throttler ThrottlerInterface, cfg *config.Config, logger *observability.Logger) *Service
func (s *Service) IngestLog(ctx context.Context, logEvent *models.LogEvent) (string, error)
```

**Notes**
Check throttler decision
If circuit open → return ErrCircuitOpen
If rate limited → return ErrRateLimited{RetryAfter: duration}
Validate log event
Enqueue to producer

error.go
```
var (
    ErrValidationFailed = errors.New("log event validation failed")
    ErrCircuitOpen      = errors.New("circuit breaker open - system overloaded")
)

type ErrRateLimited struct {
    RetryAfter time.Duration
}
```

throttler.go
```
type ThrottleDecision int

const (
    ThrottleAllow ThrottleDecision = iota
    ThrottleRateLimit
    ThrottleCircuitOpen
)

type ThrottlerInterface interface {
    Start(ctx context.Context)
    Stop()
    Decision() (ThrottleDecision, time.Duration)
    GetMetrics() (depth, pending, oldestAge int64)
}

type NoopThrottler struct{} // For testing
```

adaptive_throttler.go
```
type Throttler struct {
    producer queue.ProducerInterface
    consumer queue.ConsumerInterface
    config   *config.ThrottlerConfig
    logger   *observability.Logger
    metrics  *ThrottlerMetrics
    ctx      context.Context
    cancel   context.CancelFunc
    wg       sync.WaitGroup
}

func NewThrottler(producer queue.ProducerInterface, consumer queue.ConsumerInterface, config *config.ThrottlerConfig, logger *observability.Logger) *Throttler
func (t *Throttler) Start(parentCtx context.Context)
func (t *Throttler) Stop()
func (t *Throttler) Decision() (ThrottleDecision, time.Duration)
```

**Notes**
Check if metrics are stale (> 2x update interval) → Fail open
Calculate overload score: max(depth/softLimit, age/maxAge)
If score > 1.5 → Circuit open (503)
If score > 1.0 → Rate limit (429 + Retry-After)
Otherwise → Allow

Retry-After Calculation:
backlog = depth + pending
retrySeconds = backlog / estimatedDrainRate
Apply age multiplier if messages are old
Clamp between min (1s) and max (30s)

redis.go
```
func SetupRedis(ctx context.Context, cfg *config.Config, logger *observability.Logger) (*redis.Client, error)
```

producer.go
```
func SetupProducer(client *redis.Client, cfg *config.Config, logger *observability.Logger) queue.ProducerInterface
```

consumer.go
```
func SetupConsumer(ctx context.Context, client *redis.Client, cfg *config.Config, logger *observability.Logger) (queue.ConsumerInterface, error)
```

**Notes**
Creates consumer group with XGROUP CREATE MKSTREAM
Creates DLQ and injects into consumer

throttler.go
```
func SetupThrottler(ctx context.Context, producer queue.ProducerInterface, redisClient *redis.Client, cfg *config.Config, logger *observability.Logger) ingestion.ThrottlerInterface
```

**Notes**
Creates consumer for pending metrics
Returns NoopThrottler if disabled

env.go
```
func LoadTestEnv() error
```

**Notes**
Loads .env.test from project root using runtime.Caller

naming.go
```
func UniqueName(t *testing.T, prefix string) string
```

**Notes**
Generates unique stream/group names per test: prefix-TestName-RandomInt

processor.go
```
type SucceedingProcessor struct {
    processCount atomic.Int64
    logger       *observability.Logger
}

func NewSucceedingProcessor(logger *observability.Logger) *SucceedingProcessor
func (p *SucceedingProcessor) Process(ctx context.Context, envelope *queue.JobEnvelope) error
func (p *SucceedingProcessor) ProcessCount() int64

type FailingProcessor struct {
    processCount atomic.Int64
    errorToReturn error
    logger        *observability.Logger
}

func NewFailingProcessor(err error, logger *observability.Logger) *FailingProcessor
```

redis.go
```
func GetPendingCount(client *redis.Client, stream, group string) (int64, error)
func GetStreamLength(client *redis.Client, stream string) (int64, error)
```

wait.go
```
func WaitFor(t *testing.T, timeout time.Duration, condition func() bool, description string)
func WaitForPendingCount(t *testing.T, client *redis.Client, stream, group string, expected int64, timeout time.Duration)
```

health.go
```
type NoopHealthServer struct{}
func NewNoopHealthServer() *NoopHealthServer
```

suite_test.go
```
type Suite struct {
    Redis    *redis.Client
    Producer queue.ProducerInterface
    Config   *config.Config
    Logger   *observability.Logger
}

func newSuite(t *testing.T) *Suite {
    cfg, _ := config.Load()
    cfg.Queue.StreamName = testutil.UniqueName(t, "stream")
    cfg.Queue.ConsumerGroup = testutil.UniqueName(t, "group")
    
    redisClient, _ := bootstrap.SetupRedis(ctx, cfg, logger)
    t.Cleanup(func() {
        redisClient.Del(context.Background(), cfg.Queue.StreamName)
        redisClient.Close()
    })
    
    producer := bootstrap.SetupProducer(redisClient, cfg, logger)
    return &Suite{Redis: redisClient, Producer: producer, Config: cfg, Logger: logger}
}
```

```
func (s *Suite) newWorker(t *testing.T, ctx context.Context, processor worker.Processor) *worker.Worker {
    consumer, _ := bootstrap.SetupConsumer(ctx, s.Redis, s.Config, s.Logger)
    healthServer := testutil.NewNoopHealthServer()
    return worker.NewWorker(consumer, processor, s.Config, s.Logger, healthServer)
}
```


happy_path_test.go
```
func TestHappyPath(t *testing.T) // Table-driven: 1, 5, 20 jobs
func TestHappyPath_EnqueueProcessAck(t *testing.T)
func TestHappyPath_MultipleJobs(t *testing.T)
```

Enqueue → Worker consumes → Process → ACK
Worker ready signal
Unique streams per test (no FlushDB needed)
Pending count reaches 0

---

## TESTS TODO
```
// Test max retries → DLQ
func TestDLQ_MaxRetriesExceeded(t *testing.T)

// Test DLQ.Send
func TestDLQ_Send(t *testing.T)

// Test DLQ.Get
func TestDLQ_Get(t *testing.T)

// Test DLQ.List
func TestDLQ_List(t *testing.T)

// Test DLQ.Requeue
func TestDLQ_Requeue(t *testing.T)

// Test job fails multiple times then goes to DLQ
func TestRetry_JobEventuallyReachesDLQ(t *testing.T)
```

```
// Test job retries on failure
func TestRetry_JobRetriesOnFailure(t *testing.T)

// Test retry backoff
func TestRetry_ExponentialBackoff(t *testing.T)
```

```
// Test rate limiting (429 + Retry-After)
func TestThrottler_RateLimit(t *testing.T)

// Test circuit breaker (503)
func TestThrottler_CircuitBreaker(t *testing.T)

// Test stale metrics (fail open)
func TestThrottler_StaleMetricsFallsOpen(t *testing.T)

// Test overload score calculation
func TestThrottler_OverloadScore(t *testing.T)
```

```
// Test graceful shutdown
func TestWorker_GracefulShutdown(t *testing.T)

// Test concurrency limit
func TestWorker_ConcurrencyLimit(t *testing.T)

// Test XAUTOCLAIM reclaim
func TestWorker_ReclaimStaleMessages(t *testing.T)
```






Node registry (keep track of all active worker containers, Redis or in-memory)
Simple WAL at ingestion layer (track last_flushed_offset + last_enqueued_offset)
Deduplication strategy (before postgres upsert)
auto-scaling recommendations based on queue health metrics. 
paritioned Redis Streams queues using consistent hashing to disribute load across worker groups (workers subscribe to all partitions?)

implement load testing 


Useful for observability and metrics aggregation
Dynamic configuration
Pull throttling limits, retry counts, or other configs from a central source
Shows understanding of runtime coordination without implementing leader election
Partition simulation (optional)
Split jobs logically (by type, priority, or hash of some key)
Assign each “partition” to a worker group
Demonstrates understanding of sharding without full Kafka-style partitioning
Visualization / dashboard
Show number of workers, queue depth, inflight jobs
Very beginner-friendly but highly impressive visually
HA / failover demonstration
Spin up multiple workers, kill one, show DLQ/job reclaim works
You can script this in tests or even a small dashboard