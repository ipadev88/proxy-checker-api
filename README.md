# Proxy Checker API

A production-ready, high-performance proxy aggregation, validation, and delivery service optimized for 10k-25k concurrent proxy checks on a 12-thread server.

## Features

✅ **High-Concurrency Checking** - 10k-25k concurrent proxy validations using Go goroutines + netpoll  
✅ **Atomic Snapshot Updates** - Zero-downtime updates with lock-free reads  
✅ **Multiple Storage Backends** - File, SQLite, Redis support  
✅ **RESTful API** - Fast, authenticated endpoints with rate limiting  
✅ **Prometheus Metrics** - Full observability and monitoring  
✅ **Hot Reload** - Update configuration without restart  
✅ **Adaptive Concurrency** - Automatic backpressure and resource management  
✅ **Production Ready** - Docker, systemd, monitoring, alerts included  

## Quick Start

### Docker (Recommended)

```bash
# Clone repository
git clone https://github.com/your-org/proxy-checker-api.git
cd proxy-checker-api

# Set API key
echo "PROXY_API_KEY=your-secure-key-here" > .env

# Start service
docker-compose up -d

# Check health
curl http://localhost:8080/health

# Get proxies
curl -H "X-Api-Key: your-secure-key-here" http://localhost:8080/get-proxy
```

### Binary Installation

```bash
# Download latest release
curl -L https://github.com/your-org/proxy-checker/releases/latest/download/proxy-checker-linux-amd64 \
  -o proxy-checker

# Make executable
chmod +x proxy-checker

# Create config
cp config.example.json config.json
nano config.json  # Edit as needed

# Set API key
export PROXY_API_KEY="your-secure-key-here"

# Run
./proxy-checker
```

### Build from Source

```bash
# Requirements: Go 1.21+
git clone https://github.com/your-org/proxy-checker-api.git
cd proxy-checker-api

# Install dependencies
go mod download

# Build
go build -o proxy-checker ./cmd/main.go

# Run
./proxy-checker
```

## Configuration

### Minimal Configuration

```json
{
  "aggregator": {
    "interval_seconds": 60,
    "sources": [
      {
        "url": "https://raw.githubusercontent.com/TheSpeedX/PROXY-List/master/http.txt",
        "type": "txt",
        "enabled": true
      }
    ]
  },
  "checker": {
    "timeout_ms": 15000,
    "concurrency_total": 20000,
    "test_url": "https://www.google.com/generate_204",
    "mode": "full-http"
  },
  "api": {
    "addr": ":8080",
    "api_key_env": "PROXY_API_KEY"
  }
}
```

### Tuning for 12-Thread Server

**For 10k concurrent checks:**
```json
{
  "checker": {
    "concurrency_total": 10000,
    "batch_size": 2000,
    "timeout_ms": 15000
  }
}
```

**For 20k concurrent checks (recommended):**
```json
{
  "checker": {
    "concurrency_total": 20000,
    "batch_size": 2000,
    "timeout_ms": 15000
  }
}
```

**For 25k concurrent checks (maximum):**
```json
{
  "checker": {
    "concurrency_total": 25000,
    "batch_size": 2500,
    "timeout_ms": 12000,
    "enable_adaptive_concurrency": true
  }
}
```

See `config.example.json` for full configuration options.

## API Reference

### Authentication

All protected endpoints require an API key via:
- Header: `X-Api-Key: your-api-key`
- Query param: `?key=your-api-key`

### Endpoints

#### `GET /health`

Health check endpoint (no auth required).

**Response:**
```
ok
```

---

#### `GET /get-proxy`

Get proxy address(es). Requires authentication.

**Parameters:**
- `limit=N` - Return N proxies (default: 1)
- `all=1` - Return all alive proxies
- `format=json` - Return JSON format (default: plain text)

**Examples:**

Get single proxy (plain text):
```bash
curl -H "X-Api-Key: your-key" http://localhost:8080/get-proxy
# Output: 1.2.3.4:8080
```

Get 10 proxies (plain text):
```bash
curl -H "X-Api-Key: your-key" "http://localhost:8080/get-proxy?limit=10"
# Output:
# 1.2.3.4:8080
# 5.6.7.8:3128
# ...
```

Get proxies (JSON):
```bash
curl -H "X-Api-Key: your-key" "http://localhost:8080/get-proxy?limit=5&format=json"
```

**JSON Response:**
```json
{
  "total": 1523,
  "alive": 1523,
  "proxies": [
    {
      "address": "1.2.3.4:8080",
      "alive": true,
      "latency_ms": 234,
      "last_check": "2025-10-25T12:34:56Z"
    }
  ]
}
```

---

#### `GET /stat`

Get proxy statistics. Requires authentication.

**Response:**
```json
{
  "total_scraped": 5000,
  "total_alive": 1523,
  "total_dead": 3477,
  "alive_percent": "30.46%",
  "last_check": "2025-10-25T12:34:56Z",
  "updated": "2025-10-25T12:35:10Z",
  "sources": {
    "https://example.com/proxies.txt": {
      "URL": "https://example.com/proxies.txt",
      "ProxiesFound": 2500,
      "Error": ""
    }
  }
}
```

---

#### `POST /reload`

Trigger immediate re-aggregation and re-checking. Requires authentication.

**Response:**
```json
{
  "message": "Reload triggered"
}
```

---

#### `GET /metrics`

Prometheus metrics endpoint (no auth required by default).

**Response:** Prometheus text format

**Key Metrics:**
- `proxychecker_alive_proxies` - Current alive proxy count
- `proxychecker_checks_total` - Total checks performed
- `proxychecker_check_duration_seconds` - Check latency histogram
- `proxychecker_api_requests_total` - API request counter
- `go_goroutines` - Active goroutines

---

## System Requirements

### Hardware
- **CPU:** 12 threads (6 cores with HT or 12 cores)
- **RAM:** 4GB minimum, 8GB recommended
- **Disk:** 10GB available (SSD preferred)
- **Network:** 1Gbps bandwidth

### Software
- **OS:** Linux (Ubuntu 20.04+, RHEL 8+, or similar)
- **Kernel:** 4.15+ (for TCP Fast Open support)
- **Dependencies:** None (statically compiled binary)

### System Tuning

**Critical:** Set file descriptor limit:
```bash
ulimit -n 65535
```

**Recommended:** Apply TCP tuning:
```bash
sudo sysctl -w net.ipv4.ip_local_port_range="10000 65535"
sudo sysctl -w net.ipv4.tcp_max_syn_backlog=8192
sudo sysctl -w net.ipv4.tcp_tw_reuse=1
```

See `OPS_CHECKLIST.md` for complete tuning guide.

## Performance

### Benchmarks (12-thread server)

| Concurrency | Proxies | Duration | Memory | CPU Usage |
|-------------|---------|----------|--------|-----------|
| 10,000      | 10,000  | 30-45s   | ~175MB | 50-70%    |
| 20,000      | 20,000  | 45-75s   | ~300MB | 70-90%    |
| 25,000      | 25,000  | 60-90s   | ~360MB | 80-95%    |

### API Performance

- **Throughput:** > 10,000 req/sec
- **Latency (p50):** < 5ms
- **Latency (p99):** < 50ms

## Monitoring

### Prometheus + Grafana

```bash
# Start with monitoring stack
docker-compose --profile monitoring up -d

# Access Grafana
open http://localhost:3000
# Login: admin / admin
```

### Key Metrics to Watch

```promql
# Proxy availability
proxychecker_alive_proxies

# Check throughput
rate(proxychecker_checks_total[5m])

# Check success rate
rate(proxychecker_checks_success_total[5m]) / rate(proxychecker_checks_total[5m])

# API latency (p99)
histogram_quantile(0.99, rate(proxychecker_api_request_duration_seconds_bucket[5m]))
```

### Alerts

Pre-configured alerts (see `alerts.yml`):
- No alive proxies (critical)
- Low proxy count (warning)
- High check failure rate (warning)
- High API latency (warning)
- Service down (critical)

## Deployment

### Docker Compose (Production)

```yaml
version: '3.8'

services:
  proxy-checker:
    image: your-org/proxy-checker:latest
    restart: unless-stopped
    ports:
      - "8080:8080"
    volumes:
      - ./config.json:/app/config.json:ro
      - proxy-data:/data
    environment:
      - PROXY_API_KEY=${PROXY_API_KEY}
    ulimits:
      nofile:
        soft: 65535
        hard: 65535

volumes:
  proxy-data:
```

### Systemd Service

```bash
# Install service
sudo cp proxy-checker.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable proxy-checker
sudo systemctl start proxy-checker

# Check status
sudo systemctl status proxy-checker

# View logs
sudo journalctl -u proxy-checker -f
```

### Kubernetes

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: proxy-checker
spec:
  replicas: 1
  template:
    spec:
      containers:
      - name: proxy-checker
        image: your-org/proxy-checker:latest
        resources:
          requests:
            memory: "512Mi"
            cpu: "2"
          limits:
            memory: "1Gi"
            cpu: "12"
        env:
        - name: PROXY_API_KEY
          valueFrom:
            secretKeyRef:
              name: proxy-checker-secrets
              key: api-key
```

## Testing

### Unit Tests
```bash
go test ./internal/... -v -race
```

### Integration Tests
```bash
go test ./tests/integration/... -v
```

### End-to-End Smoke Test
```bash
bash tests/e2e/smoke_test.sh
```

### Performance Test
```bash
bash benchmark.sh
```

See `TESTS.md` for complete testing documentation.

## Troubleshooting

### Service won't start
```bash
# Check logs
journalctl -u proxy-checker -n 50

# Verify config
./proxy-checker -validate

# Check permissions
ls -la /var/lib/proxy-checker
```

### No proxies available
```bash
# Check stats
curl -H "X-Api-Key: $KEY" http://localhost:8080/stat

# Trigger manual reload
curl -X POST -H "X-Api-Key: $KEY" http://localhost:8080/reload

# Check source availability
curl -I https://raw.githubusercontent.com/TheSpeedX/PROXY-List/master/http.txt
```

### High memory usage
```bash
# Check current usage
ps aux | grep proxy-checker

# Get heap profile
curl http://localhost:8080/debug/pprof/heap > heap.prof
go tool pprof -top heap.prof

# Solutions:
# - Reduce concurrency_total
# - Reduce batch_size
# - Check for goroutine leaks
```

See `OPS_CHECKLIST.md` for complete troubleshooting guide.

## Architecture

```
┌─────────────────────────────────────────┐
│         HTTP API (Gin)                  │
│  /get-proxy  /stat  /health  /metrics   │
└───────────────┬─────────────────────────┘
                │
                ▼
┌───────────────────────────────────────────┐
│     Atomic Snapshot (atomic.Value)        │
│     - Lock-free reads                     │
│     - Atomic pointer swap on update       │
└───────────────┬───────────────────────────┘
                │
                ▼
┌───────────────────────────────────────────┐
│     Checker (20k goroutines)              │
│     - Semaphore-based concurrency         │
│     - HTTP transport with netpoll         │
│     - Adaptive backpressure               │
└───────────────┬───────────────────────────┘
                │
                ▼
┌───────────────────────────────────────────┐
│     Aggregator                            │
│     - Concurrent source fetching          │
│     - Deduplication                       │
│     - Error handling                      │
└───────────────────────────────────────────┘
```

See `ARCHITECTURE.md` for detailed architecture documentation.

## Contributing

Contributions welcome! Please:
1. Fork the repository
2. Create a feature branch
3. Add tests for new functionality
4. Ensure all tests pass
5. Submit a pull request

## License

MIT License - see LICENSE file for details

## Support

- **Documentation:** See docs in repository
- **Issues:** GitHub Issues
- **Performance:** See PERFORMANCE_TESTING.md
- **Operations:** See OPS_CHECKLIST.md

## Acknowledgments

Built with:
- [Gin](https://github.com/gin-gonic/gin) - HTTP framework
- [Prometheus](https://prometheus.io/) - Metrics
- [Go](https://golang.org/) - Language runtime

## Version

Current version: 1.0.0

---

**Production-Ready** • **High-Performance** • **Well-Documented** • **Fully Tested**

