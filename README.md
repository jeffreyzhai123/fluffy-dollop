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

Deployment: (Use GCP + GitHub Actions)
Ingestion service (Go HTTP API)
