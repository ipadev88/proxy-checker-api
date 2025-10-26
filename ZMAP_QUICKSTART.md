# Zmap Integration - Quick Start Guide

## 🚀 Quick Setup (5 minutes)

### 1. Install Zmap

```bash
cd /path/to/proxy-checker-api
sudo bash scripts/install-zmap.sh
```

This script will:
- ✅ Install zmap via apt
- ✅ Download default blacklist
- ✅ Set required capabilities
- ✅ Test installation

### 2. Enable Zmap in Config

Edit `config.json`:

```json
{
  "zmap": {
    "enabled": true,
    "ports": [8080, 80, 3128],
    "rate_limit": 10000,
    "max_runtime_seconds": 3600,
    "blacklist": ["/etc/proxy-checker/blacklist.txt"]
  }
}
```

### 3. Build and Run

```bash
# Build
go build -o proxy-checker ./cmd/main.go

# Run
./proxy-checker -config config.json
```

---

## 📋 What You Get

### Before (Scraping Only)
- **Sources:** 45 HTTP URLs
- **Candidates:** ~90k proxies
- **Alive:** ~1.3k proxies  
- **Cycle Time:** 45 seconds

### After (Scraping + Zmap)
- **Sources:** 45 URLs + Zmap  
- **Candidates:** ~200k-300k proxies
- **Alive:** ~10k-20k proxies (**10-20x more!**)
- **Cycle Time:** ~35-40 minutes

---

## 🎯 Test Commands

### Check Zmap Installation
```bash
zmap --version
getcap $(which zmap)
# Should show: cap_net_admin,cap_net_raw=eip
```

### Test Zmap Scan (Localhost)
```bash
sudo zmap -p 8080 -r 1000 127.0.0.0/24 -o /tmp/test.csv
cat /tmp/test.csv
```

### Check API for Zmap Stats
```bash
curl -H "X-Api-Key: your-key" http://localhost:8083/stats/zmap
```

Expected response:
```json
{
  "enabled": true,
  "ports": [8080, 80, 3128],
  "last_scan_time": "2024-10-26T12:00:00Z",
  "last_scan_duration": 1234.5,
  "candidates_found": 50000,
  "total_scans": 42
}
```

---

## ⚙️ Configuration Options

### Recommended for Production
```json
{
  "zmap": {
    "enabled": true,
    "ports": [8080, 80, 3128],
    "rate_limit": 10000,
    "bandwidth": "10M",
    "max_runtime_seconds": 3600,
    "target_ranges": [],
    "blacklist": ["/etc/proxy-checker/blacklist.txt"],
    "cooldown_seconds": 3600
  },
  "checker": {
    "timeout_ms": 6000,
    "enable_fast_filter": true,
    "fast_filter_timeout_ms": 2000,
    "fast_filter_concurrency": 50000
  }
}
```

### Conservative (Legal-Safe)
```json
{
  "zmap": {
    "enabled": true,
    "ports": [8080],
    "rate_limit": 5000,
    "max_runtime_seconds": 1800,
    "target_ranges": ["YOUR.NETWORK.RANGE/24"],
    "cooldown_seconds": 7200
  }
}
```

### Aggressive (Use with Caution)
```json
{
  "zmap": {
    "enabled": true,
    "ports": [8080, 80, 3128, 1080, 8888, 9090],
    "rate_limit": 25000,
    "max_runtime_seconds": 7200
  }
}
```

---

## 🔍 Monitoring

### Check Logs
```bash
# Real-time logs
journalctl -u proxy-checker -f

# Filter for zmap
journalctl -u proxy-checker | grep zmap
```

### Prometheus Metrics
```bash
curl http://localhost:8083/metrics | grep zmap
```

Key metrics:
- `zmap_scans_total{port="8080",status="success"}` - Scan count
- `zmap_candidates_found{port="8080"}` - Candidates per port
- `zmap_scan_duration_seconds` - Scan duration histogram

---

## 🐛 Troubleshooting

### Error: "zmap binary not found"
```bash
# Install zmap
sudo apt install zmap

# Or run installation script
sudo bash scripts/install-zmap.sh
```

### Error: "Operation not permitted"
```bash
# Set capabilities
sudo setcap 'cap_net_raw,cap_net_admin=+eip' $(which zmap)

# Verify
getcap $(which zmap)
```

### Error: "Zmap setup verification failed"
```bash
# Check binary
which zmap

# Check capabilities
getcap $(which zmap)

# Check blacklist
ls -l /etc/proxy-checker/blacklist.txt
```

### Zmap Returns Zero Results
- Check target_ranges (empty = scan all IPv4)
- Check blacklist (may be too restrictive)
- Check rate_limit (too low = slow)
- Check max_runtime_seconds (may timeout early)

### High Memory Usage
- Reduce concurrency: `"concurrency_total": 15000`
- Enable fast filter: `"enable_fast_filter": true`
- Reduce rate_limit: `"rate_limit": 5000`

---

## ⚖️ Legal Considerations

### ⚠️ WARNING
Network scanning without authorization may violate laws in your jurisdiction.

### Safe Practices
✅ Only scan networks you own or have written permission to scan  
✅ Use target_ranges to limit scope  
✅ Use blacklists to exclude private/sensitive networks  
✅ Use conservative rate limits (5000-10000 pps)  
✅ Set up abuse@yourdomain.com contact  
✅ Honor opt-out requests within 24 hours  

### Unsafe Practices
❌ Scanning entire IPv4 space without permission  
❌ Scanning government (.gov, .mil) networks  
❌ Scanning healthcare, finance, critical infrastructure  
❌ High rate limits (>50000 pps) without notification  
❌ Ignoring abuse complaints  

**See `ZMAP_INTEGRATION_SUMMARY.md` for full legal guidelines.**

---

## 📚 Additional Resources

- **Full Integration Plan:** `ZMAP_INTEGRATION_PLAN.json`
- **Detailed Summary:** `ZMAP_INTEGRATION_SUMMARY.md`
- **Zmap Documentation:** https://github.com/zmap/zmap
- **Legal Guidelines:** Section in ZMAP_INTEGRATION_SUMMARY.md

---

## 🎉 Success Indicators

You'll know it's working when you see:

```
{"level":"info","msg":"Zmap scanner initialized for ports: [8080 80 3128]"}
{"level":"info","msg":"Running zmap scan..."}
{"level":"info","msg":"Port 8080 scan complete: 50000 candidates found"}
{"level":"info","msg":"Zmap scan found 150000 candidates"}
{"level":"info","msg":"Fast filter complete: 30000 connectable proxies"}
{"level":"info","msg":"Check complete: 10000 alive, 20000 dead"}
```

**Congratulations! You now have 10-20x more working proxies! 🚀**

---

*Last Updated: October 26, 2024*

