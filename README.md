# Distributed Log Ingestion & Processing Pipeline

## 1. Overview

This project implements a distributed log ingestion and processing system designed to collect structured log events, process them asynchronously, and persist aggregated metrics with strong operational visibility. The system is intentionally scoped to demonstrate core distributed systems concepts—reliability, delivery guarantees, backpressure, and observability—without the operational overhead of a full-scale logging platform.

The primary goals are:

* Reliable ingestion of JSON log events via an HTTP API
* Asynchronous processing using a distributed job queue
* At-least-once delivery semantics with bounded retries
* Aggregation-oriented storage rather than raw log retention
* Clear observability into system behavior under load and failure

---

## 2. Architecture

### High-Level Components

* **Ingestion Service**: Accepts and validates incoming log events, applies rate limiting, and enqueues jobs for asynchronous processing.
* **Redis Broker**: Acts as a distributed job queue using Redis Streams (Redis Sentinel) with consumer groups and pending-entry recovery.
* **Worker Service**: Consumes jobs, performs aggregation, persists results, and acknowledges successful processing.
* **PostgreSQL**: Durable storage for aggregated log metrics.
* **Observability Stack**: Metrics and structured logs emitted by all services.

**Scope**: Broker replication and leader election are delegated to Redis itself. Reimplementing consensus is out of scope for this project. Auto-scaling is also out of scope of this project. However, the ingestion and worker services are stateless and designed to be horizontally scalable. In a production deployment, multiple instances could be deployed behind a load balancer, with worker concurrency adjusted based on queue depth metrics. 

**Architecture Diagram**
```
Clients
  |
  | HTTP (JSON logs)
  v
Ingestion Service
  |
  | XADD
  v
Redis Streams (Job Queue)
  |
  | XREADGROUP
  v
Worker Pool (Go)
  |
  | Batched UPSERT
  v
PostgreSQL (Aggregated Metrics)
```

---

## 3. Data Flow

1. A client sends a JSON log event to the ingestion API.
2. The ingestion service applies rate limiting and validates the payload.
3. A job envelope is created and appended to a Redis Stream.
4. Worker processes claim jobs from the stream via a consumer group.
5. Workers aggregate log events into time-bucketed metrics.
6. Aggregated results are written to PostgreSQL in a single transaction.
7. Upon successful commit, the job is acknowledged in Redis.
8. Failed jobs are retried or sent to a Dead Letter Queue.

---

## 4. API Contracts

### 4.1 Ingestion API

**Endpoint**

```
POST /v1/logs
```

**Headers**

```
Content-Type: application/json
X-Client-Id: <string>   // optional, used for rate limiting
```

**Request Body**

```json
{
  "source": "payments-api",
  "level": "ERROR",
  "timestamp": "2025-02-03T18:21:04Z",
  "message": "timeout contacting database",
  "trace_id": "abc-123"
}
```

**Responses**

* `202 Accepted`: Log accepted and enqueued
* `400 Bad Request`: Invalid schema or fields
* `429 Too Many Requests`: Rate limit exceeded
* `503 Service Unavailable`: Broker unavailable

---

### 4.2 Job Envelope (Internal)

Jobs enqueued into Redis Streams follow this envelope:

```json
{
  "job_id": "uuid",
  "attempt": 0,
  "created_at": "2025-02-03T18:21:05Z",
  "payload": {
    "source": "payments-api",
    "level": "ERROR",
    "timestamp": "2025-02-03T18:21:04Z"
  }
}
```

---

## 5. Delivery Guarantees

The system provides **at-least-once delivery semantics**.

* Jobs may be processed more than once in failure scenarios.
* Workers are designed to tolerate duplicate processing via idempotent aggregation.
* Jobs are acknowledged **only after** successful database commits.

Exactly-once delivery is intentionally not implemented due to its complexity and limited benefit for log aggregation workloads.

---

## 6. Failure Handling

### Ingestion Failures

* Ingestion layer does not maintain a write-ahead log, data loss is possible.
* Logs are treated as best-effort telemetry rather than transactional data.

### Worker Failures

* Unacknowledged jobs remain pending and can be reclaimed by other workers.

### Broker Unavailability

* Ingestion fails fast and returns `503 Service Unavailable`.
* Possible loss of recent unflushed stream entries. 

### Database Failures

* Transactions are rolled back.
* Jobs are retried up to a fixed attempt limit.

### Network Partition

* Worker cannot acknowledge messages and upon recovery, messages are reprocessed.

### Dead Letter Queue (DLQ)

* Jobs exceeding the retry limit are sent to a separate Redis Stream.
* DLQ entries include failure reason and payload for debugging.

### Summary of Failure Outcomes
| Failure Scenario            | Data Loss          | Duplication |
| --------------------------- | ------------------ | ----------- |
| Ingestion crash pre-enqueue | Possible           | No          |
| Worker crash pre-ACK        | No                 | Possible    |
| Redis failover              | Possible (bounded) | No          |
| PostgreSQL outage           | No                 | Possible    |
| Network partition           | No                 | Possible    |

---

## 7. Backpressure

### Policy 
| Condition                       | Action                        |
| ------------------------------- | ----------------------------- |
| depth < soft_limit              | Accept                        |
| soft_limit ≤ depth < hard_limit | 202 Accepted + warning header |
| depth ≥ hard_limit              | **429 Too Many Requests**     |

### Overload Signaling 
* Workers do not signal ingestion directly
* Queue depth is the overload signal

### Catch-up Strategy
When backlog grows:
* Workers continue consuming at max concurrency
* Autoscaler logs scale-up recommendation
* Processing windows absorb burst
* Ingestion throttles new data

---

## 8. Observability

### Metrics
Each service exposes a `/metrics` endpoint with:

* Request throughput
* Processing latency (end-to-end tracking)
* Queue depth metrics
* Retry and DLQ counts
* Error rates

### Logging
* Structured JSON logs
* Includes job_id, worker_id, attempt, and latency

### Tracing (Optional)
* Ingestion → enqueue → processing → persistence spans

---

## 9. Storage Design

### Schema
```sql
CREATE TABLE aggregated_logs (
  service_name TEXT NOT NULL,
  log_level TEXT NOT NULL,
  time_bucket TIMESTAMP NOT NULL,
  count INT NOT NULL,
  PRIMARY KEY (service_name, log_level, time_bucket)
);
```

### Write Semantics
* Batched upserts per worker
* Single transaction per batch
* ACK after commit

### Indexing & Partitioning
* Primary key supports efficient upserts
* Schema is compatible with future time-based partitioning

---

## 10. Non-Goals & Tradeoffs

The following features are explicitly out of scope:
* Exactly-once delivery
* Schema registry
* Raw log storage or search
* Kafka or RabbitMQ
* Automatic horizontal scaling
* Custom WAL (AOF/RDB)

These decisions were made to keep the system focused, understandable, and implementable within a limited timeframe (2 weeks).

---

## 11. Load Testing

A synthetic log generator is used to:
* Produce controlled log traffic
* Simulate burst loads
* Inject malformed events
* Validate backpressure and retry behavior

---

## 12. Running the System

The system is fully containerized using Docker Compose.

```
docker-compose up --build
```

Services:

* Ingestion API
* Worker service
* Redis
* PostgreSQL

Persistent volumes are used for Redis and PostgreSQL state.

---

## 13. Stream Retention & Cleanup
Redis Streams retain messages even after acknowledgment. The ingestion stream uses approximate size-based trimming (MAXLEN ~) to bound memory usage. Dead-letter queues are not automatically trimmed and are treated as audit logs for failed events.

---

## 14. Scalability
The ingestion service is stateless and designed for horizontal scaling. In this project, a single instance is deployed using Docker Compose. In production, multiple instances could be deployed behind a load balancer.

---

## 15. Deployment
Only the ingestion service will be deployed via GCP Cloud Run.

In production, Redis Streams and PostgreSQL would be provisioned as managed services (e.g., GCP Memorystore and Cloud SQL). For this project, both services are run locally via Docker Compose to intentionally keep infrastructure complexity low and focus on system design and correctness.

Worker processes are also run locally, as their deployment is not essential for demonstrating the system’s architecture. In a production environment, workers would be deployed as separate stateless services with concurrency controlled via environment configuration, for example as an additional Cloud Run service. This separation allows ingestion and processing throughput to be scaled independently in production.

---

## 16. Future Improvements
* Node Registry (TODO)
* Stream Partitioning (TODO)
* Nginx Load balancer (TODO)
* Redis Sentinel HA (TODO)
* CI/CD with testcontainers-go (TODO)
* Circuit breaker pattern (DONE) ie, if service is down stop calling it 
---
* Partitioned workload (maybe) 
* Protobuf-based ingestion 
* Schema registry 
* Multi-Pipeline fan-out
* Kafka-backed broker
* Time-partitioned tables
* Horizontal worker autoscaling
* Exactly-once delivery

---

## 17. Testing
run docker compose -f docker-compose.test.yml up -d
INTEGRATION_TESTS=true go test -v ./tests/integration -run TestHappyPath -v (single test)
INTEGRATION_TESTS=true go test ./tests/integration/... -v -timeout 120s

--- 

## 18. Design Trade-offs
* SetNX deduplication to avoid false positives and reduce complexity
* Short TTL to prevent high memory costs

* Bloom filter is memory bound with O(1) access
* Bloom filter will be implemented if scale demands 

--- 

## 19. Redis Conf
* maxmemory-policy volatile-ttl to prevent eviction of stream or message keys
* could allow for duplication to slip through if dedup keys are dropped due to memory pressure


Integration (In-Process):           
✅ Happy path (queue flow)          
✅ Job timeout                      
✅ Job failure + retries            
✅ Max retries → DLQ               
✅ Redis failure      
✅ Concurrency limits
✅ In-flight drainage & shutdown (cancel context)
✅ Reclaim
✅ Config loading

E2E (Real Binaries):
✅ HTTP request → response
✅ Health endpoints
✅ Nginx rate limiting (later)
✅ Multiple replicas (later)
✅ SIGTERM graceful shutdown


Instructions:
docker-compose -f docker-compose.test.yml up -d
INTEGRATION_TESTS=true go test -v ./tests/integration/...
