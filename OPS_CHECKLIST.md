# Operations & Safety Checklist

## Pre-Deployment Checklist

### System Requirements

- [ ] **OS:** Linux (Ubuntu 20.04+ or RHEL 8+)
- [ ] **CPU:** 12 threads minimum (verified: `nproc`)
- [ ] **RAM:** 4GB minimum, 8GB recommended
- [ ] **Disk:** 10GB available space (SSD preferred)
- [ ] **Network:** 1Gbps bandwidth, stable connectivity

### System Tuning

#### File Descriptors

```bash
# Check current limit
ulimit -n

# Set for current session
ulimit -n 65535

# Make permanent - add to /etc/security/limits.conf
echo "* soft nofile 65535" | sudo tee -a /etc/security/limits.conf
echo "* hard nofile 65535" | sudo tee -a /etc/security/limits.conf

# For systemd services - add to service file
LimitNOFILE=65535

# Verify
ulimit -n
```

**Calculation:**
- 20,000 concurrent connections × 2 FDs = 40,000
- + 1,000 for API, logs, etc. = 41,000
- Recommended: 65,535 (safe margin)

#### TCP/IP Stack Tuning

```bash
# Create tuning script
sudo tee /etc/sysctl.d/99-proxy-checker.conf <<EOF
# Increase local port range for outgoing connections
net.ipv4.ip_local_port_range = 10000 65535

# Increase max SYN backlog
net.ipv4.tcp_max_syn_backlog = 8192

# Enable TCP Fast Open
net.ipv4.tcp_fastopen = 3

# Reduce TIME_WAIT duration
net.ipv4.tcp_fin_timeout = 15

# Allow TIME_WAIT socket reuse
net.ipv4.tcp_tw_reuse = 1

# Increase socket listen backlog
net.core.somaxconn = 8192

# Increase connection tracking
net.netfilter.nf_conntrack_max = 200000
net.nf_conntrack_max = 200000

# TCP keepalive settings
net.ipv4.tcp_keepalive_time = 300
net.ipv4.tcp_keepalive_intvl = 30
net.ipv4.tcp_keepalive_probes = 3

# Network buffer sizes
net.core.rmem_max = 16777216
net.core.wmem_max = 16777216
net.ipv4.tcp_rmem = 4096 87380 16777216
net.ipv4.tcp_wmem = 4096 65536 16777216
EOF

# Apply
sudo sysctl -p /etc/sysctl.d/99-proxy-checker.conf

# Verify
sysctl net.ipv4.ip_local_port_range
sysctl net.core.somaxconn
```

#### Memory Management

```bash
# Disable swap for predictable performance (optional)
sudo swapoff -a

# Or set swappiness low
sudo sysctl -w vm.swappiness=10
echo "vm.swappiness = 10" | sudo tee -a /etc/sysctl.conf
```

### Firewall Configuration

```bash
# Allow API port (example with ufw)
sudo ufw allow 8080/tcp comment "Proxy Checker API"

# Allow Prometheus metrics scraping
sudo ufw allow from prometheus_ip to any port 8080

# Enable firewall
sudo ufw enable
```

### User & Permissions

```bash
# Create service user
sudo useradd -r -s /bin/false -M proxychecker

# Create directories
sudo mkdir -p /opt/proxy-checker
sudo mkdir -p /var/lib/proxy-checker
sudo mkdir -p /var/log/proxy-checker
sudo mkdir -p /etc/proxy-checker

# Set ownership
sudo chown -R proxychecker:proxychecker /opt/proxy-checker
sudo chown -R proxychecker:proxychecker /var/lib/proxy-checker
sudo chown -R proxychecker:proxychecker /var/log/proxy-checker

# Set permissions
sudo chmod 750 /opt/proxy-checker
sudo chmod 750 /var/lib/proxy-checker
sudo chmod 750 /etc/proxy-checker
```

---

## Installation

### Method 1: Binary Installation

```bash
# Download binary (replace with actual release)
curl -L https://github.com/your-org/proxy-checker/releases/download/v1.0.0/proxy-checker-linux-amd64 \
  -o /tmp/proxy-checker

# Verify checksum
sha256sum /tmp/proxy-checker
# Compare with published checksum

# Install
sudo mv /tmp/proxy-checker /opt/proxy-checker/proxy-checker
sudo chmod +x /opt/proxy-checker/proxy-checker
sudo chown proxychecker:proxychecker /opt/proxy-checker/proxy-checker

# Install config
sudo cp config.example.json /etc/proxy-checker/config.json
sudo chown proxychecker:proxychecker /etc/proxy-checker/config.json
sudo chmod 640 /etc/proxy-checker/config.json

# Edit config
sudo nano /etc/proxy-checker/config.json
```

### Method 2: Docker Installation

```bash
# Clone repository
git clone https://github.com/your-org/proxy-checker-api.git
cd proxy-checker-api

# Create environment file
cp .env.example .env
nano .env  # Set PROXY_API_KEY

# Edit config
cp config.example.json config.json
nano config.json

# Build and start
docker-compose up -d

# Verify
docker-compose logs -f
docker-compose ps
```

### Method 3: Build from Source

```bash
# Install Go 1.21+
wget https://go.dev/dl/go1.21.5.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.21.5.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin

# Clone and build
git clone https://github.com/your-org/proxy-checker-api.git
cd proxy-checker-api
go mod download
CGO_ENABLED=1 go build -o proxy-checker ./cmd/main.go

# Install
sudo cp proxy-checker /opt/proxy-checker/
sudo cp config.example.json /etc/proxy-checker/config.json
```

---

## Configuration

### Environment Variables

```bash
# Create environment file
sudo tee /etc/proxy-checker/env <<EOF
PROXY_API_KEY=your-secure-api-key-here-change-this
TZ=UTC
GOMAXPROCS=12
EOF

sudo chmod 600 /etc/proxy-checker/env
sudo chown proxychecker:proxychecker /etc/proxy-checker/env
```

### Configuration File

```bash
# Edit main config
sudo nano /etc/proxy-checker/config.json
```

**Key settings for 12-thread server:**

```json
{
  "checker": {
    "timeout_ms": 15000,
    "concurrency_total": 20000,
    "batch_size": 2000,
    "retries": 1
  },
  "storage": {
    "type": "file",
    "path": "/var/lib/proxy-checker/proxies.json"
  }
}
```

---

## Systemd Service Setup

### Install Service

```bash
# Copy service file
sudo cp proxy-checker.service /etc/systemd/system/

# Edit if needed
sudo nano /etc/systemd/system/proxy-checker.service

# Update paths in service file:
# WorkingDirectory=/opt/proxy-checker
# EnvironmentFile=/etc/proxy-checker/env
# ExecStart=/opt/proxy-checker/proxy-checker

# Reload systemd
sudo systemctl daemon-reload

# Enable service
sudo systemctl enable proxy-checker

# Start service
sudo systemctl start proxy-checker

# Check status
sudo systemctl status proxy-checker

# View logs
sudo journalctl -u proxy-checker -f
```

### Service Management Commands

```bash
# Start
sudo systemctl start proxy-checker

# Stop
sudo systemctl stop proxy-checker

# Restart
sudo systemctl restart proxy-checker

# Reload config (if SIGHUP handler implemented)
sudo systemctl reload proxy-checker

# Status
sudo systemctl status proxy-checker

# Enable auto-start
sudo systemctl enable proxy-checker

# Disable auto-start
sudo systemctl disable proxy-checker

# View logs (last 100 lines)
sudo journalctl -u proxy-checker -n 100

# Follow logs
sudo journalctl -u proxy-checker -f

# Logs since boot
sudo journalctl -u proxy-checker -b
```

---

## Logging

### Log Rotation

```bash
# Create logrotate config
sudo tee /etc/logrotate.d/proxy-checker <<EOF
/var/log/proxy-checker/*.log {
    daily
    rotate 14
    compress
    delaycompress
    missingok
    notifempty
    create 0640 proxychecker proxychecker
    sharedscripts
    postrotate
        systemctl reload proxy-checker > /dev/null 2>&1 || true
    endscript
}
EOF

# Test logrotate
sudo logrotate -d /etc/logrotate.d/proxy-checker

# Force rotation (if needed)
sudo logrotate -f /etc/logrotate.d/proxy-checker
```

### Log Monitoring

```bash
# Watch for errors
sudo journalctl -u proxy-checker -f | grep -i error

# Count error rate
sudo journalctl -u proxy-checker --since "1 hour ago" | grep -i error | wc -l

# Check memory usage from logs
sudo journalctl -u proxy-checker | grep "Memory:"
```

---

## Monitoring

### Prometheus Setup

```bash
# Install Prometheus
wget https://github.com/prometheus/prometheus/releases/download/v2.45.0/prometheus-2.45.0.linux-amd64.tar.gz
tar xvfz prometheus-2.45.0.linux-amd64.tar.gz
sudo mv prometheus-2.45.0.linux-amd64 /opt/prometheus

# Copy config
sudo cp prometheus.yml /opt/prometheus/

# Create systemd service
sudo tee /etc/systemd/system/prometheus.service <<EOF
[Unit]
Description=Prometheus
After=network.target

[Service]
User=prometheus
Group=prometheus
Type=simple
ExecStart=/opt/prometheus/prometheus \
  --config.file=/opt/prometheus/prometheus.yml \
  --storage.tsdb.path=/var/lib/prometheus

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable prometheus
sudo systemctl start prometheus
```

### Key Metrics to Monitor

```promql
# Alive proxy count
proxychecker_alive_proxies

# Check throughput
rate(proxychecker_checks_total[5m])

# Check success rate
rate(proxychecker_checks_success_total[5m]) / rate(proxychecker_checks_total[5m])

# Check latency (p99)
histogram_quantile(0.99, rate(proxychecker_check_duration_seconds_bucket[5m]))

# API throughput
rate(proxychecker_api_requests_total[1m])

# API latency (p99)
histogram_quantile(0.99, rate(proxychecker_api_request_duration_seconds_bucket[5m]))

# Memory usage
process_resident_memory_bytes{job="proxy-checker"}

# Goroutines
go_goroutines{job="proxy-checker"}

# Open file descriptors
process_open_fds{job="proxy-checker"}
```

### Alert Rules

Install alerts.yml as described in prometheus.yml, with alerts for:
- No alive proxies (critical)
- Low proxy count (warning)
- High failure rate (warning)
- High API latency (warning)
- Service down (critical)

---

## Backup & Recovery

### Backup Strategy

```bash
# Create backup script
sudo tee /opt/proxy-checker/backup.sh <<'EOF'
#!/bin/bash
BACKUP_DIR="/backup/proxy-checker"
DATE=$(date +%Y%m%d_%H%M%S)

mkdir -p $BACKUP_DIR

# Backup config
cp /etc/proxy-checker/config.json $BACKUP_DIR/config_$DATE.json

# Backup data
cp /var/lib/proxy-checker/proxies.json $BACKUP_DIR/proxies_$DATE.json

# Keep only last 7 days
find $BACKUP_DIR -name "*.json" -mtime +7 -delete

echo "Backup completed: $DATE"
EOF

sudo chmod +x /opt/proxy-checker/backup.sh

# Add to crontab (daily at 2 AM)
echo "0 2 * * * /opt/proxy-checker/backup.sh" | sudo crontab -u proxychecker -
```

### Recovery

```bash
# Restore from backup
sudo cp /backup/proxy-checker/config_YYYYMMDD_HHMMSS.json /etc/proxy-checker/config.json
sudo cp /backup/proxy-checker/proxies_YYYYMMDD_HHMMSS.json /var/lib/proxy-checker/proxies.json
sudo chown proxychecker:proxychecker /etc/proxy-checker/config.json
sudo chown proxychecker:proxychecker /var/lib/proxy-checker/proxies.json
sudo systemctl restart proxy-checker
```

---

## Health Checks

### Manual Health Checks

```bash
# Service running
systemctl is-active proxy-checker

# API responding
curl -f http://localhost:8080/health || echo "API down"

# Metrics available
curl -s http://localhost:8080/metrics | grep -q "proxychecker_alive_proxies"

# Proxy count
curl -s -H "X-Api-Key: $PROXY_API_KEY" http://localhost:8080/stat | jq '.total_alive'

# Memory usage
ps -o rss= -p $(pgrep proxy-checker) | awk '{print $1/1024 " MB"}'

# Open file descriptors
lsof -p $(pgrep proxy-checker) | wc -l

# Goroutines
curl -s http://localhost:8080/metrics | grep "^go_goroutines"
```

### Automated Health Check Script

```bash
#!/bin/bash
# /opt/proxy-checker/healthcheck.sh

set -e

API_KEY="${PROXY_API_KEY:-changeme123}"
THRESHOLD_PROXIES=100
THRESHOLD_MEMORY_MB=500
THRESHOLD_FDS=50000

# Check service
if ! systemctl is-active --quiet proxy-checker; then
  echo "ERROR: Service not running"
  exit 1
fi

# Check API
if ! curl -sf http://localhost:8080/health > /dev/null; then
  echo "ERROR: API not responding"
  exit 1
fi

# Check proxy count
ALIVE=$(curl -s -H "X-Api-Key: $API_KEY" http://localhost:8080/stat | jq -r '.total_alive')
if [ "$ALIVE" -lt "$THRESHOLD_PROXIES" ]; then
  echo "WARNING: Only $ALIVE alive proxies (threshold: $THRESHOLD_PROXIES)"
fi

# Check memory
PID=$(pgrep proxy-checker)
RSS_MB=$(ps -o rss= -p $PID | awk '{print $1/1024}')
if (( $(echo "$RSS_MB > $THRESHOLD_MEMORY_MB" | bc -l) )); then
  echo "WARNING: High memory usage: ${RSS_MB}MB (threshold: ${THRESHOLD_MEMORY_MB}MB)"
fi

# Check file descriptors
FDS=$(lsof -p $PID | wc -l)
if [ "$FDS" -gt "$THRESHOLD_FDS" ]; then
  echo "WARNING: High FD count: $FDS (threshold: $THRESHOLD_FDS)"
fi

echo "OK: All checks passed"
```

### Add to cron (every 5 minutes)

```bash
echo "*/5 * * * * /opt/proxy-checker/healthcheck.sh >> /var/log/proxy-checker/healthcheck.log 2>&1" | sudo crontab -u proxychecker -
```

---

## Troubleshooting

### Service Won't Start

```bash
# Check logs
sudo journalctl -u proxy-checker -n 50

# Check config syntax
sudo -u proxychecker /opt/proxy-checker/proxy-checker -config /etc/proxy-checker/config.json -validate

# Check permissions
ls -la /opt/proxy-checker
ls -la /var/lib/proxy-checker

# Check port availability
sudo netstat -tulpn | grep :8080
```

### High Memory Usage

```bash
# Check current usage
ps aux | grep proxy-checker

# Get heap profile
curl http://localhost:8080/debug/pprof/heap > heap.prof
go tool pprof -top heap.prof

# Potential fixes:
# - Reduce concurrency_total in config
# - Reduce batch_size
# - Check for goroutine leaks
# - Restart service
```

### High CPU Usage

```bash
# Check CPU
top -p $(pgrep proxy-checker)

# Get CPU profile
curl http://localhost:8080/debug/pprof/profile?seconds=30 > cpu.prof
go tool pprof -top cpu.prof

# Potential fixes:
# - Increase timeout_ms to reduce retry rate
# - Use "connect-only" mode instead of "full-http"
# - Reduce concurrency_total
```

### No Proxies Available

```bash
# Check stats
curl -s -H "X-Api-Key: $API_KEY" http://localhost:8080/stat | jq

# Check sources
grep "Source" /var/log/proxy-checker/*.log

# Manual trigger reload
curl -X POST -H "X-Api-Key: $API_KEY" http://localhost:8080/reload

# Test sources manually
curl -I https://raw.githubusercontent.com/TheSpeedX/PROXY-List/master/http.txt
```

### File Descriptor Exhaustion

```bash
# Check current FDs
lsof -p $(pgrep proxy-checker) | wc -l

# Check limit
cat /proc/$(pgrep proxy-checker)/limits | grep "open files"

# Increase limit (temporary)
prlimit --pid $(pgrep proxy-checker) --nofile=65535

# Permanent: update /etc/security/limits.conf and restart
```

---

## Security Checklist

- [ ] API key set and rotated regularly
- [ ] Service runs as non-root user
- [ ] File permissions restricted (640 for configs, 750 for binaries)
- [ ] Firewall configured (only necessary ports open)
- [ ] TLS/HTTPS for API (if exposed publicly) - use reverse proxy
- [ ] Rate limiting enabled
- [ ] Logs rotated and monitoring in place
- [ ] Regular security updates applied
- [ ] Metrics endpoint restricted (internal network only)

---

## Performance Optimization

### For 20k Concurrent Checks

```json
{
  "checker": {
    "timeout_ms": 15000,
    "concurrency_total": 20000,
    "batch_size": 2000,
    "retries": 1,
    "mode": "full-http"
  }
}
```

### For 25k Concurrent Checks (maximum)

```json
{
  "checker": {
    "timeout_ms": 12000,
    "concurrency_total": 25000,
    "batch_size": 2500,
    "retries": 0,
    "mode": "connect-only",
    "enable_adaptive_concurrency": true
  }
}
```

### Memory Constrained (< 4GB RAM)

```json
{
  "checker": {
    "concurrency_total": 10000,
    "batch_size": 1000
  }
}
```

---

## Maintenance

### Regular Tasks

**Daily:**
- [ ] Check logs for errors
- [ ] Verify proxy count > threshold
- [ ] Check memory/CPU usage

**Weekly:**
- [ ] Review metrics and performance
- [ ] Check disk space
- [ ] Verify backups

**Monthly:**
- [ ] Rotate API keys
- [ ] Review and update proxy sources
- [ ] Check for updates
- [ ] Performance testing

### Update Procedure

```bash
# 1. Backup current version
sudo cp /opt/proxy-checker/proxy-checker /opt/proxy-checker/proxy-checker.bak

# 2. Download new version
curl -L https://github.com/your-org/proxy-checker/releases/download/vX.Y.Z/proxy-checker-linux-amd64 \
  -o /tmp/proxy-checker-new

# 3. Verify checksum
sha256sum /tmp/proxy-checker-new

# 4. Stop service
sudo systemctl stop proxy-checker

# 5. Replace binary
sudo mv /tmp/proxy-checker-new /opt/proxy-checker/proxy-checker
sudo chmod +x /opt/proxy-checker/proxy-checker
sudo chown proxychecker:proxychecker /opt/proxy-checker/proxy-checker

# 6. Start service
sudo systemctl start proxy-checker

# 7. Verify
sudo systemctl status proxy-checker
curl http://localhost:8080/health

# 8. Rollback if needed
# sudo mv /opt/proxy-checker/proxy-checker.bak /opt/proxy-checker/proxy-checker
# sudo systemctl restart proxy-checker
```

---

## Support & Documentation

- **Logs:** `/var/log/proxy-checker/` or `journalctl -u proxy-checker`
- **Config:** `/etc/proxy-checker/config.json`
- **Data:** `/var/lib/proxy-checker/`
- **Metrics:** `http://localhost:8080/metrics`
- **API Docs:** See README.md

---

## Checklist Summary

✅ **Pre-Deployment**
- System requirements met
- ulimit set to 65535
- TCP tuning applied
- User and directories created

✅ **Deployment**
- Binary installed or container running
- Configuration customized
- Environment variables set
- Service enabled and started

✅ **Post-Deployment**
- Health check passing
- Metrics available
- Logs being written
- Proxies being checked
- API responding

✅ **Monitoring**
- Prometheus scraping
- Alerts configured
- Logs rotated
- Backups scheduled

✅ **Security**
- API key set
- Non-root user
- Firewall configured
- Permissions restricted

