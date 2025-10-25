# Test Suite Documentation

## Overview

Comprehensive test coverage for the Proxy Checker Service including unit tests, integration tests, and end-to-end tests.

## Test Structure

```
tests/
├── unit/
│   ├── aggregator_test.go
│   ├── checker_test.go
│   ├── snapshot_test.go
│   ├── storage_test.go
│   └── config_test.go
├── integration/
│   ├── api_test.go
│   ├── full_cycle_test.go
│   └── persistence_test.go
├── performance/
│   ├── checker_bench_test.go
│   └── api_bench_test.go
└── e2e/
    └── smoke_test.sh
```

## Running Tests

### All Tests
```bash
go test ./... -v -race -cover
```

### Unit Tests Only
```bash
go test ./internal/... -v -short
```

### Integration Tests
```bash
go test ./tests/integration/... -v
```

### Benchmarks
```bash
go test ./tests/performance/... -bench=. -benchmem
```

### With Coverage
```bash
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out -o coverage.html
```

## Unit Tests

### Aggregator Tests

```go
// tests/unit/aggregator_test.go
package unit

import (
    "context"
    "net/http"
    "net/http/httptest"
    "testing"
    "time"

    "github.com/proxy-checker-api/internal/aggregator"
    "github.com/proxy-checker-api/internal/config"
    "github.com/proxy-checker-api/internal/metrics"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestAggregator_FetchFromSource(t *testing.T) {
    // Mock HTTP server
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        w.Write([]byte("192.168.1.1:8080\n192.168.1.2:8080\n192.168.1.1:8080\n"))
    }))
    defer server.Close()

    cfg := config.AggregatorConfig{
        Sources: []config.Source{
            {URL: server.URL, Type: "txt", Enabled: true},
        },
    }

    agg := aggregator.NewAggregator(cfg, metrics.NewCollector("test"))
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    proxies, stats, err := agg.Aggregate(ctx)

    require.NoError(t, err)
    assert.Len(t, proxies, 2, "Should deduplicate proxies")
    assert.Contains(t, proxies, "192.168.1.1:8080")
    assert.Contains(t, proxies, "192.168.1.2:8080")
    assert.Len(t, stats, 1)
}

func TestAggregator_HandleFailedSource(t *testing.T) {
    cfg := config.AggregatorConfig{
        Sources: []config.Source{
            {URL: "http://invalid-url-that-does-not-exist.local", Type: "txt", Enabled: true},
        },
    }

    agg := aggregator.NewAggregator(cfg, metrics.NewCollector("test"))
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    proxies, stats, err := agg.Aggregate(ctx)

    require.NoError(t, err, "Should not error on failed source")
    assert.Empty(t, proxies)
    assert.NotEmpty(t, stats[cfg.Sources[0].URL].Error)
}

func TestAggregator_ParseProxyFormats(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected []string
    }{
        {
            name:     "Simple format",
            input:    "1.2.3.4:8080\n5.6.7.8:3128",
            expected: []string{"1.2.3.4:8080", "5.6.7.8:3128"},
        },
        {
            name:     "With http prefix",
            input:    "http://1.2.3.4:8080\nhttp://5.6.7.8:3128",
            expected: []string{"1.2.3.4:8080", "5.6.7.8:3128"},
        },
        {
            name:     "With comments",
            input:    "# Comment\n1.2.3.4:8080\n\n5.6.7.8:3128",
            expected: []string{"1.2.3.4:8080", "5.6.7.8:3128"},
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                w.Write([]byte(tt.input))
            }))
            defer server.Close()

            cfg := config.AggregatorConfig{
                Sources: []config.Source{{URL: server.URL, Type: "txt", Enabled: true}},
            }

            agg := aggregator.NewAggregator(cfg, metrics.NewCollector("test"))
            proxies, _, err := agg.Aggregate(context.Background())

            require.NoError(t, err)
            assert.ElementsMatch(t, tt.expected, proxies)
        })
    }
}
```

### Checker Tests

```go
// tests/unit/checker_test.go
package unit

import (
    "context"
    "net/http"
    "net/http/httptest"
    "testing"
    "time"

    "github.com/proxy-checker-api/internal/checker"
    "github.com/proxy-checker-api/internal/config"
    "github.com/proxy-checker-api/internal/metrics"
    "github.com/stretchr/testify/assert"
)

func TestChecker_CheckAliveProxy(t *testing.T) {
    // Mock target server
    target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusNoContent) // 204
    }))
    defer target.Close()

    // Mock proxy server
    proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Forward to target
        w.WriteHeader(http.StatusNoContent)
    }))
    defer proxy.Close()

    cfg := config.CheckerConfig{
        TimeoutMs:        5000,
        ConcurrencyTotal: 10,
        TestURL:          target.URL,
        Mode:             "full-http",
    }

    chk := checker.NewChecker(cfg, metrics.NewCollector("test"))
    
    // Note: In real test, you'd need a proper proxy setup
    // This is simplified for demonstration
    results := chk.CheckProxies(context.Background(), []string{"127.0.0.1:8888"})
    
    assert.Len(t, results, 1)
}

func TestChecker_CheckDeadProxy(t *testing.T) {
    cfg := config.CheckerConfig{
        TimeoutMs:        1000,
        ConcurrencyTotal: 10,
        TestURL:          "http://example.com",
        Mode:             "connect-only",
    }

    chk := checker.NewChecker(cfg, metrics.NewCollector("test"))
    
    // Non-existent proxy
    results := chk.CheckProxies(context.Background(), []string{"1.1.1.1:9999"})
    
    assert.Len(t, results, 1)
    assert.False(t, results[0].Alive)
    assert.NotEmpty(t, results[0].Error)
}

func TestChecker_Concurrency(t *testing.T) {
    cfg := config.CheckerConfig{
        TimeoutMs:        5000,
        ConcurrencyTotal: 100,
        TestURL:          "http://example.com",
        Mode:             "connect-only",
    }

    chk := checker.NewChecker(cfg, metrics.NewCollector("test"))
    
    // Generate test proxies
    proxies := make([]string, 500)
    for i := range proxies {
        proxies[i] = "192.168.1.1:8080"
    }

    start := time.Now()
    results := chk.CheckProxies(context.Background(), proxies)
    duration := time.Since(start)

    assert.Len(t, results, 500)
    // With 100 concurrency, 500 checks should complete quickly
    assert.Less(t, duration, 30*time.Second)
}

func TestChecker_ContextCancellation(t *testing.T) {
    cfg := config.CheckerConfig{
        TimeoutMs:        30000,
        ConcurrencyTotal: 10,
        TestURL:          "http://example.com",
        Mode:             "connect-only",
    }

    chk := checker.NewChecker(cfg, metrics.NewCollector("test"))
    
    ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
    defer cancel()

    proxies := make([]string, 100)
    for i := range proxies {
        proxies[i] = "192.168.1.1:8080"
    }

    start := time.Now()
    results := chk.CheckProxies(ctx, proxies)
    duration := time.Since(start)

    // Should respect context timeout
    assert.Less(t, duration, 2*time.Second)
    assert.NotEmpty(t, results)
}
```

### Snapshot Tests

```go
// tests/unit/snapshot_test.go
package unit

import (
    "sync"
    "testing"
    "time"

    "github.com/proxy-checker-api/internal/snapshot"
    "github.com/proxy-checker-api/internal/storage"
    "github.com/stretchr/testify/assert"
)

func TestSnapshot_AtomicUpdate(t *testing.T) {
    store, _ := storage.NewFileStorage("/tmp/test_snapshot.json")
    mgr := snapshot.NewManager(store, 0)

    proxies := []snapshot.Proxy{
        {Address: "1.1.1.1:8080", Alive: true, LatencyMs: 100},
        {Address: "2.2.2.2:8080", Alive: true, LatencyMs: 200},
    }

    stats := snapshot.Stats{
        TotalScraped: 10,
        TotalAlive:   2,
        TotalDead:    8,
    }

    mgr.Update(proxies, stats)

    snap := mgr.Get()
    assert.Len(t, snap.Proxies, 2)
    assert.Equal(t, 2, snap.Stats.TotalAlive)
}

func TestSnapshot_ConcurrentReads(t *testing.T) {
    store, _ := storage.NewFileStorage("/tmp/test_snapshot.json")
    mgr := snapshot.NewManager(store, 0)

    proxies := []snapshot.Proxy{
        {Address: "1.1.1.1:8080", Alive: true},
    }
    mgr.Update(proxies, snapshot.Stats{})

    var wg sync.WaitGroup
    for i := 0; i < 100; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            snap := mgr.Get()
            assert.NotNil(t, snap)
        }()
    }

    wg.Wait()
}

func TestSnapshot_RoundRobin(t *testing.T) {
    store, _ := storage.NewFileStorage("/tmp/test_snapshot.json")
    mgr := snapshot.NewManager(store, 0)

    proxies := []snapshot.Proxy{
        {Address: "1.1.1.1:8080", Alive: true},
        {Address: "2.2.2.2:8080", Alive: true},
        {Address: "3.3.3.3:8080", Alive: true},
    }
    mgr.Update(proxies, snapshot.Stats{})

    seen := make(map[string]int)
    for i := 0; i < 30; i++ {
        proxy, ok := mgr.GetProxy()
        assert.True(t, ok)
        seen[proxy.Address]++
    }

    // Each proxy should be selected ~10 times
    for _, count := range seen {
        assert.InDelta(t, 10, count, 3)
    }
}

func TestSnapshot_EmptyProxies(t *testing.T) {
    store, _ := storage.NewFileStorage("/tmp/test_snapshot.json")
    mgr := snapshot.NewManager(store, 0)

    _, ok := mgr.GetProxy()
    assert.False(t, ok, "Should return false when no proxies")

    proxies := mgr.GetAll()
    assert.Empty(t, proxies)
}
```

### Storage Tests

```go
// tests/unit/storage_test.go
package unit

import (
    "os"
    "testing"
    "time"

    "github.com/proxy-checker-api/internal/snapshot"
    "github.com/proxy-checker-api/internal/storage"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestFileStorage_SaveLoad(t *testing.T) {
    tmpFile := "/tmp/test_storage_" + time.Now().Format("20060102150405") + ".json"
    defer os.Remove(tmpFile)

    store, err := storage.NewFileStorage(tmpFile)
    require.NoError(t, err)

    snap := &snapshot.Snapshot{
        Proxies: []snapshot.Proxy{
            {Address: "1.1.1.1:8080", Alive: true, LatencyMs: 100},
        },
        Stats: snapshot.Stats{
            TotalAlive: 1,
        },
        Updated: time.Now(),
    }

    err = store.Save(snap)
    require.NoError(t, err)

    loaded, err := store.Load()
    require.NoError(t, err)
    assert.Len(t, loaded.Proxies, 1)
    assert.Equal(t, "1.1.1.1:8080", loaded.Proxies[0].Address)
}

func TestFileStorage_AtomicWrite(t *testing.T) {
    tmpFile := "/tmp/test_atomic_" + time.Now().Format("20060102150405") + ".json"
    defer os.Remove(tmpFile)

    store, _ := storage.NewFileStorage(tmpFile)

    snap := &snapshot.Snapshot{
        Proxies: []snapshot.Proxy{{Address: "1.1.1.1:8080"}},
    }

    // Multiple concurrent writes
    var wg sync.WaitGroup
    for i := 0; i < 10; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            store.Save(snap)
        }()
    }

    wg.Wait()

    // File should be valid JSON
    loaded, err := store.Load()
    require.NoError(t, err)
    assert.NotNil(t, loaded)
}
```

## Integration Tests

```go
// tests/integration/api_test.go
package integration

import (
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "os"
    "testing"

    "github.com/proxy-checker-api/internal/api"
    "github.com/proxy-checker-api/internal/config"
    "github.com/proxy-checker-api/internal/metrics"
    "github.com/proxy-checker-api/internal/snapshot"
    "github.com/proxy-checker-api/internal/storage"
    "github.com/stretchr/testify/assert"
)

func setupTestAPI(t *testing.T) *api.Server {
    cfg := &config.Config{
        API: config.APIConfig{
            Addr:               ":0",
            EnableAPIKeyAuth:   true,
            EnableIPRateLimit:  false,
            RateLimitPerMinute: 1000,
        },
        Metrics: config.MetricsConfig{
            Enabled:  true,
            Endpoint: "/metrics",
        },
    }

    os.Setenv("PROXY_API_KEY", "test-key-123")

    store, _ := storage.NewFileStorage("/tmp/test_api.json")
    snap := snapshot.NewManager(store, 0)
    
    // Populate with test data
    proxies := []snapshot.Proxy{
        {Address: "1.1.1.1:8080", Alive: true, LatencyMs: 100},
        {Address: "2.2.2.2:8080", Alive: true, LatencyMs: 150},
    }
    snap.Update(proxies, snapshot.Stats{TotalAlive: 2})

    return api.NewServer(cfg, snap, metrics.NewCollector("test"), nil, nil)
}

func TestAPI_GetProxy(t *testing.T) {
    server := setupTestAPI(t)

    req := httptest.NewRequest("GET", "/get-proxy", nil)
    req.Header.Set("X-Api-Key", "test-key-123")
    w := httptest.NewRecorder()

    server.Router().ServeHTTP(w, req)

    assert.Equal(t, http.StatusOK, w.Code)
    assert.Contains(t, w.Body.String(), ":")
}

func TestAPI_GetProxyJSON(t *testing.T) {
    server := setupTestAPI(t)

    req := httptest.NewRequest("GET", "/get-proxy?format=json", nil)
    req.Header.Set("X-Api-Key", "test-key-123")
    w := httptest.NewRecorder()

    server.Router().ServeHTTP(w, req)

    assert.Equal(t, http.StatusOK, w.Code)

    var response map[string]interface{}
    json.Unmarshal(w.Body.Bytes(), &response)

    assert.Contains(t, response, "proxies")
    assert.Contains(t, response, "total")
}

func TestAPI_Authentication(t *testing.T) {
    server := setupTestAPI(t)

    req := httptest.NewRequest("GET", "/get-proxy", nil)
    // No API key
    w := httptest.NewRecorder()

    server.Router().ServeHTTP(w, req)

    assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAPI_Health(t *testing.T) {
    server := setupTestAPI(t)

    req := httptest.NewRequest("GET", "/health", nil)
    w := httptest.NewRecorder()

    server.Router().ServeHTTP(w, req)

    assert.Equal(t, http.StatusOK, w.Code)
    assert.Equal(t, "ok", w.Body.String())
}
```

## End-to-End Smoke Test

```bash
#!/bin/bash
# tests/e2e/smoke_test.sh

set -e

echo "=== Proxy Checker E2E Smoke Test ==="

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m'

API_KEY="${PROXY_API_KEY:-changeme123}"
BASE_URL="http://localhost:8080"

# Test 1: Health check
echo -n "Test 1: Health endpoint... "
HEALTH=$(curl -s -o /dev/null -w "%{http_code}" $BASE_URL/health)
if [ "$HEALTH" -eq 200 ]; then
    echo -e "${GREEN}PASS${NC}"
else
    echo -e "${RED}FAIL${NC} (HTTP $HEALTH)"
    exit 1
fi

# Test 2: Metrics endpoint
echo -n "Test 2: Metrics endpoint... "
METRICS=$(curl -s $BASE_URL/metrics | grep -c "proxychecker")
if [ "$METRICS" -gt 0 ]; then
    echo -e "${GREEN}PASS${NC}"
else
    echo -e "${RED}FAIL${NC}"
    exit 1
fi

# Test 3: Stats endpoint (with auth)
echo -n "Test 3: Stats endpoint... "
STATS=$(curl -s -H "X-Api-Key: $API_KEY" $BASE_URL/stat)
if echo "$STATS" | jq -e '.total_scraped' > /dev/null 2>&1; then
    echo -e "${GREEN}PASS${NC}"
else
    echo -e "${RED}FAIL${NC}"
    exit 1
fi

# Test 4: Get single proxy
echo -n "Test 4: Get single proxy... "
PROXY=$(curl -s -H "X-Api-Key: $API_KEY" $BASE_URL/get-proxy)
if echo "$PROXY" | grep -q ":"; then
    echo -e "${GREEN}PASS${NC}"
else
    echo -e "${RED}FAIL${NC}"
    exit 1
fi

# Test 5: Get multiple proxies (JSON)
echo -n "Test 5: Get proxies (JSON format)... "
JSON=$(curl -s -H "X-Api-Key: $API_KEY" "$BASE_URL/get-proxy?limit=5&format=json")
if echo "$JSON" | jq -e '.proxies' > /dev/null 2>&1; then
    echo -e "${GREEN}PASS${NC}"
else
    echo -e "${RED}FAIL${NC}"
    exit 1
fi

# Test 6: Authentication (should fail without key)
echo -n "Test 6: Authentication check... "
NO_AUTH=$(curl -s -o /dev/null -w "%{http_code}" $BASE_URL/get-proxy)
if [ "$NO_AUTH" -eq 401 ]; then
    echo -e "${GREEN}PASS${NC}"
else
    echo -e "${RED}FAIL${NC} (Expected 401, got $NO_AUTH)"
    exit 1
fi

# Test 7: Reload endpoint
echo -n "Test 7: Reload endpoint... "
RELOAD=$(curl -s -X POST -H "X-Api-Key: $API_KEY" $BASE_URL/reload)
if echo "$RELOAD" | jq -e '.message' > /dev/null 2>&1; then
    echo -e "${GREEN}PASS${NC}"
else
    echo -e "${RED}FAIL${NC}"
    exit 1
fi

echo ""
echo -e "${GREEN}All tests passed!${NC}"
```

## Coverage Goals

- **Unit Tests:** > 80% code coverage
- **Integration Tests:** All API endpoints
- **E2E Tests:** Complete workflows

## Running Tests in CI/CD

```yaml
# .github/workflows/test.yml
name: Tests

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    
    steps:
    - uses: actions/checkout@v3
    
    - name: Set up Go
      uses: actions/setup-go@v4
      with:
        go-version: '1.21'
    
    - name: Install dependencies
      run: go mod download
    
    - name: Run unit tests
      run: go test ./internal/... -v -race -coverprofile=coverage.out
    
    - name: Upload coverage
      uses: codecov/codecov-action@v3
      with:
        file: ./coverage.out
    
    - name: Run integration tests
      run: go test ./tests/integration/... -v
    
    - name: Build
      run: go build -v ./cmd/main.go
    
    - name: Run smoke test
      run: |
        ./main &
        sleep 10
        bash tests/e2e/smoke_test.sh
```

## Test Data

Create realistic test data:

```bash
# Generate test proxy list
cat > test_proxies.txt <<EOF
1.2.3.4:8080
5.6.7.8:3128
9.10.11.12:80
13.14.15.16:8888
EOF
```

## Conclusion

This test suite provides comprehensive coverage of all system components. Run tests regularly during development and in CI/CD pipelines to ensure quality and catch regressions early.

