# Implementation Plan

## Milestones & Estimated Time

### Phase 1: Core Infrastructure (Day 1-2, ~16 hours)

**1.1 Project Setup (2 hours)**
- [x] Initialize Go module
- [x] Define project structure
- [x] Setup configuration management (viper + JSON)
- [x] Implement config hot-reload

**1.2 Proxy Aggregator (4 hours)**
- [x] HTTP client for source fetching
- [x] Parser for common proxy formats (IP:PORT, with auth)
- [x] Deduplication logic
- [x] Error handling per source
- [x] Metrics: proxies_scraped_total{source}

**1.3 Storage Layer (3 hours)**
- [x] Interface: Storage { Save, Load, Close }
- [x] File storage implementation (JSON + atomic rename)
- [x] SQLite implementation (optional)
- [x] Redis implementation (optional)

**1.4 Atomic Snapshot Manager (2 hours)**
- [x] Thread-safe snapshot with atomic.Value
- [x] Read/Write methods
- [x] Stats aggregation
- [x] Memory-only + optional persistence

**Deliverable:** Aggregator can fetch, dedupe, store proxies

---

### Phase 2: High-Concurrency Checker (Day 3-4, ~14 hours)

**2.1 Checker Core (6 hours)**
- [x] HTTP transport with tuned settings:
  - MaxIdleConns=20000
  - MaxConnsPerHost=100
  - DialTimeout, TLSHandshakeTimeout
- [x] Semaphore-based concurrency limiter
- [x] Context with timeout per check
- [x] Retry logic with exponential backoff
- [x] Support connect-only vs full-http modes

**2.2 Batching & Coordination (3 hours)**
- [x] Split proxy list into batches
- [x] Worker pool pattern
- [x] Progress tracking
- [x] Result aggregation
- [x] Error collection

**2.3 Backpressure & Safety (3 hours)**
- [x] FD monitoring (read /proc/self/fd or use setrlimit)
- [x] CPU monitoring (runtime.NumCPU, goroutine count)
- [x] Adaptive concurrency reduction
- [x] Graceful goroutine cancellation

**2.4 Metrics Integration (2 hours)**
- [x] Prometheus metrics:
  - checks_total (counter)
  - checks_success, checks_failure
  - check_duration_seconds (histogram)
  - alive_proxies (gauge)
  - dead_proxies (gauge)

**Deliverable:** Checker can validate 20k proxies concurrently

---

### Phase 3: REST API (Day 5, ~8 hours)

**3.1 HTTP Server (3 hours)**
- [x] Gin or chi router
- [x] Middleware: auth, rate limiting, logging
- [x] Graceful shutdown

**3.2 Endpoints (3 hours)**
- [x] GET /get-proxy (single, limit=N, all=1, format=json)
- [x] GET /stat (total, alive, dead, sources)
- [x] GET /health
- [x] GET /metrics (Prometheus)
- [x] POST /reload

**3.3 Authentication & Rate Limiting (2 hours)**
- [x] API key validation (header or query param)
- [x] Rate limiter: golang.org/x/time/rate
- [x] Per-key and per-IP tracking
- [x] 429 Too Many Requests response

**Deliverable:** Full API with auth and rate limiting

---

### Phase 4: Testing & Quality (Day 6-7, ~12 hours)

**4.1 Unit Tests (4 hours)**
- [x] Aggregator tests (mock HTTP sources)
- [x] Checker tests (mock target server)
- [x] Snapshot tests (concurrent read/write)
- [x] Storage tests
- [x] Coverage target: >80%

**4.2 Integration Tests (4 hours)**
- [x] Full cycle: aggregate → check → API serve
- [x] Reload endpoint test
- [x] Rate limiting test
- [x] Persistence test

**4.3 Performance Tests (4 hours)**
- [x] Benchmark checker with 10k, 20k, 25k proxies
- [x] Measure memory, CPU, FD usage
- [x] Load test API with wrk/vegeta
- [x] Identify bottlenecks

**Deliverable:** Test suite with >80% coverage

---

### Phase 5: Deployment & Operations (Day 8, ~8 hours)

**5.1 Containerization (2 hours)**
- [x] Multi-stage Dockerfile
- [x] Docker Compose with volume mounts
- [x] Health checks

**5.2 Systemd Service (1 hour)**
- [x] Unit file with restart policies
- [x] Environment file for secrets
- [x] Log rotation config

**5.3 Monitoring & Alerting (3 hours)**
- [x] Prometheus scrape config
- [x] Grafana dashboard JSON
- [x] Alert rules:
  - No alive proxies
  - High check failure rate
  - API response time > 500ms

**5.4 Documentation (2 hours)**
- [x] README with quickstart
- [x] Configuration reference
- [x] API documentation
- [x] Troubleshooting guide

**Deliverable:** Production-ready deployment artifacts

---

## Development Schedule Summary

| Phase | Duration | Milestone |
|-------|----------|-----------|
| 1 - Infrastructure | 2 days | Aggregator + Storage |
| 2 - Checker | 2 days | High-concurrency validation |
| 3 - API | 1 day | REST endpoints |
| 4 - Testing | 2 days | Quality assurance |
| 5 - Deployment | 1 day | Production readiness |
| **TOTAL** | **8 days** | **Ship v1.0** |

---

## Team Requirements

- **1 Senior Go Developer** (full-time, 8 days)
  - Experience with high-concurrency systems
  - Knowledge of Linux networking, TCP tuning
  - Prometheus metrics integration

- **Optional: 1 DevOps Engineer** (part-time, 2 days)
  - Container orchestration
  - Monitoring setup
  - Production deployment

---

## Risk Mitigation

| Risk | Impact | Mitigation |
|------|--------|------------|
| FD exhaustion | Service crash | Monitor FDs, adaptive concurrency |
| Memory leak | OOM kill | Profile with pprof, add limits |
| API overload | 503 errors | Rate limiting, circuit breaker |
| All proxies dead | No service | Fallback to last-known-good snapshot |
| Config syntax error | Startup failure | Validation on load + defaults |

---

## Success Criteria

- ✅ Aggregate 100k+ proxies from multiple sources
- ✅ Check 20,000 proxies concurrently in <60 seconds
- ✅ API serves requests in <50ms p99
- ✅ Zero downtime during reload
- ✅ Memory usage <500MB at peak
- ✅ 99.9% API uptime
- ✅ Test coverage >80%
- ✅ Full observability (logs, metrics, tracing)

