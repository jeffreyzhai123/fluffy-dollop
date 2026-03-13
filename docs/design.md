# Project: Distributed Log Ingestion & Processing Pipeline (Go)

## System Goal:
Build a distributed log ingestion and processing system that reliably collects JSON log events, processes them asynchronously using a job queue, aggregates metrics, and exposes operational observability, while providing at-least-once delivery guarantees.

## Design Considerations:
Redis Streams were chosen to model a distributed, persistent job queue with consumer groups and at-least-once delivery, without the operational overhead of a full messaging system such as Kafka.

This system relies on Redis persistence (AOF/RDB) for durability. Implementing a custom WAL was intentionally avoided to keep the project focused on delivery semantics and worker reliability.

The system provides at-least-once delivery semantics. Jobs may be reprocessed in failure scenarios, and workers are designed to be idempotent at the aggregation layer.

## Functional Requirements:

### Log Ingestion
FR-1: The system shall expose an HTTP API that accepts JSON-formatted log events.

FR-2: The ingestion service shall validate incoming log events against a predefined schema.

FR-3: The ingestion service shall apply rate limiting per client to prevent overload.

FR-4: Upon successful enqueueing, the ingestion service shall acknowledge receipt to the client.

FR-5: If the broker is unavailable, the ingestion service shall fail fast and return a retriable error.

### Asynchronous Processing
FR-6: Accepted log events shall be enqueued into a distributed job queue for asynchronous processing.

FR-7: The queue shall support consumer groups and pending job recovery.

FR-8: Jobs shall be processed by a pool of worker processes running concurrently.

FR-9: The system shall provide at-least-once delivery semantics.

### Log Processing & Aggregation
FR-10: Workers shall aggregate log events into time-bucketed metrics (e.g., counts per service and log level).

FR-11: Aggregation shall be idempotent at the bucket level to tolerate duplicate job processing.

FR-12: Aggregation results shall be persisted to durable storage.

### Storage
FR-13: The system shall persist aggregated log metrics in a relational database.

FR-14: Stored metrics shall support basic analytical queries (e.g., error rate per service per minute).

### Observability & Monitoring
FR-15: Each service shall expose metrics related to throughput, latency, and error rates.

FR-16: The system shall track queue depth and job processing lag.

FR-17: The system shall support basic request tracing across ingestion and processing stages.

### Failure Handling & DLQ

FR-18: Jobs that repeatedly fail processing beyond a retry threshold shall be moved to a Dead Letter Queue.

FR-19: Operators shall be able to inspect failed jobs for debugging purposes.

## Non-functional Requirements:

NFR-1: The system prioritizes simplicity and correctness over maximum throughput.

NFR-2: The system is designed to run on a single machine but simulates distributed behavior via multiple processes.

NFR-3: Horizontal scalability is demonstrated conceptually but not automated.

## Tech Stack

Language:
- Go
    - goroutines
    - channels
    - context
    - net/http

Broker:
- Redis Streams
    - Consumer groups
    - Pending entries
    - Acknowledgements

Storage:
- PostgreSQL
    - Aggregated metrics only

Observability:
- Prometheus metrics
- Structured logging
- Optional OpenTelemetry tracing

Deployment:
- Docker Compose
    - ingestion service
    - worker service
    - Redis
    - Postgres

## Testing Scenarios:
Redis unavailable
Worker crash mid-batch
DB failure
Rate-limit exceeded

## Deliverables:
- Benchmark throughput
- Validate backpressure
- Show metrics moving

Write the README skeleton (DONE)
Draw the architecture diagram (DONE)
Define schemas (DONE)
Write Docker Compose (BEGIN)
Then start coding ingestion → queue → worker

Deployment: (Use AWS + GitHub Actions)
Ingestion service (Go HTTP API)

Option A (best): gRPC between ingestion → worker (via Redis metadata)
Workers expose a /health or /stats endpoint
Ingestion occasionally queries worker health
Demonstrates service discovery & RPC


Autoscaling simulated with decisions logged.
Containers will not be spinned up dynamically.

Project Structure (Follows https://github.com/golang-standards/project-layout)
fluffy-dollop/
├── build/
│   ├── ingestion/          # Dockerfile
│   ├── worker/             # Dockerfile
├── cmd/
│   ├── ingestion/
│   │   └── main.go         # starts ingestion service
│   └── worker/
│       └── main.go         # starts worker service
├── internal/
│   ├── ingestion/          # ingestion-specific helpers
│   │   ├── handler.go
│   │   ├── middleware.go   # wraps handler with logger
│   │   ├── server.go     
│   │   ├── router.go
│   │   ├── response.go     # response helpers
│   │   └── service.go.     # not implemented
│   ├── worker/           
│   │   ├── processor.go
│   │   ├── worker.go       # starts up worker & poll loop       
│   ├── config/         
│   │   ├── config.go
│   │   ├── default.go
│   │   ├── loader.go
│   │   └── validator.go
│   ├── health/           
│   │   ├── aggregator.go   # aggregates health check results
│   │   ├── checker.go      # health checker interface
│   │   ├── postgres.go
│   │   └── redis.go
│   ├── observability/           
│   │   ├── logger.go       
│   │   ├── metrics.go      # not implemented
│   │   └── tracing.go      # not implemented
│   ├── queue/           
│   │   ├── consumer.go       
│   │   ├── dlq.go          # not implemented
│   │   ├── producer.go        
│   │   └── message.go      
│   └── storage/
│   │   └── postgres/ 
├── pkg/
│   └── models/           # JobEnvelope, AggregationSchema, API contracts
├── infra/
│   ├── nginx/
│   ├── grafana/
│   ├── prometheus/
│   ├── postgres/
│   │   └── init.sql
│   │   └── migriations/
│   └── redis/
│       └── redis.conf
├── scripts/            # not implemented
├── tests/              # not implemented
│   ├── fixtures/
│   ├── integration/
│   └── testutil/
├── monitoring/         # not implemented
│   ├── grafana/
│   └── prometheus/
├── docker-compose.yml
├── go.work
├── go.mod
└── README.md

## Testing Checklist
```
[ ] Happy path: Request → Enqueue → Process → Ack
[ ] Job timeout: Long job gets cancelled and retried
[ ] Job failure: Failed job retries MaxRetries times
[ ] Max retries exceeded: Job goes to DLQ (implement first)
[ ] Graceful shutdown: SIGTERM drains in-flight work
[ ] Concurrency limits: Semaphore enforces max workers


[ ] Multiple consumers: Work distribution across replicas       
[ ] Redis failure: Graceful error handling
[ ] Message deserialization error: Bad message handling
[ ] Duplicate ACK: Already-acked message doesn't cause issues     (idempotent process, business logic)

func (s *Server) Shutdown(ctx context.Context) error {
    s.logger.LogShutdown("received shutdown signal")
    
    err := s.httpServer.Shutdown(ctx)
    // ...
}
```

**What `httpServer.Shutdown(ctx)` does internally:**

1. **Stop accepting new connections**
   - Closes the listening socket
   - New requests get connection refused

2. **Wait for active requests to finish**
   - Tracks in-flight requests
   - Waits for handlers to return
   - Up to `ctx` timeout (e.g., 30 seconds)

3. **Close idle connections**
   - Keep-alive connections are closed

4. **Return**
   - `nil` if successful
   - Error if context timeout exceeded

**Example timeline:**
```
T+0s: Shutdown() called
T+0s: Stop accepting new connections
T+0s: 3 requests still processing
T+5s: 2 requests finished, 1 still processing
T+8s: Last request finished
T+8s: Shutdown() returns nil

OR (timeout):
T+0s: Shutdown() called (ctx timeout = 30s)
T+30s: Still 1 request processing
T+30s: Context timeout - forcefully close
T+30s: Shutdown() returns context.DeadlineExceeded