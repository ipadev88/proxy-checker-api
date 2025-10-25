# Troubleshooting Guide

This guide covers common issues and their solutions for the Proxy Checker API.

## Table of Contents

1. [Docker-Compose Issues](#docker-compose-issues)
2. [Container Issues](#container-issues)
3. [Network Issues](#network-issues)
4. [Performance Issues](#performance-issues)
5. [Proxy Issues](#proxy-issues)
6. [API Issues](#api-issues)

---

## Docker-Compose Issues

### Error: `URLSchemeUnknown: Not supported URL scheme http+docker`

**Cause:** Incompatible Python packages in docker-compose

**Solution:** Use the automated fix script:

```bash
# Run the setup script (recommended)
sudo bash setup-ubuntu.sh
```

Or manually install Docker Compose Plugin v2:

```bash
# Remove old version
sudo apt-get remove docker-compose

# Install v2 plugin
sudo apt-get update
sudo apt-get install docker-compose-plugin

# Use 'docker compose' (with space, not hyphen)
docker compose up -d
```

**See also:** `DEPLOY_NOW.md` for deployment guide with all fixes applied

---

### Error: `docker: command not found`

**Cause:** Docker not installed

**Solution:**

```bash
# Run the setup script
sudo bash setup-ubuntu.sh
```

Or install manually:

```bash
curl -fsSL https://get.docker.com -o get-docker.sh
sudo sh get-docker.sh
sudo systemctl start docker
sudo systemctl enable docker
```

---

### Error: `Cannot connect to the Docker daemon`

**Cause:** Docker service not running or permission issues

**Solution:**

```bash
# Start Docker service
sudo systemctl start docker

# Check status
sudo systemctl status docker

# Add your user to docker group
sudo usermod -aG docker $USER

# Logout and login again for group change to take effect
```

---

## Container Issues

### Container exits immediately

**Diagnosis:**

```bash
# Check logs
docker compose logs proxy-checker

# Check container status
docker ps -a | grep proxy-checker
```

**Common Causes & Solutions:**

1. **Missing config.json**
   ```bash
   cp config.example.json config.json
   ```

2. **Invalid JSON in config**
   ```bash
   # Validate JSON
   cat config.json | jq
   
   # If error, fix the JSON or restore from example
   cp config.example.json config.json
   ```

3. **Missing .env file**
   ```bash
   echo "PROXY_API_KEY=$(openssl rand -hex 16)" > .env
   ```

4. **Port already in use**
   ```bash
   # Check what's using port 8083
   sudo netstat -tulpn | grep 8083
   
   # Or with ss
   sudo ss -tulpn | grep 8083
   
   # Change port in config.json or stop conflicting service
   ```

---

### Container unhealthy

**Diagnosis:**

```bash
# Check health status
docker inspect proxy-checker | grep -A 10 Health

# Check logs
docker compose logs proxy-checker --tail=50
```

**Solution:**

```bash
# Restart container
docker compose restart proxy-checker

# If still unhealthy, rebuild
docker compose down
docker compose build --no-cache
docker compose up -d
```

---

### Out of Memory (OOM) errors

**Symptoms:**
- Container crashes
- Logs show: `killed` or `OOM`

**Solution:**

1. Reduce concurrency in `config.json`:
   ```json
   {
     "checker": {
       "concurrency_total": 10000,
       "batch_size": 1000
     }
   }
   ```

2. Increase Docker memory limit in `docker-compose.yml`:
   ```yaml
   services:
     proxy-checker:
       deploy:
         resources:
           limits:
             memory: 2G
   ```

3. Check memory usage:
   ```bash
   docker stats proxy-checker
   ```

---

## Network Issues

### Cannot access API from outside

**Cause:** Firewall or port not exposed

**Solution:**

```bash
# Check if port is listening
sudo netstat -tulpn | grep 8083

# Check firewall (UFW)
sudo ufw status
sudo ufw allow 8083/tcp

# Check firewall (iptables)
sudo iptables -L -n | grep 8083
```

---

### Slow proxy checking

**Symptoms:**
- Checking takes >5 minutes
- High CPU usage
- Many timeout errors

**Diagnosis:**

```bash
# Check logs for timeout errors
docker compose logs proxy-checker | grep -i timeout

# Check system resources
docker stats proxy-checker
htop
```

**Solutions:**

1. **Reduce concurrency** (start conservative):
   ```json
   {
     "checker": {
       "concurrency_total": 5000,
       "timeout_ms": 10000
     }
   }
   ```

2. **Apply system tuning** (see `OPS_CHECKLIST.md`):
   ```bash
   sudo bash setup-ubuntu.sh  # Applies tuning automatically
   ```

3. **Increase timeout** for slow proxies:
   ```json
   {
     "checker": {
       "timeout_ms": 20000
     }
   }
   ```

---

### Too many open files

**Symptoms:**
- Logs show: `too many open files`
- Service crashes during check

**Solution:**

```bash
# Check current limit
ulimit -n

# Set limit temporarily
ulimit -n 65535

# Set permanently (already done by setup-ubuntu.sh)
echo "* soft nofile 65535" | sudo tee -a /etc/security/limits.conf
echo "* hard nofile 65535" | sudo tee -a /etc/security/limits.conf

# Restart container
docker compose restart proxy-checker
```

---

## Proxy Issues

### No proxies available

**Symptoms:**
- API returns: `No alive proxies available`
- `/stat` shows `total_alive: 0`

**Diagnosis:**

```bash
# Get your API key
API_KEY=$(grep PROXY_API_KEY .env | cut -d= -f2)

# Check statistics
curl -H "X-Api-Key: $API_KEY" http://localhost:8083/stat | jq

# Check logs
docker compose logs proxy-checker | grep -i "alive"
```

**Solutions:**

1. **Wait for first check cycle** (1-2 minutes):
   ```bash
   # Wait
   sleep 120
   
   # Check again
   curl -H "X-Api-Key: $API_KEY" http://localhost:8083/stat | jq
   ```

2. **Manually trigger reload**:
   ```bash
   curl -X POST -H "X-Api-Key: $API_KEY" http://localhost:8083/reload
   ```

3. **Check if sources are accessible**:
   ```bash
   # Test proxy sources
   curl -I https://raw.githubusercontent.com/TheSpeedX/PROXY-List/master/http.txt
   curl -I https://api.proxyscrape.com/v2/?request=get&protocol=http
   ```

4. **Verify configuration**:
   ```bash
   # Check that sources are enabled
   cat config.json | jq '.aggregator.sources[] | select(.enabled == true)'
   ```

5. **Check test URL accessibility**:
   ```bash
   # Test if google.com is accessible from server
   curl -I https://www.google.com/generate_204
   
   # If blocked, change test_url in config.json to:
   # "test_url": "http://httpbin.org/get"
   ```

---

### Low alive proxy count

**Symptoms:**
- Very few proxies work (< 5%)
- Most proxies show as dead

**Possible Causes:**

1. **Timeout too aggressive**:
   ```json
   {
     "checker": {
       "timeout_ms": 20000  // Increase from 15000
     }
   }
   ```

2. **Test URL unreachable**:
   - Change to different test URL
   - Use `http://httpbin.org/get` or `http://ip-api.com/json`

3. **Proxy sources have bad proxies**:
   - Normal to have 10-30% success rate
   - Add better quality sources to `config.json`

4. **Network issues from your server**:
   ```bash
   # Test outbound connectivity
   curl -I https://www.google.com
   curl -I https://1.1.1.1
   ```

---

## API Issues

### 401 Unauthorized

**Cause:** Invalid or missing API key

**Solution:**

```bash
# Get your API key
cat .env | grep PROXY_API_KEY

# Use it in requests
API_KEY="your-key-here"
curl -H "X-Api-Key: $API_KEY" http://localhost:8083/stat

# Or as query parameter
curl "http://localhost:8083/stat?key=$API_KEY"
```

---

### 429 Too Many Requests

**Cause:** Rate limit exceeded

**Solution:**

Increase rate limit in `config.json`:

```json
{
  "api": {
    "rate_limit_per_minute": 3000,
    "rate_limit_per_ip": 200
  }
}
```

Then restart:

```bash
docker compose restart proxy-checker
```

---

### Slow API responses

**Diagnosis:**

```bash
# Test response time
time curl -H "X-Api-Key: $API_KEY" http://localhost:8083/get-proxy

# Check metrics
curl http://localhost:8083/metrics | grep api_request_duration
```

**Solutions:**

1. **Too many proxies in memory**:
   - Limit stored proxies
   - Use Redis storage instead of file

2. **High system load**:
   ```bash
   # Check load
   uptime
   docker stats
   
   # Reduce checker concurrency
   ```

---

## Performance Issues

### High CPU usage

**Symptoms:**
- CPU at 100% continuously
- System slow/unresponsive

**Solutions:**

1. **Reduce concurrency**:
   ```json
   {
     "checker": {
       "concurrency_total": 10000  // Reduce from 20000+
     }
   }
   ```

2. **Enable adaptive concurrency**:
   ```json
   {
     "checker": {
       "enable_adaptive_concurrency": true,
       "max_cpu_usage_percent": 85
     }
   }
   ```

3. **Increase check interval**:
   ```json
   {
     "aggregator": {
       "interval_seconds": 300  // Check every 5 minutes instead of 1
     }
   }
   ```

---

### High memory usage

**Diagnosis:**

```bash
# Check memory
docker stats proxy-checker

# Get heap profile
curl http://localhost:8083/metrics | grep go_memstats
```

**Solutions:**

1. **Reduce batch size**:
   ```json
   {
     "checker": {
       "batch_size": 1000  // Reduce from 2000+
     }
   }
   ```

2. **Reduce total proxies**:
   - Disable some sources
   - Keep only best quality sources

3. **Restart periodically**:
   ```bash
   # Add to crontab for daily restart at 3 AM
   0 3 * * * cd /root/proxy-checker-api && docker compose restart proxy-checker
   ```

---

## Getting More Help

### Collect Debug Information

```bash
#!/bin/bash
# debug-info.sh - Collect diagnostic information

echo "=== System Info ==="
uname -a
cat /etc/os-release

echo -e "\n=== Docker Version ==="
docker --version
docker compose version

echo -e "\n=== Container Status ==="
docker ps -a | grep proxy-checker

echo -e "\n=== Container Stats ==="
docker stats proxy-checker --no-stream

echo -e "\n=== Container Logs (last 50 lines) ==="
docker compose logs proxy-checker --tail=50

echo -e "\n=== Config ==="
cat config.json | jq

echo -e "\n=== Environment ==="
cat .env

echo -e "\n=== Port Status ==="
sudo netstat -tulpn | grep 8083

echo -e "\n=== File Descriptors ==="
ulimit -n

echo -e "\n=== Sysctl Settings ==="
sysctl net.ipv4.ip_local_port_range
sysctl net.ipv4.tcp_max_syn_backlog
sysctl net.core.somaxconn

echo -e "\n=== Disk Space ==="
df -h

echo -e "\n=== Memory ==="
free -h

echo -e "\n=== CPU Info ==="
lscpu | grep -E '^CPU\(s\)|^Thread|^Core'
```

Save and run:

```bash
bash debug-info.sh > debug-output.txt
cat debug-output.txt
```

---

## Quick Reference

### Essential Commands

```bash
# View logs
docker compose logs -f proxy-checker

# Restart service
docker compose restart proxy-checker

# Rebuild and restart
docker compose down
docker compose build --no-cache
docker compose up -d

# Check health
curl http://localhost:8083/health

# Get statistics (with your API key)
API_KEY=$(grep PROXY_API_KEY .env | cut -d= -f2)
curl -H "X-Api-Key: $API_KEY" http://localhost:8083/stat | jq

# Trigger reload
curl -X POST -H "X-Api-Key: $API_KEY" http://localhost:8083/reload

# Check resource usage
docker stats proxy-checker

# Access container shell
docker compose exec proxy-checker sh
```

---

### Config Reload Without Restart

```bash
# Edit config
nano config.json

# Trigger reload (picks up new config)
curl -X POST -H "X-Api-Key: $API_KEY" http://localhost:8083/reload
```

Note: Some settings (like port, storage type) require a full restart.

---

## Still Having Issues?

1. Run the automated setup script: `sudo bash setup-ubuntu.sh`
2. Check `DEPLOY_NOW.md` for quick deployment guide
3. Review `README.md` for general documentation
4. Check `OPS_CHECKLIST.md` for production setup
5. Review logs carefully: `docker compose logs proxy-checker`

If all else fails, try a clean reinstall:

```bash
# Backup config and env
cp config.json config.json.backup
cp .env .env.backup

# Clean everything
docker compose down -v
docker system prune -a

# Restore and restart
cp config.json.backup config.json
cp .env.backup .env
sudo bash setup-ubuntu.sh
```

