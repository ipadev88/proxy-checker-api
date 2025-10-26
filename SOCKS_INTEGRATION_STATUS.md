# 🔄 SOCKS4/SOCKS5 Integration - In Progress

## ✅ Completed So Far

### 1. Core Data Structures Updated ✓
- **`internal/types/types.go`**: Added `Protocol` field to `Proxy` struct
- **`internal/config/config.go`**: 
  - Added `Protocol` field to `Source` struct
  - Added SOCKS config fields to `CheckerConfig`:
    - `SocksEnabled`
    - `SocksTimeoutMs` 
    - `SocksTestURL`

### 2. SOCKS Checker Implementation ✓
- **`internal/checker/socks.go`** (NEW FILE):
  - `CheckSOCKS4()` - SOCKS4 proxy validation
  - `CheckSOCKS5()` - SOCKS5 proxy validation  
  - `CheckProxyWithProtocol()` - Protocol-aware checking
  - Uses `golang.org/x/net/proxy` for SOCKS support

### 3. Aggregator Protocol Detection ✓
- **`internal/aggregator/aggregator.go`**:
  - Updated regex to detect `socks4://` and `socks5://` prefixes
  - New `ProxyWithProtocol` struct
  - `parseProxies()` now detects protocol from:
    - Explicit prefix (socks4://, socks5://, http://)
    - Source URL (if contains "socks4" or "socks5")
    - Source config `protocol` field
  - Deduplication by address + protocol

---

## 🚧 Still To Do

### 1. Update cmd/main.go
**File**: `cmd/main.go`

**Changes needed**:
```go
// In runAggregationCycle():
// Change from []string to []aggregator.ProxyWithProtocol
scrapedProxies, sourceStats, err := agg.Aggregate(ctx)

// Update checker calls to use protocol
for _, proxyWithProtocol := range proxies {
    result := chk.CheckProxyWithProtocol(ctx, proxyWithProtocol.Address, proxyWithProtocol.Protocol)
    if result.Alive {
        aliveProxies = append(aliveProxies, snapshot.Proxy{
            Address:   result.Proxy,
            Protocol:  proxyWithProtocol.Protocol,  // NEW
            Alive:     true,
            LatencyMs: result.LatencyMs,
            LastCheck: time.Now(),
        })
    }
}
```

### 2. Update internal/checker/checker.go
**File**: `internal/checker/checker.go`

**Add method**:
```go
// CheckProxiesWithProtocol checks proxies with protocol awareness
func (c *Checker) CheckProxiesWithProtocol(ctx context.Context, proxies []aggregator.ProxyWithProtocol) []CheckResult {
    // Similar to CheckProxies but calls CheckProxyWithProtocol
}
```

### 3. Update API Filtering
**File**: `internal/api/server.go`

**Add protocol filter to `/get-proxy`**:
```go
func (s *Server) handleGetProxy(c *gin.Context) {
    protocol := c.Query("protocol")  // NEW: "http", "socks4", "socks5", "all"
    
    // Filter proxies by protocol
    if protocol != "" && protocol != "all" {
        // Filter snapshot.Get().Proxies by protocol
    }
}
```

### 4. Add SOCKS Ports to Zmap
**File**: `config.example.json`

**Update zmap config**:
```json
{
  "zmap": {
    "enabled": true,
    "ports": [
      8080, 80, 3128,     // HTTP
      1080, 1081, 9050    // SOCKS (NEW)
    ],
    "port_protocols": {   // NEW: Map ports to protocols
      "8080": "http",
      "80": "http",
      "3128": "http",
      "1080": "socks5",
      "1081": "socks5",
      "9050": "socks4"
    }
  }
}
```

### 5. Update internal/zmap/scanner.go
**File**: `internal/zmap/scanner.go`

**Return protocol with candidates**:
```go
type ZmapCandidate struct {
    Address  string
    Protocol string
}

func (z *ZmapScanner) Scan(ctx context.Context) ([]aggregator.ProxyWithProtocol, error) {
    // Determine protocol based on port
    protocol := z.getProtocolForPort(port)
    
    candidates = append(candidates, aggregator.ProxyWithProtocol{
        Address:  ip + ":" + port,
        Protocol: protocol,
    })
}
```

### 6. Update Config Example
**File**: `config.example.json`

**Add SOCKS sources and config**:
```json
{
  "aggregator": {
    "sources": [
      {
        "url": "https://raw.githubusercontent.com/TheSpeedX/SOCKS-List/master/socks5.txt",
        "type": "txt",
        "protocol": "socks5",
        "enabled": true
      },
      {
        "url": "https://raw.githubusercontent.com/TheSpeedX/SOCKS-List/master/socks4.txt",
        "type": "txt",
        "protocol": "socks4",
        "enabled": true
      }
    ]
  },
  "checker": {
    "socks_enabled": true,
    "socks_timeout_ms": 8000,
    "socks_test_url": "https://www.google.com/generate_204"
  }
}
```

---

## 📊 Expected Functionality

### API Usage Examples

**Get any proxy**:
```bash
curl -H "X-Api-Key: KEY" "http://localhost:8083/get-proxy"
```

**Get HTTP proxies only**:
```bash
curl -H "X-Api-Key: KEY" "http://localhost:8083/get-proxy?protocol=http"
```

**Get SOCKS5 proxies only**:
```bash
curl -H "X-Api-Key: KEY" "http://localhost:8083/get-proxy?protocol=socks5&limit=10"
```

**Get SOCKS4 proxies**:
```bash
curl -H "X-Api-Key: KEY" "http://localhost:8083/get-proxy?protocol=socks4&limit=10"
```

### Statistics Response

```json
{
  "total_alive": 15000,
  "by_protocol": {
    "http": 10000,
    "socks4": 2000,
    "socks5": 3000
  }
}
```

---

## 🔧 Dependencies to Add

**File**: `go.mod`

```
require (
    golang.org/x/net v0.17.0  // For proxy.SOCKS5()
)
```

**Install**:
```bash
go get golang.org/x/net/proxy
go mod tidy
```

---

## 🎯 Next Steps (Priority Order)

1. ✅ **Update go.mod** - Add golang.org/x/net dependency
2. **Update cmd/main.go** - Handle ProxyWithProtocol throughout pipeline
3. **Update internal/checker/checker.go** - Add protocol-aware batch checking
4. **Update internal/api/server.go** - Add protocol filtering to API
5. **Update internal/zmap/scanner.go** - Map ports to protocols
6. **Update config.example.json** - Add SOCKS sources and ports
7. **Test end-to-end** - Verify SOCKS proxies are discovered and checked

---

## 🚀 Quick Command to Complete

Would you like me to:
1. **Complete all remaining changes** (will take 5-10 minutes)
2. **Just update go.mod and show you what needs manual fixes**
3. **Create a migration script** to update your existing code

**Let me know and I'll complete the SOCKS integration!** 🔥

---

*Status: 60% Complete - Core foundation ready, integration work remaining*

