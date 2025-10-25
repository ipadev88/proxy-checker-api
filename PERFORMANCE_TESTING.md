# Performance Testing Plan

## Overview

This document outlines comprehensive performance testing strategies for the Proxy Checker Service optimized for 10k-25k concurrent proxy checks on a 12-thread server.

## Test Environment

### Hardware Requirements
- **CPU:** 12 threads (6 cores with HT or 12 cores)
- **RAM:** 4GB minimum, 8GB recommended
- **Network:** 1Gbps or better
- **Storage:** SSD for low-latency disk I/O

### System Tuning

Before testing, apply these optimizations:

```bash
# Increase file descriptor limit
ulimit -n 65535

# Kernel tuning
sudo sysctl -w net.ipv4.ip_local_port_range="10000 65535"
sudo sysctl -w net.ipv4.tcp_max_syn_backlog=8192
sudo sysctl -w net.ipv4.tcp_fastopen=3
sudo sysctl -w net.ipv4.tcp_fin_timeout=15
sudo sysctl -w net.ipv4.tcp_tw_reuse=1
sudo sysctl -w net.core.somaxconn=8192
sudo sysctl -w net.netfilter.nf_conntrack_max=200000

# Make permanent in /etc/sysctl.conf
cat >> /etc/sysctl.conf <<EOF
net.ipv4.ip_local_port_range = 10000 65535
net.ipv4.tcp_max_syn_backlog = 8192
net.ipv4.tcp_fastopen = 3
net.ipv4.tcp_fin_timeout = 15
net.ipv4.tcp_tw_reuse = 1
net.core.somaxconn = 8192
net.netfilter.nf_conntrack_max = 200000
EOF
sudo sysctl -p
```

## Test Categories

### 1. Checker Performance Tests

#### Test 1.1: Baseline Concurrency (10k)

**Objective:** Validate 10k concurrent proxy checks complete within 60 seconds

**Setup:**
```json
{
  "checker": {
    "timeout_ms": 15000,
    "concurrency_total": 10000,
    "batch_size": 2000,
    "retries": 1,
    "mode": "full-http"
  }
}
```

**Test Data:** 10,000 test proxies (mix of alive/dead)

**Expected Results:**
- Completion time: 30-60 seconds
- Memory usage: < 200MB
- CPU usage: 50-70%
- Open file descriptors: < 12,000
- Success rate: Depends on proxy quality

**Measurement:**
```bash
# Monitor during test
watch -n 1 'ps aux | grep proxy-checker'
watch -n 1 'lsof -p $(pgrep proxy-checker) | wc -l'

# Check logs
journalctl -u proxy-checker -f | grep "Check complete"
```

#### Test 1.2: Target Concurrency (20k)

**Objective:** Validate 20k concurrent checks as designed

**Setup:**
```json
{
  "checker": {
    "concurrency_total": 20000,
    "batch_size": 2000
  }
}
```

**Test Data:** 20,000 test proxies

**Expected Results:**
- Completion time: 45-90 seconds
- Memory usage: < 350MB
- CPU usage: 70-90%
- Open file descriptors: < 25,000
- No crashes or goroutine leaks

#### Test 1.3: Maximum Concurrency (25k)

**Objective:** Test upper limit at 25k concurrent

**Setup:**
```json
{
  "checker": {
    "concurrency_total": 25000,
    "batch_size": 2500
  }
}
```

**Expected Results:**
- Completion time: 60-120 seconds
- Memory usage: < 400MB
- CPU usage: 80-95%
- Open file descriptors: < 30,000
- Stable operation without OOM

#### Test 1.4: Stress Test - Sustained Load

**Objective:** Run continuous checking cycles for 1 hour

**Setup:** 20k concurrency, run every 60 seconds

**Expected Results:**
- No memory leaks (stable RSS over time)
- No goroutine leaks (stable count)
- No file descriptor leaks
- Consistent performance across all cycles

**Monitoring Script:**
```bash
#!/bin/bash
# stress-monitor.sh

PID=$(pgrep proxy-checker)
LOG_FILE="stress_test_$(date +%Y%m%d_%H%M%S).log"

echo "timestamp,rss_mb,cpu_percent,goroutines,fds" > $LOG_FILE

for i in {1..3600}; do
  RSS=$(ps -o rss= -p $PID | awk '{print $1/1024}')
  CPU=$(ps -o %cpu= -p $PID)
  GOROUTINES=$(curl -s http://localhost:8080/metrics | grep go_goroutines | awk '{print $2}')
  FDS=$(lsof -p $PID | wc -l)
  
  echo "$(date +%s),$RSS,$CPU,$GOROUTINES,$FDS" >> $LOG_FILE
  sleep 1
done
```

#### Test 1.5: Mode Comparison

**Objective:** Compare connect-only vs full-http modes

**Tests:**
- Mode: connect-only, 25k concurrency
- Mode: full-http, 25k concurrency

**Expected Results:**
- Connect-only: ~50% faster, lower CPU
- Full-http: More accurate alive detection

---

### 2. API Performance Tests

#### Test 2.1: API Throughput - /get-proxy

**Tool:** wrk

**Command:**
```bash
wrk -t12 -c400 -d60s --latency \
  -H "X-Api-Key: your-api-key" \
  http://localhost:8080/get-proxy
```

**Expected Results:**
- Requests/sec: > 10,000
- Latency p50: < 5ms
- Latency p99: < 50ms
- No 503 errors (assuming proxies available)

#### Test 2.2: API Throughput - /get-proxy?limit=10

**Command:**
```bash
wrk -t12 -c400 -d60s --latency \
  -H "X-Api-Key: your-api-key" \
  "http://localhost:8080/get-proxy?limit=10"
```

**Expected Results:**
- Requests/sec: > 8,000
- Latency p50: < 10ms
- Latency p99: < 100ms

#### Test 2.3: Rate Limiting Test

**Tool:** vegeta

**Command:**
```bash
echo "GET http://localhost:8080/get-proxy" | \
  vegeta attack -header "X-Api-Key: your-api-key" \
  -rate=1500/1m -duration=60s | \
  vegeta report
```

**Expected Results:**
- First 1200 req/min: 200 OK
- Excess requests: 429 Too Many Requests
- No service degradation

#### Test 2.4: Concurrent API Clients

**Objective:** Simulate 100 concurrent API consumers

**Tool:** Custom Go test script

```go
// test/api_load_test.go
package main

import (
    "fmt"
    "net/http"
    "sync"
    "time"
)

func main() {
    clients := 100
    requestsPerClient := 1000
    
    var wg sync.WaitGroup
    success := 0
    failure := 0
    var mu sync.Mutex
    
    start := time.Now()
    
    for i := 0; i < clients; i++ {
        wg.Add(1)
        go func(id int) {
            defer wg.Done()
            
            client := &http.Client{Timeout: 5 * time.Second}
            
            for j := 0; j < requestsPerClient; j++ {
                req, _ := http.NewRequest("GET", "http://localhost:8080/get-proxy", nil)
                req.Header.Set("X-Api-Key", "your-api-key")
                
                resp, err := client.Do(req)
                if err == nil && resp.StatusCode == 200 {
                    mu.Lock()
                    success++
                    mu.Unlock()
                    resp.Body.Close()
                } else {
                    mu.Lock()
                    failure++
                    mu.Unlock()
                }
            }
        }(i)
    }
    
    wg.Wait()
    duration := time.Since(start)
    
    total := clients * requestsPerClient
    fmt.Printf("Total: %d, Success: %d, Failure: %d\n", total, success, failure)
    fmt.Printf("Duration: %v, RPS: %.2f\n", duration, float64(total)/duration.Seconds())
}
```

**Expected Results:**
- 100k total requests complete in < 30 seconds
- Success rate: > 99%
- RPS: > 3,000

---

### 3. End-to-End Integration Tests

#### Test 3.1: Full Cycle Test

**Objective:** Test complete workflow from aggregation to API delivery

**Steps:**
1. Start service with empty state
2. Trigger aggregation (or wait for scheduled run)
3. Wait for check cycle to complete
4. Query API for proxies
5. Verify metrics are updated

**Validation:**
```bash
# Wait for first cycle
sleep 120

# Check stats
curl -H "X-Api-Key: your-api-key" http://localhost:8080/stat | jq

# Get proxies
curl -H "X-Api-Key: your-api-key" http://localhost:8080/get-proxy?limit=5

# Check metrics
curl http://localhost:8080/metrics | grep proxychecker
```

**Expected Results:**
- Stats show non-zero values
- Proxies returned successfully
- Metrics reflect actual operations

#### Test 3.2: Reload Test

**Objective:** Validate hot reload doesn't cause downtime

**Script:**
```bash
#!/bin/bash
# Test API availability during reload

# Start background API requests
while true; do
  curl -s -H "X-Api-Key: your-api-key" \
    http://localhost:8080/get-proxy > /dev/null
  echo -n "."
  sleep 0.1
done &
BG_PID=$!

# Trigger reload
sleep 5
curl -X POST -H "X-Api-Key: your-api-key" \
  http://localhost:8080/reload

# Wait for reload to complete
sleep 90

# Stop background requests
kill $BG_PID

echo "Reload test complete - check for any gaps in dots above"
```

**Expected Results:**
- No errors during reload
- API remains responsive
- Continuous dots output (no gaps)

#### Test 3.3: Persistence Test

**Objective:** Validate snapshot persistence and recovery

**Steps:**
```bash
# 1. Start service and wait for check cycle
docker-compose up -d
sleep 120

# 2. Get current proxy count
BEFORE=$(curl -s -H "X-Api-Key: changeme123" \
  http://localhost:8080/stat | jq '.total_alive')
echo "Before: $BEFORE proxies"

# 3. Stop service
docker-compose stop

# 4. Restart service
docker-compose start

# 5. Immediately check if proxies are available
sleep 5
AFTER=$(curl -s -H "X-Api-Key: changeme123" \
  http://localhost:8080/stat | jq '.total_alive')
echo "After: $AFTER proxies"

# 6. Verify counts match (allowing for stale proxy filtering)
if [ "$AFTER" -ge $((BEFORE * 80 / 100)) ]; then
  echo "✓ Persistence test PASSED"
else
  echo "✗ Persistence test FAILED"
fi
```

---

### 4. Resource Usage Tests

#### Test 4.1: Memory Profiling

**Objective:** Identify memory leaks and optimize allocations

**Tool:** Go pprof

**Steps:**
```bash
# Add pprof endpoint (already included in production)
import _ "net/http/pprof"

# Capture heap profile during load
go tool pprof http://localhost:8080/debug/pprof/heap

# Commands in pprof:
# > top10          # Show top memory consumers
# > list funcName  # Show code with allocations
# > web            # Generate graph
```

**Expected Results:**
- No continuously growing allocations
- Heap size stabilizes after first cycle
- Most memory in goroutine stacks and HTTP buffers

#### Test 4.2: CPU Profiling

**Command:**
```bash
# Capture 30-second CPU profile during check cycle
curl http://localhost:8080/debug/pprof/profile?seconds=30 > cpu.prof

# Analyze
go tool pprof cpu.prof
# > top20
# > list checkProxy
```

**Expected Results:**
- Most CPU time in network I/O (syscall)
- Efficient context switching
- No hot loops or busy-waiting

#### Test 4.3: Goroutine Leak Detection

**Monitoring:**
```bash
# Before check cycle
BEFORE=$(curl -s http://localhost:8080/debug/pprof/goroutine | grep goroutine | wc -l)

# After multiple cycles
sleep 300
AFTER=$(curl -s http://localhost:8080/debug/pprof/goroutine | grep goroutine | wc -l)

echo "Before: $BEFORE, After: $AFTER"
# Should be roughly equal (±50)
```

---

### 5. Failure Mode Tests

#### Test 5.1: No Proxies Available

**Scenario:** All source URLs return empty or fail

**Expected Behavior:**
- Service continues running
- `/get-proxy` returns 503
- `/health` still returns 200
- Logs warning messages
- Next cycle attempts re-aggregation

#### Test 5.2: All Proxies Dead

**Scenario:** All aggregated proxies fail checks

**Expected Behavior:**
- Service completes check cycle
- Stats show 0% alive
- `/get-proxy` returns 503
- Metrics updated correctly

#### Test 5.3: File Descriptor Exhaustion

**Simulation:**
```bash
# Temporarily lower ulimit
ulimit -n 1024

# Start service with high concurrency
# Should trigger adaptive concurrency reduction
```

**Expected Behavior:**
- Service detects high FD usage
- Reduces concurrency automatically
- Logs warning
- Continues operating (degraded mode)

#### Test 5.4: Network Timeout Storm

**Scenario:** All target URLs timeout simultaneously

**Expected Behavior:**
- Check cycle completes (with failures)
- Doesn't hang indefinitely
- Respects timeout_ms setting
- All goroutines properly cleaned up

#### Test 5.5: Graceful Shutdown Under Load

**Test:**
```bash
# Start service
./proxy-checker &
PID=$!

# Wait for check cycle to start
sleep 10

# Send SIGTERM
kill -TERM $PID

# Observe logs
```

**Expected Behavior:**
- Catches signal
- Cancels in-flight checks gracefully
- Persists current snapshot
- Shuts down within 30 seconds
- No goroutine panics

---

## Test Tools & Scripts

### Mock Proxy Server

For controlled testing, use a mock proxy server:

```go
// test/mock_proxy_server.go
package main

import (
    "fmt"
    "math/rand"
    "net/http"
    "time"
)

func main() {
    // Simulate proxy with configurable behavior
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        // Random latency 10-500ms
        delay := time.Duration(10+rand.Intn(490)) * time.Millisecond
        time.Sleep(delay)
        
        // 70% success rate
        if rand.Float32() < 0.7 {
            w.WriteHeader(http.StatusOK)
            fmt.Fprintf(w, "OK")
        } else {
            w.WriteHeader(http.StatusServiceUnavailable)
        }
    })
    
    // Start multiple proxy instances on different ports
    for port := 8081; port < 8181; port++ {
        go http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
    }
    
    select {} // Block forever
}
```

### Test Proxy Generator

Generate realistic test proxy lists:

```bash
#!/bin/bash
# generate_test_proxies.sh

COUNT=${1:-10000}
OUTPUT="test_proxies.txt"

> $OUTPUT

for i in $(seq 1 $COUNT); do
  IP="$((RANDOM % 256)).$((RANDOM % 256)).$((RANDOM % 256)).$((RANDOM % 256))"
  PORT=$((8000 + RANDOM % 2000))
  echo "$IP:$PORT" >> $OUTPUT
done

echo "Generated $COUNT proxies in $OUTPUT"
```

### Benchmark Script

Comprehensive benchmark runner:

```bash
#!/bin/bash
# benchmark.sh

set -e

echo "=== Proxy Checker Performance Benchmark ==="
echo "Started: $(date)"
echo ""

# Configuration
CONCURRENCY_LEVELS=(5000 10000 15000 20000 25000)
RESULTS_DIR="benchmark_results_$(date +%Y%m%d_%H%M%S)"
mkdir -p $RESULTS_DIR

for CONC in "${CONCURRENCY_LEVELS[@]}"; do
  echo "Testing concurrency: $CONC"
  
  # Update config
  jq ".checker.concurrency_total = $CONC" config.json > config.tmp.json
  mv config.tmp.json config.json
  
  # Restart service
  docker-compose restart proxy-checker
  sleep 10
  
  # Wait for one complete cycle
  sleep 120
  
  # Capture metrics
  curl -s http://localhost:8080/metrics > "$RESULTS_DIR/metrics_${CONC}.txt"
  curl -s http://localhost:8080/stat > "$RESULTS_DIR/stats_${CONC}.json"
  
  # Get resource usage
  docker stats --no-stream proxy-checker > "$RESULTS_DIR/resources_${CONC}.txt"
  
  echo "  ✓ Results saved"
done

echo ""
echo "Benchmark complete. Results in: $RESULTS_DIR"
```

---

## Performance Targets Summary

| Metric | Target | Critical Threshold |
|--------|--------|-------------------|
| Concurrent Checks | 20,000 | 10,000 |
| Check Cycle Duration | < 90s | < 180s |
| Memory Usage (RSS) | < 350MB | < 500MB |
| CPU Usage | 70-90% | < 95% |
| Open File Descriptors | < 25,000 | < 50,000 |
| API Latency (p99) | < 50ms | < 200ms |
| API Throughput | > 10k RPS | > 5k RPS |
| Goroutines (steady state) | < 500 | < 1000 |
| Zero Downtime Reload | ✓ | ✓ |

---

## Backoff Thresholds

Trigger adaptive concurrency reduction when:

| Resource | Warning Threshold | Action |
|----------|------------------|--------|
| File Descriptors | > 80% of ulimit | Reduce concurrency by 20% |
| CPU Usage | > 95% for 30s | Reduce concurrency by 10% |
| Memory | > 450MB RSS | Reduce batch size |
| Goroutines | > 30k | Delay batch processing |
| Network Errors | > 50% failure rate | Increase timeout |

---

## Continuous Performance Monitoring

### Daily Performance Test

Run automated tests daily:

```bash
# crontab entry
0 2 * * * /opt/proxy-checker/scripts/daily_perf_test.sh
```

### Key Metrics to Track

1. **Throughput:** Checks per second
2. **Latency:** Check duration histogram
3. **Resource Usage:** Memory, CPU, FDs over time
4. **Error Rates:** Failed checks, API errors
5. **API Performance:** Response times, throughput

### Dashboard Queries (Prometheus)

```promql
# Checks per second
rate(proxychecker_checks_total[5m])

# P99 check latency
histogram_quantile(0.99, rate(proxychecker_check_duration_seconds_bucket[5m]))

# Memory trend
process_resident_memory_bytes{job="proxy-checker"}

# API throughput
rate(proxychecker_api_requests_total[1m])
```

---

## Troubleshooting Performance Issues

### Issue: Low Throughput

**Symptoms:** Fewer than 100 checks/sec

**Investigation:**
```bash
# Check goroutine count
curl http://localhost:8080/debug/pprof/goroutine?debug=1

# Check for blocking
go tool pprof http://localhost:8080/debug/pprof/block

# Network connectivity
ss -s | grep TCP
```

**Solutions:**
- Increase concurrency_total
- Check network bandwidth
- Verify DNS resolution is fast
- Increase timeout_ms if many timeouts

### Issue: High Memory Usage

**Symptoms:** RSS > 500MB

**Investigation:**
```bash
# Heap profile
go tool pprof -alloc_space http://localhost:8080/debug/pprof/heap
```

**Solutions:**
- Reduce concurrency_total
- Reduce batch_size
- Check for goroutine leaks
- Verify snapshot is being updated (not accumulating)

### Issue: API Latency Spikes

**Symptoms:** p99 > 200ms

**Investigation:**
- Check if check cycle is running (mutex contention)
- Verify snapshot size
- Check CPU usage

**Solutions:**
- Use atomic.Value for lock-free reads
- Pre-allocate slices
- Add caching layer for frequently requested proxies

---

## Report Template

After each performance test, document:

```markdown
## Performance Test Report

**Date:** YYYY-MM-DD
**Version:** vX.Y.Z
**Tester:** Name

### Environment
- CPU: 12 threads
- RAM: 8GB
- OS: Linux 5.x
- Network: 1Gbps

### Configuration
- concurrency_total: 20000
- timeout_ms: 15000
- mode: full-http

### Results

| Metric | Value | Target | Status |
|--------|-------|--------|--------|
| Check Cycle Duration | 75s | <90s | ✓ PASS |
| Memory Usage | 320MB | <350MB | ✓ PASS |
| CPU Usage | 85% | 70-90% | ✓ PASS |
| API Latency p99 | 45ms | <50ms | ✓ PASS |

### Issues Found
- None

### Recommendations
- Current configuration optimal for 20k concurrency
- Can safely increase to 25k if needed
```

---

## Conclusion

This comprehensive testing plan ensures the Proxy Checker Service meets all performance requirements for high-concurrency operation. Execute tests in order, document results, and establish baseline metrics for production monitoring.

