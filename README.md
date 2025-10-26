# Proxy Checker API

<div align="center">

**A production-ready, high-performance proxy aggregation, validation, and delivery service**

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?logo=go)](https://golang.org/)
[![Docker](https://img.shields.io/badge/Docker-Ready-2496ED?logo=docker)](https://www.docker.com/)
[![Protocols](https://img.shields.io/badge/Protocols-HTTP%20%7C%20SOCKS4%20%7C%20SOCKS5-green)](https://github.com/ipadev88/proxy-checker-api)

*Optimized for 10k-25k concurrent proxy checks with zmap network scanning on a 12-thread server*

[Quick Start](#quick-start) • [Features](#features) • [API Usage](#api-reference) • [Documentation](#documentation)

</div>

---

## ✨ Features

- ⚡ **High-Concurrency Checking** - 10k-25k concurrent proxy validations using Go goroutines + netpoll
- 🎯 **Multi-Protocol Support** - HTTP, HTTPS, SOCKS4, and SOCKS5 proxies
- 🔍 **Zmap Network Scanning** - High-speed port scanning to discover new proxies (10k-50k candidates/scan)
- 🚀 **Fast TCP Filter** - Quick connect-only filtering before full validation
- 🔄 **Atomic Snapshot Updates** - Zero-downtime updates with lock-free reads
- 💾 **Multiple Storage Backends** - File, SQLite, Redis support
- 🌐 **RESTful API** - Fast, authenticated endpoints with protocol filtering and rate limiting
- 📊 **Prometheus Metrics** - Full observability and monitoring
- 🔥 **Hot Reload** - Update configuration without restart
- 🎯 **Adaptive Concurrency** - Automatic backpressure and resource management
- 🚀 **Production Ready** - Docker, systemd, monitoring, alerts included

---

## 🚀 Quick Start

### Ubuntu Server (Automated - Recommended)

**One-command setup that fixes all common issues including docker-compose errors:**

```bash
# Clone repository
git clone https://github.com/ipadev88/proxy-checker-api.git
cd proxy-checker-api

# Run automated setup
sudo bash setup-ubuntu.sh
```

**What the script does:**
- ✅ Installs Docker if needed
- ✅ Fixes docker-compose compatibility issues
- ✅ Installs and configures zmap with capabilities (CAP_NET_RAW, CAP_NET_ADMIN)
- ✅ Downloads blacklist for safe network scanning
- ✅ Applies system tuning (file descriptors, TCP settings, network parameters)
- ✅ Creates configuration files with SOCKS4/SOCKS5 support
- ✅ Generates secure API key
- ✅ Starts all services (proxy-checker, Redis, Prometheus, Grafana)
- ✅ Displays test commands

**What you get out of the box:**
- ✅ HTTP/HTTPS proxy checking
- ✅ SOCKS4 proxy checking
- ✅ SOCKS5 proxy checking
- ✅ Zmap network scanning on ports: 80, 8080, 3128 (HTTP), 1080 (SOCKS5), 1081 (SOCKS4)
- ✅ 45+ proxy sources pre-configured
- ✅ Automatic aggregation every 60 seconds
- ✅ Grafana dashboards for monitoring

### Docker (Manual Setup)

```bash
# Clone and navigate
git clone https://github.com/ipadev88/proxy-checker-api.git
cd proxy-checker-api

# Copy configuration
cp config.example.json config.json

# Generate API key
echo "PROXY_API_KEY=$(openssl rand -hex 16)" > .env

# Start service (note: use 'docker compose' with space, not hyphen)
docker compose up -d

# Check health
curl http://localhost:8083/health

# Get your API key
API_KEY=$(grep PROXY_API_KEY .env | cut -d= -f2)

# Wait 1-2 minutes for first proxy check, then test
curl -H "X-Api-Key: $API_KEY" http://localhost:8083/stat | jq
```

**⚠️ Common Issue:** Getting `URLSchemeUnknown: Not supported URL scheme http+docker` error?  
→ See **[TROUBLESHOOTING.md](TROUBLESHOOTING.md)** or run `sudo bash setup-ubuntu.sh`

### Binary Installation

```bash
# Download latest release
curl -L https://github.com/ipadev88/proxy-checker/releases/latest/download/proxy-checker-linux-amd64 \
  -o proxy-checker

# Make executable
chmod +x proxy-checker

# Create config
cp config.example.json config.json

# Set API key
export PROXY_API_KEY="your-secure-key-here"

# Run
./proxy-checker
```

### Build from Source

```bash
# Requirements: Go 1.21+
git clone https://github.com/ipadev88/proxy-checker-api.git
cd proxy-checker-api

# Install dependencies
go mod download

# Build
go build -o proxy-checker ./cmd/main.go

# Run
./proxy-checker
```

---

## 📖 API Reference

### Authentication

All protected endpoints require an API key via:
- **Header:** `X-Api-Key: your-api-key`
- **Query parameter:** `?key=your-api-key`

### Endpoints

#### `GET /health`

Health check endpoint (no auth required).

```bash
curl http://localhost:8083/health
```

**Response:** `ok`

---

#### `GET /get-proxy`

Get proxy address(es). Requires authentication.

**Parameters:**
- `limit=N` - Return N proxies (default: 1)
- `all=1` - Return all alive proxies
- `format=json` - Return JSON format (default: plain text)
- `protocol=http|socks4|socks5` - Filter by protocol (optional)

**Examples:**

```bash
# Get single proxy (plain text, any protocol)
curl -H "X-Api-Key: your-key" http://localhost:8083/get-proxy

# Get 10 HTTP proxies
curl -H "X-Api-Key: your-key" "http://localhost:8083/get-proxy?limit=10&protocol=http"

# Get all SOCKS5 proxies
curl -H "X-Api-Key: your-key" "http://localhost:8083/get-proxy?all=1&protocol=socks5"

# Get 50 SOCKS4 proxies in JSON format
curl -H "X-Api-Key: your-key" "http://localhost:8083/get-proxy?limit=50&protocol=socks4&format=json" | jq
```

**Plain Text Response:**
```
1.2.3.4:8080
5.6.7.8:1080
9.10.11.12:3128
```

**JSON Response:**
```json
{
  "total": 1523,
  "alive": 1523,
  "proxies": [
    {
      "address": "1.2.3.4:8080",
      "protocol": "http",
      "alive": true,
      "latency_ms": 234,
      "last_check": "2025-10-26T12:34:56Z"
    },
    {
      "address": "5.6.7.8:1080",
      "protocol": "socks5",
      "alive": true,
      "latency_ms": 456,
      "last_check": "2025-10-26T12:34:56Z"
    }
  ]
}
```

---

#### `GET /stat`

Get proxy statistics. Requires authentication.

```bash
curl -H "X-Api-Key: your-key" http://localhost:8083/stat | jq
```

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

```bash
curl -X POST -H "X-Api-Key: your-key" http://localhost:8083/reload
```

**Response:**
```json
{
  "message": "Reload triggered"
}
```

---

#### `GET /metrics`

Prometheus metrics endpoint (no auth required by default).

```bash
curl http://localhost:8083/metrics
```

**Key Metrics:**
- `proxychecker_alive_proxies` - Current alive proxy count
- `proxychecker_checks_total` - Total checks performed
- `proxychecker_check_duration_seconds` - Check latency histogram
- `proxychecker_api_requests_total` - API request counter
- `proxychecker_zmap_scans_total` - Zmap scans performed
- `proxychecker_zmap_candidates_found` - Candidates discovered by zmap
- `go_goroutines` - Active goroutines

---

#### `GET /stats/zmap`

Get zmap scanner statistics. Requires authentication.

```bash
curl -H "X-Api-Key: your-key" http://localhost:8083/stats/zmap | jq
```

**Response:**
```json
{
  "enabled": true,
  "ports": [8080, 80, 3128, 1080, 1081],
  "last_scan_time": "2025-10-26T12:00:00Z",
  "last_scan_duration": 1834.5,
  "candidates_found": 15234,
  "total_scans": 24
}
```

---

## ⚙️ Configuration

### Minimal Configuration

```json
{
  "aggregator": {
    "interval_seconds": 60,
    "sources": [
      {
        "url": "https://raw.githubusercontent.com/TheSpeedX/PROXY-List/master/http.txt",
        "type": "txt",
        "protocol": "http",
        "enabled": true
      },
      {
        "url": "https://api.proxyscrape.com/v2/?request=get&protocol=socks5",
        "type": "txt",
        "protocol": "socks5",
        "enabled": true
      }
    ]
  },
  "zmap": {
    "enabled": true,
    "ports": [8080, 80, 3128, 1080, 1081],
    "rate_limit": 10000,
    "bandwidth": "10M"
  },
  "checker": {
    "timeout_ms": 15000,
    "concurrency_total": 20000,
    "test_url": "http://www.gstatic.com/generate_204",
    "mode": "full-http",
    "socks_enabled": true,
    "socks_timeout_ms": 15000
  },
  "api": {
    "addr": ":8083",
    "api_key_env": "PROXY_API_KEY"
  }
}
```

### Performance Tuning for 12-Thread Server

**Conservative (Low Resource Usage):**
```json
{
  "checker": {
    "concurrency_total": 10000,
    "batch_size": 1000,
    "timeout_ms": 15000
  }
}
```

**Balanced (Recommended):**
```json
{
  "checker": {
    "concurrency_total": 20000,
    "batch_size": 2000,
    "timeout_ms": 15000
  }
}
```

**Aggressive (Maximum Performance):**
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

**Hot Reload Configuration:**
```bash
# After editing config.json
curl -X POST -H "X-Api-Key: $API_KEY" http://localhost:8083/reload
```

See [config.example.json](config.example.json) for all available options.

---

## 💻 System Requirements

### Hardware

- **CPU:** 12 threads (6 cores with HT or 12 cores)
- **RAM:** 4GB minimum, 8GB recommended
- **Disk:** 10GB available (SSD preferred)
- **Network:** 1Gbps bandwidth

### Software

- **OS:** Linux (Ubuntu 20.04+, RHEL 8+, or similar)
- **Docker:** 20.10+ (for containerized deployment)
- **Go:** 1.21+ (for building from source)

### System Tuning

The setup script applies these automatically. For manual setup:

```bash
# File descriptor limit (critical)
ulimit -n 65535

# TCP tuning (recommended)
sudo sysctl -w net.ipv4.ip_local_port_range="10000 65535"
sudo sysctl -w net.ipv4.tcp_max_syn_backlog=8192
sudo sysctl -w net.ipv4.tcp_tw_reuse=1
sudo sysctl -w net.core.somaxconn=8192
```

See [OPS_CHECKLIST.md](OPS_CHECKLIST.md) for complete tuning guide.

---

## 📊 Performance

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

See [PERFORMANCE_TESTING.md](PERFORMANCE_TESTING.md) for detailed benchmarks.

---

## 📈 Monitoring

### Prometheus + Grafana

```bash
# Start with monitoring stack
docker compose --profile monitoring up -d

# Access Grafana at http://localhost:3000
# Default credentials: admin / admin
```

### Key Metrics

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

### Pre-configured Alerts

- 🔴 **Critical:** No alive proxies
- 🔴 **Critical:** Service down
- 🟡 **Warning:** Low proxy count (< 100)
- 🟡 **Warning:** High check failure rate (> 80%)
- 🟡 **Warning:** High API latency (> 100ms)

See [alerts.yml](alerts.yml) for configuration.

---

## 🐛 Troubleshooting

### Docker-Compose Error

**Error:** `URLSchemeUnknown: Not supported URL scheme http+docker`

**Quick Fix:**
```bash
sudo bash setup-ubuntu.sh
```

**Manual Fix:**
```bash
# Remove old Python-based docker-compose
sudo apt-get remove docker-compose

# Install Docker Compose Plugin v2
sudo apt-get install docker-compose-plugin

# Use 'docker compose' (with space, not hyphen)
docker compose up -d
```

📖 **See [TROUBLESHOOTING.md](TROUBLESHOOTING.md) for detailed solutions**

---

### No Proxies Available

```bash
# Wait 1-2 minutes for first check cycle
sleep 120

# Check statistics
API_KEY=$(grep PROXY_API_KEY .env | cut -d= -f2)
curl -H "X-Api-Key: $API_KEY" http://localhost:8083/stat | jq

# Trigger manual reload
curl -X POST -H "X-Api-Key: $API_KEY" http://localhost:8083/reload
```

---

### Service Won't Start

```bash
# Check logs
docker compose logs proxy-checker --tail=50

# Verify config exists
cp config.example.json config.json

# Rebuild and restart
docker compose down
docker compose build --no-cache
docker compose up -d
```

---

### High CPU/Memory Usage

Edit `config.json` to reduce concurrency:

```json
{
  "checker": {
    "concurrency_total": 10000,
    "batch_size": 1000
  }
}
```

Then restart:
```bash
docker compose restart proxy-checker
```

---

### 401 Unauthorized

```bash
# Check your API key
cat .env | grep PROXY_API_KEY

# Use it in requests
API_KEY="your-actual-key"
curl -H "X-Api-Key: $API_KEY" http://localhost:8083/stat
```

---

📖 **For complete troubleshooting, see [TROUBLESHOOTING.md](TROUBLESHOOTING.md)**

---

## 🏗️ Architecture

```
┌─────────────────────────────────────────┐
│         HTTP API (Gin)                  │
│  /get-proxy  /stat  /health  /metrics   │
└───────────────┬─────────────────────────┘
                │
                ▼
┌───────────────────────────────────────────┐
│     Atomic Snapshot (atomic.Value)        │
│     • Lock-free reads                     │
│     • Atomic pointer swap on update       │
└───────────────┬───────────────────────────┘
                │
                ▼
┌───────────────────────────────────────────┐
│     Checker (20k goroutines)              │
│     • Semaphore-based concurrency         │
│     • HTTP transport with netpoll         │
│     • Adaptive backpressure               │
└───────────────┬───────────────────────────┘
                │
                ▼
┌───────────────────────────────────────────┐
│     Aggregator                            │
│     • Concurrent source fetching          │
│     • Deduplication                       │
│     • Error handling                      │
└───────────────────────────────────────────┘
```

**Key Design Principles:**
- **Zero-downtime updates** via atomic pointer swaps
- **Lock-free reads** for maximum throughput
- **Adaptive concurrency** to prevent resource exhaustion
- **Graceful degradation** under high load
- **Observable** via Prometheus metrics

See [ARCHITECTURE.md](ARCHITECTURE.md) for detailed documentation.

---

## 🚢 Deployment

### Docker Compose (Production)

```yaml
version: '3.8'

services:
  proxy-checker:
    image: your-org/proxy-checker:latest
    restart: unless-stopped
    ports:
      - "8083:8083"
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

See [DEPLOYMENT_GUIDE.md](DEPLOYMENT_GUIDE.md) for step-by-step instructions.

---

## 🧪 Testing

### Unit Tests

```bash
go test ./internal/... -v -race
```

### Integration Tests

```bash
go test ./tests/integration/... -v
```

### End-to-End Tests

```bash
bash tests/e2e/smoke_test.sh
```

### Performance Tests

```bash
bash benchmark.sh
```

See [TESTS.md](TESTS.md) for complete testing documentation.

---

## 📚 Documentation

### 🚀 Quick Setup
- **[DEPLOYMENT_READY.md](DEPLOYMENT_READY.md)** - **START HERE!** Complete setup and deployment guide
- **[setup-ubuntu.sh](setup-ubuntu.sh)** - Automated one-command setup script

### 🎯 Protocol Support
- **[SOCKS_INTEGRATION_COMPLETE.md](SOCKS_INTEGRATION_COMPLETE.md)** - SOCKS4/SOCKS5 implementation details
- **[SOCKS_SETUP_GUIDE.md](SOCKS_SETUP_GUIDE.md)** - SOCKS configuration guide

### 🔍 Network Scanning
- **[ZMAP_INTEGRATION_SUMMARY.md](ZMAP_INTEGRATION_SUMMARY.md)** - Zmap integration overview
- **[ZMAP_QUICKSTART.md](ZMAP_QUICKSTART.md)** - Quick zmap setup guide

### 🛠️ Operations & Troubleshooting
- **[OPS_CHECKLIST.md](OPS_CHECKLIST.md)** - Production operations checklist
- **[PERFORMANCE_TESTING.md](PERFORMANCE_TESTING.md)** - Performance benchmarks

### 🏗️ Technical Details
- **[ARCHITECTURE.md](ARCHITECTURE.md)** - System architecture details
- **[IMPLEMENTATION_PLAN.md](IMPLEMENTATION_PLAN.md)** - Implementation overview
- **[TESTS.md](TESTS.md)** - Testing documentation

---

## 🤝 Contributing

Contributions are welcome! Please:

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Add tests for new functionality
4. Ensure all tests pass (`go test ./...`)
5. Commit your changes (`git commit -m 'Add amazing feature'`)
6. Push to the branch (`git push origin feature/amazing-feature`)
7. Open a Pull Request

---

## 📝 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

---

## 🙏 Acknowledgments

Built with excellent open-source tools:

- [Go](https://golang.org/) - Programming language
- [Gin](https://github.com/gin-gonic/gin) - HTTP web framework
- [Prometheus](https://prometheus.io/) - Metrics and monitoring
- [Redis](https://redis.io/) - Optional storage backend
- [Docker](https://www.docker.com/) - Containerization

---

## 📞 Support

- 🐛 **Bug Reports:** [GitHub Issues](https://github.com/yourusername/proxy-checker-api/issues)
- 💬 **Questions:** [GitHub Discussions](https://github.com/yourusername/proxy-checker-api/discussions)
- 📖 **Documentation:** See [docs](#documentation) above
- ⚡ **Quick Setup:** Run `sudo bash setup-ubuntu.sh`

---

## 📈 Version

**Current Version:** 2.0.0

**What's New in v2.0.0:**
- ✅ **SOCKS4 & SOCKS5 Support** - Full implementation with protocol detection
- ✅ **Zmap Network Scanning** - High-speed port scanning (10k-50k candidates/scan)
- ✅ **Fast TCP Filter** - Quick connect-only filtering for better performance
- ✅ **Protocol-Aware API** - Filter proxies by protocol (`?protocol=http|socks4|socks5`)
- ✅ **Port-to-Protocol Mapping** - Automatic protocol detection from zmap scans
- ✅ **Enhanced Aggregation** - Protocol detection from URLs and proxy lists
- ✅ **Automated Setup** - One-command deployment with zmap configuration
- ✅ **SOCKS Sources** - Pre-configured SOCKS4/SOCKS5 proxy sources
- ✅ **Comprehensive Documentation** - Complete guides for all features

**Previous Releases:**
- **v1.0.0** - Initial production release with HTTP support
  - Fixed docker-compose compatibility issues
  - Standardized ports on 8083
  - Added automated setup script
  - Fixed all build errors

---

<div align="center">

**Production-Ready** • **High-Performance** • **Well-Documented** • **Fully Tested**

Made with ❤️ for the proxy community

⭐ **Star this repo if you find it helpful!** ⭐

</div>
