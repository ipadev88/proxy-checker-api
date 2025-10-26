# Zmap Integration Summary

## 📋 Executive Summary

This document summarizes the plan to integrate **zmap network scanning** into your existing proxy-checker-api service, enabling discovery of 50k-200k proxy candidates per scan cycle (10-20x improvement over scraping alone).

**Key Benefits:**
- 🚀 **10-20x more working proxies** (from ~1.3k to 10k-20k)
- ⚡ **Active discovery** via network scanning (not just scraping)
- 🔧 **Reusable components** from Zmap-ProxyScanner
- 🎯 **Backward compatible** (zmap is opt-in)
- 📊 **Full observability** (Prometheus metrics, Grafana dashboards)
- ⚖️ **Legal safeguards** (blacklists, rate limits, opt-out)

---

## 🔍 Zmap-ProxyScanner Analysis

### What It Does
- **Reads IPs from stdin** (piped from zmap), files, or HTTP URLs
- **Validates proxies** by making HTTP requests through them
- **Concurrent checking** with 2000 threads (configurable)
- **Outputs** working proxies to file

### Key Reusable Components
1. ✅ **stdin scanner pattern** (`bufio_scanner.go`) - Read zmap output line-by-line
2. ✅ **Queue architecture** (`queue.go`) - Producer-consumer with channels
3. ✅ **HTTP validation** (`http.go`) - Proxy verification logic
4. ✅ **Concurrency control** (atomic counters) - Thread-safe statistics
5. ✅ **File exporter** (`exporter.go`) - Incremental output writer

### How Zmap Is Used
```bash
# Zmap pipes IPs to scanner
zmap -p 8080 | ./ZmapProxyScanner -p 8080 -o proxies.txt

# Scanner:
# 1. Reads IP per line from stdin
# 2. Appends :PORT (from -p flag)
# 3. Tests by making HTTP GET through proxy
# 4. Writes working proxies to output file
```

### Shortcomings (vs. Our Service)
- ❌ No structured logging (uses `fmt.Printf`)
- ❌ No Prometheus metrics
- ❌ Only checks `StatusCode == 200` (should accept 2xx/204)
- ❌ No adaptive concurrency
- ❌ Global config (not thread-safe reload)
- ❌ No REST API

---

## 🏗️ Integration Architecture

### High-Level Flow

```
┌─────────────────────────────────────────────────────────────────┐
│                     Aggregation Cycle (Hourly)                  │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  ┌──────────────┐      ┌──────────────┐      ┌──────────────┐ │
│  │   Scraper    │      │     Zmap     │      │  Fast Filter │ │
│  │   (URLs)     │─────▶│   Scanner    │─────▶│  (TCP only)  │ │
│  │  45 sources  │      │ Ports: 8080  │      │   2s timeout │ │
│  │  ~90k proxies│      │  3128, 80    │      │  50k conc.   │ │
│  └──────────────┘      │ ~150k cand.  │      └──────────────┘ │
│                        └──────────────┘             │          │
│                                                      ▼          │
│                                              ┌──────────────┐  │
│                                              │ Full HTTP    │  │
│                                              │ Checker      │  │
│                                              │ 6s timeout   │  │
│                                              │ 20k conc.    │  │
│                                              └──────────────┘  │
│                                                      │          │
│                                                      ▼          │
│                                              ┌──────────────┐  │
│                                              │   Snapshot   │  │
│                                              │   Update     │  │
│                                              │ (atomic swap)│  │
│                                              └──────────────┘  │
│                                                      │          │
└──────────────────────────────────────────────────────┼──────────┘
                                                       ▼
                                                  ┌─────────┐
                                                  │   API   │
                                                  │ Serve   │
                                                  └─────────┘
```

### Phase Breakdown

**PHASE 1: Scraping (Existing)**
- Fetch from 45 HTTP sources
- Parse IP:PORT format
- Deduplicate
- **Output:** ~90k unique proxies

**PHASE 2: Zmap Scanning (NEW)**
- Run `zmap -p 8080` → CSV output
- Parse: `saddr,sport,timestamp`
- Convert to `IP:PORT` format
- Repeat for ports 3128, 80, 1080, etc.
- **Output:** ~150k candidate proxies

**PHASE 3: Merge & Deduplicate**
- Combine scraped + zmap lists
- Deduplicate by `IP:PORT`
- **Output:** ~200k unique candidates

**PHASE 4: Two-Stage Checking (OPTIMIZED)**
- **Stage 1 - Fast Filter:**
  - TCP connect-only (no HTTP)
  - 2s timeout, 50k concurrency
  - Filters out ~80% dead proxies in <15s
  - **Output:** ~40k connectable proxies
  
- **Stage 2 - Full HTTP:**
  - HTTP GET through proxy
  - 6s timeout, 20k concurrency
  - Tests actual proxy functionality
  - **Output:** ~10k working proxies

**PHASE 5: Snapshot Update**
- Atomic swap of alive proxy list
- Update statistics
- Persist to storage
- Expose via API

---

## 📁 File Changes Required

### New Files to Create (8)

| File | Purpose | LOC |
|------|---------|-----|
| `internal/zmap/scanner.go` | Core zmap integration, exec & parsing | 300 |
| `internal/zmap/capabilities.go` | Check CAP_NET_RAW, root access | 80 |
| `internal/zmap/zgrab.go` | Optional zgrab2 integration | 100 |
| `internal/zmap/safety.go` | Blacklist loading, target validation | 120 |
| `internal/checker/fastfilter.go` | TCP-only pre-filter | 150 |
| `docs/LEGAL_SCANNING.md` | Legal warnings & best practices | N/A |
| `scripts/install_zmap.sh` | Automated zmap installation | 100 |
| `scripts/zmap_smoke_test.sh` | E2E validation script | 80 |

### Files to Modify (6)

| File | Changes | Complexity |
|------|---------|------------|
| `internal/config/config.go` | Add `ZmapConfig` struct + validation | Medium |
| `cmd/main.go` | Add zmap to aggregation loop | Medium |
| `internal/api/server.go` | Add `/stats/zmap`, mode to `/reload` | Low |
| `internal/metrics/metrics.go` | Add zmap Prometheus metrics | Low |
| `Dockerfile` | Install zmap, set capabilities | High |
| `docker-compose.yml` | Add `cap_add: [NET_RAW, NET_ADMIN]` | Low |

---

## ⚙️ Configuration Example

```json
{
  "zmap": {
    "enabled": true,
    "ports": [8080, 3128, 80, 1080, 8888],
    "rate_limit": 10000,
    "bandwidth": "10M",
    "max_runtime_seconds": 7200,
    "target_ranges": ["1.0.0.0/8", "2.0.0.0/8"],
    "blacklist": ["/etc/proxy-checker/blacklist.txt"],
    "zmap_binary": "/usr/local/bin/zmap",
    "cooldown_seconds": 3600
  },
  "checker": {
    "enable_fast_filter": true,
    "fast_filter_timeout_ms": 2000,
    "fast_filter_concurrency": 50000
  }
}
```

---

## 🚀 Performance Expectations

### Current (Scraping Only)
- **Sources:** 45 URLs
- **Scraped:** ~90k proxies
- **Alive:** ~1.3k proxies
- **Cycle Time:** 45 seconds
- **Check Speed:** 3,606 checks/sec

### With Zmap Integration
- **Sources:** 45 URLs + Zmap (5 ports)
- **Candidates:** ~200k-300k proxies
- **Alive:** ~10k-20k proxies (10-20x improvement)
- **Zmap Scan:** 20-30 minutes
- **Fast Filter:** <15 seconds
- **Full Check:** ~60 seconds
- **Total Cycle:** ~35-40 minutes
- **Check Speed:** 3k-5k checks/sec (maintained)

### Bottlenecks
- **Zmap scan duration:** Depends on `rate_limit` and IP range size
  - 10k pps = ~5 days for full IPv4
  - Practical: Scan targeted ranges in 20-30 min
- **Validation:** With fast filter, can handle 200k candidates in <2 minutes

---

## 🔐 Security & Legal

### ⚠️ Critical Requirements

1. **Capabilities:** Zmap requires `CAP_NET_RAW` + `CAP_NET_ADMIN`
   ```bash
   sudo setcap 'cap_net_raw,cap_net_admin=+ep' /usr/local/bin/zmap
   ```

2. **Legal Authorization:** MUST have permission to scan networks
   - Only scan your own networks or with written authorization
   - Scanning without permission may violate CFAA (US) or Computer Misuse Act (UK)

3. **Blacklists:** Mandatory exclusion of:
   - Private ranges (10.0.0.0/8, 192.168.0.0/16, 172.16.0.0/12)
   - Government (.mil, .gov)
   - Healthcare, finance, critical infrastructure

4. **Rate Limiting:** Conservative defaults
   - Default: 10k pps
   - Recommended: 5k pps for safety
   - Cooldown: 1 hour between scans

5. **Opt-Out Mechanism:**
   - Maintain `optout.txt` blacklist
   - Honor requests within 24 hours
   - Set up `abuse@yourdomain.com`

### Docker Capabilities

```yaml
# docker-compose.yml
services:
  proxy-checker:
    cap_add:
      - NET_RAW
      - NET_ADMIN
    # Alternative: privileged: true (not recommended)
```

### Systemd Capabilities

```ini
# proxy-checker.service
[Service]
AmbientCapabilities=CAP_NET_RAW CAP_NET_ADMIN
CapabilityBoundingSet=CAP_NET_RAW CAP_NET_ADMIN
```

---

## 📊 API Changes

### New Endpoints

**GET `/stats/zmap`** - Zmap statistics
```json
{
  "enabled": true,
  "ports": [8080, 3128, 80],
  "last_scan_time": "2024-10-25T12:00:00Z",
  "last_scan_duration": 1234.5,
  "candidates_found": 50000,
  "total_scans": 42
}
```

**POST `/reload`** - Enhanced with mode parameter
```json
{
  "mode": "scrape-only" | "zmap-only" | "both"
}
```

### Existing Endpoints (Unchanged)
- `GET /get-proxy?limit=10` - Same behavior
- `GET /stat` - Includes zmap statistics
- `GET /health` - Same
- `GET /metrics` - Adds zmap metrics

---

## 🧪 Testing Strategy

### Unit Tests
- ✅ Config parsing & validation
- ✅ Zmap CSV/JSON parsing
- ✅ Command building (flags verification)
- ✅ Blacklist loading
- ✅ Fast filter logic

### Integration Tests
- ✅ Full zmap scan cycle (localhost)
- ✅ Timeout handling
- ✅ Error recovery
- ✅ Metrics recording

### Smoke Test
```bash
# scripts/zmap_smoke_test.sh
python3 -m http.server 8888 &
sudo zmap -p 8888 -r 1000 127.0.0.0/24 -o /tmp/test.csv
grep -q "127.0.0.1" /tmp/test.csv && echo "✅ Pass"
```

### Performance Test
- Load test: 200k candidates
- Target: <2 minutes total (fast filter + full check)
- Monitor: goroutines, memory, FDs

---

## 📦 Installation Steps

### System Requirements
- **OS:** Linux (Ubuntu 20.04+ recommended)
- **zmap:** >= 2.1.0
- **zgrab2:** >= 0.1.0 (optional)
- **Capabilities:** Root or CAP_NET_RAW
- **Resources:** 12+ CPU cores, 8GB RAM, 1Gbps network

### Quick Install (Ubuntu)

```bash
# 1. Install zmap
sudo apt update
sudo apt install -y zmap libpcap-dev
zmap --version

# 2. Install zgrab2 (optional)
go install github.com/zmap/zgrab2@latest
sudo cp ~/go/bin/zgrab2 /usr/local/bin/

# 3. Download blacklist
sudo mkdir -p /etc/proxy-checker
sudo wget -O /etc/proxy-checker/blacklist.txt \
  https://raw.githubusercontent.com/zmap/zmap/master/conf/blacklist.conf

# 4. Build proxy-checker
cd /path/to/proxy-checker-api
go build -o proxy-checker ./cmd/main.go

# 5. Set capabilities
sudo setcap 'cap_net_raw,cap_net_admin=+ep' ./proxy-checker

# 6. Update config
vim config.json
# Set zmap.enabled = true

# 7. Run
./proxy-checker -config config.json
```

### Docker Install

```bash
# 1. Update docker-compose.yml (add cap_add)
# 2. Build
docker-compose build

# 3. Run
docker-compose up -d

# 4. Verify
docker exec -it proxy-checker zmap --version
docker logs -f proxy-checker
```

---

## 🎯 Zmap Command Examples

### Basic Scan
```bash
sudo zmap -p 8080 \
  -r 10000 \
  -o /tmp/proxies_8080.csv \
  --output-fields=saddr,sport,timestamp-str \
  --output-module=csv
```

### Multiple Ports
```bash
for PORT in 8080 3128 80; do
  sudo zmap -p $PORT \
    -o /tmp/proxies_$PORT.csv \
    --output-fields=saddr \
    --output-module=csv \
    -r 10000 \
    -T 1800
done
```

### With Blacklist
```bash
sudo zmap -p 8080 \
  -o /tmp/proxies.csv \
  -r 10000 \
  -b /etc/proxy-checker/blacklist.txt \
  1.0.0.0/8 2.0.0.0/8
```

### With Bandwidth Limit
```bash
sudo zmap -p 8080 \
  -B 10M \
  -r 10000 \
  -o /tmp/proxies.csv
```

---

## 📈 Monitoring

### Key Metrics

**Zmap:**
- `zmap_scans_total{port, status}` - Counter of scans
- `zmap_candidates_found{port}` - Gauge of candidates per port
- `zmap_scan_duration_seconds{port}` - Histogram of scan times
- `zmap_errors_total{type}` - Counter of errors

**Checker:**
- `proxychecker_checks_total` - Total checks performed
- `proxychecker_checks_success` - Successful checks
- `proxychecker_checks_failed` - Failed checks
- `proxychecker_alive_proxies` - Current alive count

### Grafana Dashboard

New panels to add:
- **Zmap Scans** - Counter (scans/hour)
- **Candidates by Port** - Gauge (multi-series)
- **Zmap Duration** - Heatmap
- **Scrape vs Zmap** - Pie chart (contribution %)
- **Error Rate** - Graph

### Alerts

```yaml
# alerts.yml
- alert: ZmapScanFailed
  expr: increase(zmap_scans_total{status="error"}[5m]) > 0
  for: 5m
  
- alert: ZmapNoResults
  expr: zmap_candidates_found == 0
  for: 30m
```

---

## 🔄 Workflow Recommendations

### Development (Scrape-Only)
```json
{
  "aggregator": { "interval_seconds": 60 },
  "zmap": { "enabled": false }
}
```
- Fast iterations (~1 min cycles)
- No zmap complexity
- Good for testing

### Production (Hybrid)
```json
{
  "aggregator": { "interval_seconds": 3600 },
  "zmap": {
    "enabled": true,
    "rate_limit": 10000,
    "cooldown_seconds": 3600
  }
}
```
- Hourly cycles
- Zmap runs once per cycle
- Balanced scrape + scan

### Aggressive (Zmap-Heavy)
```json
{
  "aggregator": { "interval_seconds": 7200 },
  "zmap": {
    "enabled": true,
    "rate_limit": 25000,
    "max_runtime_seconds": 7200,
    "cooldown_seconds": 1800
  }
}
```
- 2-hour cycles
- Faster zmap (25k pps)
- Maximum proxy discovery
- **Use with caution - legal implications**

---

## ⏱️ Implementation Estimate

| Phase | Tasks | Hours | Priority |
|-------|-------|-------|----------|
| Config & Types | Add ZmapConfig, validation | 2 | High |
| Zmap Module | scanner.go, capabilities.go | 6 | High |
| Fast Filter | fastfilter.go implementation | 2 | Medium |
| Integration | Update cmd/main.go orchestration | 3 | High |
| API Updates | /stats/zmap, mode parameter | 1 | Low |
| Metrics | Prometheus + Grafana | 2 | Medium |
| Docker | Dockerfile, capabilities setup | 3 | High |
| Testing | Unit + integration tests | 4 | High |
| Docs | Legal, README updates | 2 | Medium |

**Total Estimated Time:** 20-25 hours

---

## ✅ Next Steps

1. **Review Legal Considerations** → Read `docs/LEGAL_SCANNING.md`
2. **Install Dependencies** → Run `scripts/install_zmap.sh`
3. **Run Smoke Test** → Verify zmap works: `sudo bash scripts/zmap_smoke_test.sh`
4. **Update Config** → Enable zmap with conservative settings
5. **Implement Core Module** → Start with `internal/zmap/scanner.go`
6. **Add Tests** → Unit tests for parsing, integration tests for full cycle
7. **Deploy to Staging** → Test in isolated environment
8. **Monitor Metrics** → Verify performance targets met
9. **Legal Review** → Get authorization for scan ranges
10. **Production Deploy** → Gradual rollout with monitoring

---

## 📚 Resources

- **Zmap Documentation:** https://github.com/zmap/zmap
- **Zgrab2 Documentation:** https://github.com/zmap/zgrab2
- **Legal Best Practices:** https://github.com/zmap/zmap/wiki/Scanning-Best-Practices
- **CFAA Overview:** https://www.eff.org/issues/cfaa
- **Integration Plan (JSON):** `ZMAP_INTEGRATION_PLAN.json`

---

## 🤝 Support

Questions or issues? Create an issue on GitHub or contact:
- **Email:** abuse@yourdomain.com
- **Docs:** `docs/LEGAL_SCANNING.md`
- **Troubleshooting:** `TROUBLESHOOTING.md`

---

**Generated:** October 26, 2025  
**Version:** 1.0  
**Status:** Ready for Implementation

