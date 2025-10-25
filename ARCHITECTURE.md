# Proxy Checker Service - Architecture

## System Overview

```
┌─────────────────────────────────────────────────────────────────────┐
│                         PROXY CHECKER SERVICE                        │
├─────────────────────────────────────────────────────────────────────┤
│                                                                       │
│  ┌──────────────┐      ┌──────────────────────────────────────┐    │
│  │  HTTP API    │      │     Aggregator (Goroutine)           │    │
│  │              │      │  - Fetch sources every N seconds      │    │
│  │ /get-proxy   │      │  - Dedupe                             │    │
│  │ /stat        │      │  - Trigger checker                    │    │
│  │ /health      │      └──────────────┬───────────────────────┘    │
│  │ /metrics     │                     │                             │
│  │ /reload      │                     ▼                             │
│  └──────┬───────┘      ┌──────────────────────────────────────┐    │
│         │              │     Proxy Checker Engine              │    │
│         │              │                                        │    │
│         │              │  Process Manager (4 workers)          │    │
│         │              │  ┌──────────┐ ┌──────────┐            │    │
│         │              │  │Worker 1  │ │Worker 2  │            │    │
│         │              │  │5k conc   │ │5k conc   │            │    │
│         │              │  └──────────┘ └──────────┘            │    │
│         │              │  ┌──────────┐ ┌──────────┐            │    │
│         │              │  │Worker 3  │ │Worker 4  │            │    │
│         │              │  │5k conc   │ │5k conc   │            │    │
│         │              │  └──────────┘ └──────────┘            │    │
│         │              │                                        │    │
│         │              │  = 20k total concurrent checks         │    │
│         │              └──────────────┬─────────────────────────┘    │
│         │                             │                             │
│         │                             ▼                             │
│         │              ┌──────────────────────────────────────┐    │
│         └─────────────▶│   Atomic Snapshot (RWMutex)          │    │
│                        │                                       │    │
│                        │  - Live proxies map                   │    │
│                        │  - Stats counters                     │    │
│                        │  - Atomic pointer swap                │    │
│                        └──────────────┬────────────────────────┘    │
│                                       │                             │
│                                       ▼                             │
│                        ┌──────────────────────────────────────┐    │
│                        │   Persistence Layer                   │    │
│                        │   - SQLite / File / Redis             │    │
│                        └───────────────────────────────────────┘    │
│                                                                      │
└──────────────────────────────────────────────────────────────────────┘

         ▲                                          │
         │                                          │
    API Requests                              Prometheus
    (rate-limited)                              Scraper
```

## Concurrency Model: Why Go + Goroutines

**Choice: Go with goroutines + netpoll**

### Justification for 12-thread, 10k-25k concurrency:

1. **Go Runtime Advantages:**
   - M:N threading (goroutines on OS threads)
   - Built-in netpoll for non-blocking I/O
   - Automatic work stealing across P's (logical processors)
   - Goroutines are cheap: ~2KB stack vs 1-2MB for OS threads
   - Can easily spawn 20k+ goroutines on 12 threads

2. **Architecture:**
   ```
   GOMAXPROCS=12 → 12 OS threads (P's)
   Each goroutine → ~2-8KB memory
   20,000 goroutines → ~40-160MB for stacks (manageable)
   ```

3. **Multi-Process Design:**
   - 4 worker processes × 5k concurrent checks each = 20k total
   - Each process: GOMAXPROCS=3 (3×4=12 threads utilized)
   - Alternative: 1 process with 20k goroutines (simpler, preferred)
   - Coordination via shared memory or IPC not needed if single-process

4. **Event Loop per Goroutine:**
   ```
   for each proxy in batch:
       spawn goroutine:
           - non-blocking TCP connect (netpoll)
           - HTTP request with timeout
           - record result
           - semaphore release
   ```

5. **Connection Pooling:**
   - http.Transport with MaxIdleConns=20000
   - MaxConnsPerHost=100
   - IdleConnTimeout=90s
   - DisableKeepAlives=false

## File Descriptor Budget

**Target: 20,000 concurrent checks**

Estimate per connection:
- 1 socket fd
- ~1-2 internal fds (Go runtime)
- **Total: ~2 fds per active check**

Calculation:
```
20,000 concurrent × 2 fds = 40,000 fds
+ 1,000 for API server, logs, etc.
= 41,000 fds required

Recommended: ulimit -n 65535 (with margin)
```

## Memory & CPU Projections

### 10k Concurrent Checks:
- Goroutines: 10,000 × 4KB = 40MB
- Buffers: 10,000 × 8KB = 80MB
- Results cache: ~5MB
- **Total: ~125MB + Go runtime (~50MB) = ~175MB**
- **CPU: 4-6 cores at 60-80% utilization**

### 20k Concurrent Checks:
- Goroutines: 20,000 × 4KB = 80MB
- Buffers: 20,000 × 8KB = 160MB
- Results cache: ~10MB
- **Total: ~250MB + runtime = ~300MB**
- **CPU: 8-10 cores at 70-90% utilization**

### 25k Concurrent Checks:
- Goroutines: 25,000 × 4KB = 100MB
- Buffers: 25,000 × 8KB = 200MB
- Results cache: ~12MB
- **Total: ~312MB + runtime = ~360MB**
- **CPU: 10-12 cores at 80-95% utilization**

## Atomic Snapshot Strategy

```go
type ProxySnapshot struct {
    Proxies      []Proxy
    Stats        Stats
    LastUpdated  time.Time
}

var (
    currentSnapshot atomic.Value  // stores *ProxySnapshot
    snapshotMu      sync.RWMutex  // fallback protection
)

// Writer (checker) side:
func updateSnapshot(newProxies []Proxy, stats Stats) {
    snapshot := &ProxySnapshot{
        Proxies:     newProxies,
        Stats:       stats,
        LastUpdated: time.Now(),
    }
    currentSnapshot.Store(snapshot)
    persistToDisk(snapshot)  // async
}

// Reader (API) side:
func getSnapshot() *ProxySnapshot {
    return currentSnapshot.Load().(*ProxySnapshot)
}
```

## Backpressure & Safety

1. **Semaphore-based concurrency limiting:**
   ```go
   sem := make(chan struct{}, maxConcurrency)
   for _, proxy := range proxies {
       sem <- struct{}{}  // acquire
       go func(p Proxy) {
           defer func() { <-sem }()  // release
           checkProxy(p)
       }(proxy)
   }
   ```

2. **Adaptive concurrency:**
   - Monitor: open file descriptors (`/proc/self/fd`)
   - If FDs > 80% of ulimit: reduce concurrency by 20%
   - If CPU > 95% for 30s: reduce concurrency by 10%

3. **Graceful degradation:**
   - Context with timeout for entire scan cycle (max 5 minutes)
   - Cancel all goroutines on shutdown signal
   - No goroutine leaks: always defer cleanup

4. **Retry with backoff:**
   ```go
   for retry := 0; retry <= maxRetries; retry++ {
       if err := checkProxy(proxy); err == nil {
           break
       }
       time.Sleep(time.Duration(retry) * 100 * time.Millisecond)
   }
   ```

## Batching Strategy

```
Total proxies: 100,000
Batch size: 2,000
Concurrency per batch: 20,000

Process:
1. Split 100k into 50 batches of 2k each
2. Process 10 batches concurrently (20k checks active)
3. As batches complete, start next batch
4. Total time: ~15-30 seconds for 100k proxies
```

## TCP Tuning Requirements

```bash
# Increase local port range
sysctl -w net.ipv4.ip_local_port_range="10000 65535"

# Increase max syn backlog
sysctl -w net.ipv4.tcp_max_syn_backlog=8192

# Enable TCP Fast Open
sysctl -w net.ipv4.tcp_fastopen=3

# Reduce TIME_WAIT sockets
sysctl -w net.ipv4.tcp_fin_timeout=15
sysctl -w net.ipv4.tcp_tw_reuse=1

# Increase connection tracking
sysctl -w net.netfilter.nf_conntrack_max=200000
sysctl -w net.nf_conntrack_max=200000
```

## Scaling Path

Current: 12 threads, 20k concurrent
- **Vertical:** Upgrade to 24-32 threads → 40k-50k concurrent
- **Horizontal:** Run 2-3 instances behind load balancer → 60k+ concurrent
- **Hybrid:** Use Redis for shared snapshot, multiple API nodes

## Why Not Other Options?

**Node.js cluster:**
- Single-threaded event loop, need 12 worker processes
- V8 memory overhead per process (~50MB × 12)
- Less efficient goroutine-like concurrency

**Rust + tokio:**
- Excellent choice, comparable performance
- More complex error handling
- Longer development time
- Go preferred for simpler, faster development

**Python asyncio:**
- GIL limits multi-core usage
- Slower per-connection overhead
- Not suitable for 20k+ concurrent on 12 threads

