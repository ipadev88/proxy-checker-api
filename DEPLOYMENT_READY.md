# 🚀 Deployment Ready - Complete Setup Guide

## Quick Start (5 Minutes)

### Prerequisites
- Ubuntu 20.04+ server
- Root or sudo access
- Public IP address
- Ports 8083 (API), 9090 (Prometheus), 3000 (Grafana) open in firewall

### One-Command Setup
```bash
git clone https://github.com/ipadev88/proxy-checker-api.git
cd proxy-checker-api
sudo bash setup-ubuntu.sh
```

That's it! The script will:
1. ✅ Install Docker & Docker Compose v2
2. ✅ Install zmap with capabilities
3. ✅ Configure system for high-performance (ulimits, TCP tuning)
4. ✅ Download blacklist for safe scanning
5. ✅ Set up firewall rules
6. ✅ Generate API key
7. ✅ Build and start all services
8. ✅ Enable HTTP + SOCKS4 + SOCKS5 support

## What's Included

### Services
- **Proxy Checker API** (port 8083) - Main service with zmap integration
- **Redis** (port 6379) - Cache and session storage
- **Prometheus** (port 9090) - Metrics collection
- **Grafana** (port 3000) - Visualization dashboards

### Features
- ✅ High-speed proxy scraping (45+ sources)
- ✅ Zmap network scanning (10k-25k proxies/second)
- ✅ HTTP, SOCKS4, SOCKS5 protocol support
- ✅ Fast TCP filter for initial pruning
- ✅ Adaptive concurrency (10k-25k concurrent checks)
- ✅ RESTful API with authentication
- ✅ Prometheus metrics & Grafana dashboards
- ✅ Automatic snapshot persistence
- ✅ Rate limiting per IP
- ✅ Health checks and auto-recovery

## API Usage

### Authentication
Set your API key:
```bash
export API_KEY="your-generated-key"
```

### Get Proxies

#### Single Random Proxy (Any Protocol)
```bash
curl -H "X-Api-Key: $API_KEY" "http://your-server:8083/get-proxy"
```

#### Get 100 HTTP Proxies
```bash
curl -H "X-Api-Key: $API_KEY" "http://your-server:8083/get-proxy?limit=100&protocol=http"
```

#### Get All SOCKS5 Proxies
```bash
curl -H "X-Api-Key: $API_KEY" "http://your-server:8083/get-proxy?all=1&protocol=socks5"
```

#### Get SOCKS4 Proxies in JSON Format
```bash
curl -H "X-Api-Key: $API_KEY" "http://your-server:8083/get-proxy?limit=50&protocol=socks4&format=json"
```

### Statistics & Monitoring

#### Check Service Health
```bash
curl http://your-server:8083/health
```

#### Get Statistics
```bash
curl -H "X-Api-Key: $API_KEY" "http://your-server:8083/stat"
```

Example response:
```json
{
  "total_scraped": 125000,
  "total_alive": 1500,
  "total_dead": 123500,
  "alive_percent": "1.20%",
  "last_check": "2025-10-26T10:30:00Z",
  "updated": "2025-10-26T10:30:15Z"
}
```

#### Zmap Statistics
```bash
curl -H "X-Api-Key: $API_KEY" "http://your-server:8083/stats/zmap"
```

#### Prometheus Metrics
```bash
curl http://your-server:8083/metrics
```

### Reload Configuration
```bash
curl -X POST -H "X-Api-Key: $API_KEY" "http://your-server:8083/reload"
```

## Configuration

### Main Config File
Edit `config.json` (or copy from `config.example.json`):

```json
{
  "aggregator": {
    "interval_seconds": 60,
    "sources": [
      {
        "url": "https://api.proxyscrape.com/v2/?request=get&protocol=http",
        "protocol": "http",
        "enabled": true
      },
      {
        "url": "https://api.proxyscrape.com/v2/?request=get&protocol=socks5",
        "protocol": "socks5",
        "enabled": true
      }
    ]
  },
  "zmap": {
    "enabled": true,
    "ports": [8080, 80, 3128, 1080, 1081],
    "rate_limit": 10000,
    "bandwidth": "10M",
    "max_runtime_seconds": 3600
  },
  "checker": {
    "timeout_ms": 10000,
    "concurrency_total": 20000,
    "enable_fast_filter": true,
    "fast_filter_timeout_ms": 2000,
    "socks_enabled": true,
    "socks_timeout_ms": 15000
  }
}
```

### Environment Variables
```bash
export PROXY_API_KEY="your-secure-api-key"
```

## Management Commands

### View Logs
```bash
# All services
docker-compose logs -f

# Proxy checker only
docker-compose logs -f proxy-checker

# Last 100 lines
docker-compose logs --tail=100 proxy-checker
```

### Restart Services
```bash
# All services
docker-compose restart

# Proxy checker only
docker-compose restart proxy-checker
```

### Stop Services
```bash
docker-compose stop
```

### Start Services
```bash
docker-compose up -d
```

### Rebuild After Config Changes
```bash
docker-compose down
docker-compose up -d --build
```

## Performance Tuning

### For Higher Concurrency (25k+)
```json
{
  "checker": {
    "concurrency_total": 25000,
    "batch_size": 3000,
    "timeout_ms": 8000,
    "enable_fast_filter": true,
    "fast_filter_concurrency": 60000
  }
}
```

### For Faster Scanning
```json
{
  "aggregator": {
    "interval_seconds": 30
  },
  "zmap": {
    "rate_limit": 50000,
    "bandwidth": "100M",
    "cooldown_seconds": 1800
  }
}
```

### For Stability (Lower Concurrency)
```json
{
  "checker": {
    "concurrency_total": 10000,
    "batch_size": 1000,
    "timeout_ms": 15000
  }
}
```

## Automated Proxy Collection

### Cron Job for Regular Updates
Create `/root/update-proxies.sh`:
```bash
#!/bin/bash
API_KEY="your-api-key"
API_URL="http://localhost:8083"
OUTPUT_FILE="/root/proxies.txt"

curl -s -H "X-Api-Key: $API_KEY" "$API_URL/get-proxy?all=1" -o "$OUTPUT_FILE"
echo "$(date): Updated $(wc -l < $OUTPUT_FILE) proxies" >> /var/log/proxy-update.log
```

Add to crontab:
```bash
chmod +x /root/update-proxies.sh
crontab -e
# Add this line (update every 2 minutes):
*/2 * * * * /root/update-proxies.sh
```

## Monitoring & Dashboards

### Grafana Dashboard
1. Access: `http://your-server:3000`
2. Default credentials: `admin` / `admin`
3. Dashboard included: Proxy Checker Overview
4. Import custom dashboard: Upload `grafana-dashboard.json`

### Prometheus Alerts
Alerts configured in `alerts.yml`:
- High proxy failure rate
- Zmap scan failures
- API response time degradation

### System Monitoring
```bash
# Check resource usage
docker stats

# Check disk space
df -h /var/lib/docker

# Check system load
htop
```

## Troubleshooting

### Service Won't Start
```bash
# Check logs
docker-compose logs proxy-checker

# Verify config
cat config.json | jq

# Check port availability
sudo netstat -tulpn | grep 8083
```

### Zmap Not Working
```bash
# Verify zmap is installed
which zmap

# Check capabilities
getcap /usr/local/bin/zmap

# Re-run capabilities setup
sudo setcap cap_net_raw,cap_net_admin+eip /usr/local/bin/zmap

# Test zmap manually
sudo zmap -p 8080 -o /tmp/test.csv 8.8.8.8/32
```

### Low Proxy Count
1. **Check sources**: Verify URLs in config are accessible
2. **Check zmap**: Ensure zmap is enabled and running
3. **Adjust timeouts**: Increase `timeout_ms` in checker config
4. **Check network**: Verify server has good internet connection
5. **Review logs**: Look for specific error messages

### High Resource Usage
```bash
# Reduce concurrency
# Edit config.json, set:
"checker": {
  "concurrency_total": 10000,
  "max_fd_usage_percent": 50,
  "max_cpu_usage_percent": 60
}

# Restart
docker-compose restart proxy-checker
```

### API Key Issues
```bash
# Generate new API key
openssl rand -hex 16

# Set in environment
export PROXY_API_KEY="new-key"

# Restart services
docker-compose restart
```

## Security Best Practices

### 1. API Key
- Use strong, random API keys
- Rotate keys regularly
- Never commit keys to git

### 2. Firewall
```bash
# Allow only necessary ports
sudo ufw allow 22/tcp
sudo ufw allow 8083/tcp
sudo ufw allow 9090/tcp
sudo ufw allow 3000/tcp
sudo ufw enable
```

### 3. Rate Limiting
Already configured in API (1200 requests/minute globally, 100/minute per IP)

### 4. Zmap Safety
- Blacklist configured: `/etc/proxy-checker/blacklist.txt`
- Default target: public internet (be responsible!)
- Rate limited: 10k packets/second
- Bandwidth limited: 10M

### 5. Updates
```bash
# Update regularly
cd ~/proxy-checker-api
git pull
docker-compose down
docker-compose up -d --build
```

## Production Checklist

- ☑️ Set strong API key in environment
- ☑️ Configure firewall (ufw)
- ☑️ Set up automated backups (config.json, snapshots)
- ☑️ Configure monitoring alerts
- ☑️ Test all API endpoints
- ☑️ Set up log rotation
- ☑️ Configure reverse proxy (optional: nginx)
- ☑️ Set up SSL/TLS (optional: Let's Encrypt)
- ☑️ Schedule regular maintenance window
- ☑️ Document custom configurations

## Reverse Proxy (Optional)

### Nginx Config
```nginx
server {
    listen 80;
    server_name your-domain.com;

    location / {
        proxy_pass http://localhost:8083;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    }
}
```

### SSL with Let's Encrypt
```bash
sudo apt install certbot python3-certbot-nginx
sudo certbot --nginx -d your-domain.com
```

## Support & Documentation

- **Full Documentation**: See `README.md`
- **Zmap Integration**: See `ZMAP_INTEGRATION_SUMMARY.md`
- **SOCKS Support**: See `SOCKS_INTEGRATION_COMPLETE.md`
- **Architecture**: See `ARCHITECTURE.md`
- **Performance**: See `PERFORMANCE_TESTING.md`

## Backup & Recovery

### Backup
```bash
#!/bin/bash
BACKUP_DIR="/backup/proxy-checker"
mkdir -p $BACKUP_DIR

# Backup config
cp config.json $BACKUP_DIR/config-$(date +%Y%m%d).json

# Backup snapshot
cp /data/proxies.json $BACKUP_DIR/proxies-$(date +%Y%m%d).json

# Backup docker volumes
docker-compose down
tar -czf $BACKUP_DIR/volumes-$(date +%Y%m%d).tar.gz /var/lib/docker/volumes/
docker-compose up -d
```

### Restore
```bash
# Restore config
cp $BACKUP_DIR/config-20251026.json config.json

# Restore snapshot
cp $BACKUP_DIR/proxies-20251026.json /data/proxies.json

# Restart services
docker-compose restart
```

## Performance Expectations

### Server: 12 CPU threads, 16GB RAM
- **Aggregation**: 100k+ proxies/minute from 45+ sources
- **Zmap scanning**: 10k-25k candidates/second
- **Checking**: 15k-25k concurrent checks
- **API throughput**: 1200 requests/minute (configurable)
- **Typical alive rate**: 1-3% (depends on sources)

### Expected Results
- **Total scraped**: 100k-500k per cycle
- **Zmap candidates**: 5k-50k per scan
- **Alive proxies**: 1k-5k
- **Check duration**: 30-60 seconds per cycle
- **Memory usage**: 500MB-2GB
- **CPU usage**: 50-80% during checks

## Next Steps

1. ✅ Run `sudo bash setup-ubuntu.sh`
2. ✅ Test API endpoints
3. ✅ Monitor initial aggregation cycle
4. ✅ Set up automated proxy collection
5. ✅ Configure Grafana dashboards
6. ✅ Set up backups
7. ✅ Document your custom configuration

---

**Status**: ✅ Production Ready
**Version**: 2.0.0 with SOCKS + Zmap
**Last Updated**: October 26, 2025

🎉 **Your proxy checker is ready to deploy!**

