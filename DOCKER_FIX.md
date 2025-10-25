# Docker-Compose Fix Guide

## Problem
Error: `URLSchemeUnknown: Not supported URL scheme http+docker`

This occurs due to incompatible versions of `urllib3`, `requests`, and the Python-based `docker-compose`.

## Solution Options

### Option 1: Install Docker Compose Plugin (RECOMMENDED)

This installs the modern Go-based Docker Compose v2, which doesn't have Python dependency issues.

```bash
# Remove old Python-based docker-compose
sudo apt-get remove docker-compose

# Install Docker Compose Plugin (v2)
sudo apt-get update
sudo apt-get install docker-compose-plugin

# Verify installation
docker compose version

# Now use 'docker compose' (space, not hyphen)
docker compose up -d
```

### Option 2: Fix Python Dependencies

If you need to keep the Python version:

```bash
# Uninstall conflicting packages
sudo pip3 uninstall urllib3 requests docker docker-compose -y

# Reinstall with compatible versions
sudo pip3 install urllib3==1.26.15
sudo pip3 install requests==2.28.2
sudo pip3 install docker==6.0.1
sudo pip3 install docker-compose==1.29.2

# Verify
docker-compose --version
```

### Option 3: Use Docker Compose Standalone Binary

```bash
# Download standalone binary
sudo curl -L "https://github.com/docker/compose/releases/download/v2.23.0/docker-compose-$(uname -s)-$(uname -m)" \
  -o /usr/local/bin/docker-compose

# Make executable
sudo chmod +x /usr/local/bin/docker-compose

# Verify
docker-compose --version
```

## Quick Start After Fix

### 1. Create config.json (if not exists)
```bash
cd ~/proxy-checker-api
cp config.example.json config.json
```

### 2. Create .env file (if not exists)
```bash
echo "PROXY_API_KEY=your-secure-api-key-here-$(openssl rand -hex 8)" > .env
```

### 3. Start the service

**If using Docker Compose v2 (plugin):**
```bash
docker compose up -d
```

**If using Docker Compose v1 (Python):**
```bash
docker-compose up -d
```

### 4. Check status
```bash
# View logs
docker compose logs -f proxy-checker

# Or for v1:
docker-compose logs -f proxy-checker
```

### 5. Test the service
```bash
# Check health
curl http://localhost:8083/health

# Get your API key
cat .env | grep PROXY_API_KEY

# Test with API key (replace YOUR_KEY with actual key)
curl -H "X-Api-Key: YOUR_KEY" http://localhost:8083/stat
```

## Common Issues After Fix

### Issue: "Cannot connect to Docker daemon"
```bash
# Start Docker service
sudo systemctl start docker

# Enable Docker to start on boot
sudo systemctl enable docker

# Add your user to docker group (logout/login required)
sudo usermod -aG docker $USER
```

### Issue: "Port already in use"
```bash
# Check what's using port 8083
sudo netstat -tulpn | grep 8083

# Or with ss
sudo ss -tulpn | grep 8083

# Stop conflicting service or change port in config.json
```

### Issue: Container exits immediately
```bash
# Check logs for errors
docker compose logs proxy-checker

# Common causes:
# 1. Missing config.json - copy from config.example.json
# 2. Invalid JSON in config.json - validate with: cat config.json | jq
# 3. Permission issues - check file ownership
```

### Issue: "No alive proxies available"
```bash
# Wait 1-2 minutes for first check cycle to complete
sleep 120

# Then check stats
curl -H "X-Api-Key: YOUR_KEY" http://localhost:8083/stat

# If still empty, check if proxy sources are accessible
curl -I https://raw.githubusercontent.com/TheSpeedX/PROXY-List/master/http.txt

# Manually trigger reload
curl -X POST -H "X-Api-Key: YOUR_KEY" http://localhost:8083/reload
```

## System Requirements Check

Before running, ensure your system meets requirements:

```bash
# Check Docker version (need 20.10+)
docker --version

# Check available memory (need 4GB+)
free -h

# Check disk space (need 10GB+)
df -h

# Set file descriptor limit
ulimit -n 65535

# Make permanent by adding to /etc/security/limits.conf:
echo "* soft nofile 65535" | sudo tee -a /etc/security/limits.conf
echo "* hard nofile 65535" | sudo tee -a /etc/security/limits.conf
```

## Production Deployment Checklist

1. **Security:**
   - [ ] Generate strong API key: `openssl rand -hex 32`
   - [ ] Set secure file permissions: `chmod 600 .env`
   - [ ] Configure firewall rules
   - [ ] Enable HTTPS with reverse proxy (nginx/caddy)

2. **Performance:**
   - [ ] Apply system tuning (see `OPS_CHECKLIST.md`)
   - [ ] Set `ulimit -n 65535`
   - [ ] Configure concurrency based on server specs

3. **Monitoring:**
   - [ ] Start with monitoring stack: `docker compose --profile monitoring up -d`
   - [ ] Access Grafana at http://localhost:3000 (admin/admin)
   - [ ] Set up alerts in Prometheus

4. **Backup:**
   - [ ] Backup config.json and .env files
   - [ ] Set up volume backups if using Redis
   - [ ] Document your configuration

## Troubleshooting Commands

```bash
# Restart everything
docker compose down
docker compose up -d

# Rebuild from scratch
docker compose down -v
docker compose build --no-cache
docker compose up -d

# View container stats
docker stats proxy-checker

# Execute commands in container
docker compose exec proxy-checker sh

# Check container health
docker inspect proxy-checker | grep -A 10 Health
```

## Getting Help

If you still have issues:

1. Check logs: `docker compose logs proxy-checker --tail=100`
2. Verify config: `cat config.json | jq`
3. Check environment: `docker compose exec proxy-checker env`
4. Review `README.md` and `OPS_CHECKLIST.md`

## Quick Command Reference

```bash
# Start
docker compose up -d

# Stop
docker compose down

# Restart
docker compose restart

# View logs
docker compose logs -f

# Update image
docker compose pull
docker compose up -d

# Check health
curl http://localhost:8083/health

# Get statistics
curl -H "X-Api-Key: YOUR_KEY" http://localhost:8083/stat | jq

# Get proxy
curl -H "X-Api-Key: YOUR_KEY" http://localhost:8083/get-proxy

# Reload proxies
curl -X POST -H "X-Api-Key: YOUR_KEY" http://localhost:8083/reload
```

