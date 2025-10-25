# Proxy Checker API - Quick Reference Card

**Version:** 1.0.0 | **Port:** 8083 | **Protocol:** HTTP/REST

---

## 🚀 Installation (Ubuntu)

### One-Command Setup
```bash
sudo bash setup-ubuntu.sh
```

### Manual Setup
```bash
cp config.example.json config.json
echo "PROXY_API_KEY=$(openssl rand -hex 16)" > .env
docker compose up -d
```

---

## 🔧 Essential Commands

### Service Management
```bash
# Start service
docker compose up -d

# Stop service
docker compose down

# Restart service
docker compose restart proxy-checker

# View logs
docker compose logs -f proxy-checker

# Rebuild
docker compose down
docker compose build --no-cache
docker compose up -d
```

### Monitoring
```bash
# Check health
curl http://localhost:8083/health

# View logs (last 50 lines)
docker compose logs proxy-checker --tail=50

# Check container stats
docker stats proxy-checker

# Check container status
docker ps | grep proxy-checker
```

---

## 🌐 API Endpoints

### Get API Key
```bash
API_KEY=$(grep PROXY_API_KEY .env | cut -d= -f2)
echo $API_KEY
```

### Health Check (No Auth)
```bash
curl http://localhost:8083/health
```

### Get Statistics
```bash
curl -H "X-Api-Key: $API_KEY" http://localhost:8083/stat | jq
```

### Get Single Proxy
```bash
curl -H "X-Api-Key: $API_KEY" http://localhost:8083/get-proxy
```

### Get Multiple Proxies
```bash
# Get 10 proxies
curl -H "X-Api-Key: $API_KEY" "http://localhost:8083/get-proxy?limit=10"

# Get all proxies
curl -H "X-Api-Key: $API_KEY" "http://localhost:8083/get-proxy?all=1"

# Get as JSON
curl -H "X-Api-Key: $API_KEY" "http://localhost:8083/get-proxy?format=json" | jq
```

### Trigger Manual Reload
```bash
curl -X POST -H "X-Api-Key: $API_KEY" http://localhost:8083/reload
```

### Prometheus Metrics
```bash
curl http://localhost:8083/metrics
```

---

## ⚙️ Configuration Quick Edit

### File Location
```bash
nano ~/proxy-checker-api/config.json
```

### Key Settings

**Reduce Concurrency (if high CPU/memory)**
```json
{
  "checker": {
    "concurrency_total": 10000,
    "batch_size": 1000
  }
}
```

**Increase Timeout (for slow proxies)**
```json
{
  "checker": {
    "timeout_ms": 20000
  }
}
```

**Change Check Interval**
```json
{
  "aggregator": {
    "interval_seconds": 300
  }
}
```

**After editing config:**
```bash
# Option 1: Reload without restart (most settings)
curl -X POST -H "X-Api-Key: $API_KEY" http://localhost:8083/reload

# Option 2: Full restart (for port, storage changes)
docker compose restart proxy-checker
```

---

## 🐛 Common Issues & Quick Fixes

### Error: `URLSchemeUnknown: Not supported URL scheme http+docker`
```bash
# Quick fix
sudo bash setup-ubuntu.sh

# Or manual fix
sudo apt-get remove docker-compose
sudo apt-get install docker-compose-plugin
docker compose up -d  # Note: space, not hyphen
```

### No proxies available
```bash
# Wait 1-2 minutes, then:
curl -H "X-Api-Key: $API_KEY" http://localhost:8083/stat | jq

# Force reload
curl -X POST -H "X-Api-Key: $API_KEY" http://localhost:8083/reload
```

### Container won't start
```bash
# Check logs
docker compose logs proxy-checker --tail=50

# Ensure config exists
cp config.example.json config.json

# Rebuild
docker compose down
docker compose up -d --build
```

### Port already in use
```bash
# Find what's using port 8083
sudo netstat -tulpn | grep 8083

# Change port in config.json
nano config.json  # Change "addr": ":8083" to ":8084"

# Restart
docker compose restart proxy-checker
```

### 401 Unauthorized
```bash
# Check API key
cat .env | grep PROXY_API_KEY

# Use correct key
API_KEY="your-actual-key-here"
curl -H "X-Api-Key: $API_KEY" http://localhost:8083/stat
```

### High CPU usage
```bash
# Edit config to reduce concurrency
nano config.json

# Set:
# "concurrency_total": 10000  (reduce from 20000+)
# "batch_size": 1000         (reduce from 2000+)

# Restart
docker compose restart proxy-checker
```

---

## 📊 System Tuning

### File Descriptors
```bash
# Check current limit
ulimit -n

# Set temporarily
ulimit -n 65535

# Set permanently (add to /etc/security/limits.conf)
* soft nofile 65535
* hard nofile 65535
```

### TCP Tuning
```bash
sudo sysctl -w net.ipv4.ip_local_port_range="10000 65535"
sudo sysctl -w net.ipv4.tcp_max_syn_backlog=8192
sudo sysctl -w net.core.somaxconn=8192
```

Or just run:
```bash
sudo bash setup-ubuntu.sh  # Applies all tuning
```

---

## 📈 Monitoring Stack

### Start Prometheus + Grafana
```bash
docker compose --profile monitoring up -d
```

### Access
- **Grafana:** http://localhost:3000 (admin/admin)
- **Prometheus:** http://localhost:9090

---

## 🔍 Debugging

### Collect Debug Info
```bash
# System info
docker --version
docker compose version
uname -a

# Service status
docker ps | grep proxy-checker
docker stats proxy-checker --no-stream

# Logs
docker compose logs proxy-checker --tail=100

# Config
cat config.json | jq

# Environment
cat .env

# Resources
free -h
df -h
ulimit -n
```

### Access Container Shell
```bash
docker compose exec proxy-checker sh
```

---

## 📚 Documentation

- **DEPLOY_NOW.md** - Quick deployment guide with all fixes
- **TROUBLESHOOTING.md** - Complete troubleshooting guide
- **README.md** - Full documentation
- **OPS_CHECKLIST.md** - Production deployment
- **PERFORMANCE_TESTING.md** - Performance benchmarks

---

## 🎯 Performance Tuning Guide

### Conservative (Low Resource)
```json
{
  "checker": {
    "concurrency_total": 5000,
    "batch_size": 500,
    "timeout_ms": 10000
  }
}
```

### Balanced (Recommended)
```json
{
  "checker": {
    "concurrency_total": 10000,
    "batch_size": 1000,
    "timeout_ms": 15000
  }
}
```

### Aggressive (High Performance)
```json
{
  "checker": {
    "concurrency_total": 20000,
    "batch_size": 2000,
    "timeout_ms": 15000,
    "enable_adaptive_concurrency": true
  }
}
```

### Maximum (12-thread server)
```json
{
  "checker": {
    "concurrency_total": 25000,
    "batch_size": 2500,
    "timeout_ms": 12000,
    "enable_adaptive_concurrency": true,
    "max_cpu_usage_percent": 95
  }
}
```

---

## 🔐 Security Checklist

- [ ] Use strong API key: `openssl rand -hex 32`
- [ ] Set `.env` permissions: `chmod 600 .env`
- [ ] Enable firewall: `sudo ufw enable && sudo ufw allow 8083/tcp`
- [ ] Use HTTPS with reverse proxy (nginx/caddy)
- [ ] Keep Docker updated: `sudo apt update && sudo apt upgrade`
- [ ] Regular backups of config.json and .env

---

## 📞 Quick Help

**Issue with docker-compose?**
→ See DEPLOY_NOW.md or run `sudo bash setup-ubuntu.sh`

**No proxies working?**
→ Wait 2 minutes, then check `/stat` endpoint

**High resource usage?**
→ Reduce concurrency_total and batch_size in config.json

**Need more help?**
→ Check TROUBLESHOOTING.md for comprehensive guide

---

**Tip:** Bookmark this file for quick reference!

Save this command for easy access:
```bash
cat ~/proxy-checker-api/QUICKREF.md
```

Or create an alias:
```bash
echo 'alias proxyref="cat ~/proxy-checker-api/QUICKREF.md"' >> ~/.bashrc
source ~/.bashrc
proxyref  # Show this reference anytime
```

