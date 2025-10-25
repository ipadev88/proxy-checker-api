# Complete Deployment Guide for Ubuntu Server

This guide will help you deploy the Proxy Checker API on your Ubuntu server with all fixes applied.

---

## 🎯 Your Specific Issue - SOLVED

### The Error You Had

```
URLSchemeUnknown: Not supported URL scheme http+docker
```

### Root Cause

Your Ubuntu server has the old Python-based docker-compose (v1.29.2) which conflicts with newer versions of urllib3 and requests libraries. This is a **very common issue** on Ubuntu servers.

### The Solution

I've created an automated fix script that handles everything.

---

## 🚀 Step-by-Step Deployment

### Step 1: Update Your Code

On your Ubuntu server:

```bash
cd ~/proxy-checker-api
git pull origin main
```

This will get all the fixes including:
- ✅ Port fixes (consistent 8083)
- ✅ Automated setup script
- ✅ New documentation
- ✅ Configuration templates

### Step 2: Run the Automated Setup

```bash
sudo bash setup-ubuntu.sh
```

**This script will:**
1. Check for Docker (install if missing)
2. Remove old Python-based docker-compose
3. Install Docker Compose Plugin v2 (Go-based, no Python issues)
4. Install required utilities (curl, jq, etc.)
5. Create `config.json` from example
6. Generate a secure API key in `.env` file
7. Apply system tuning (file descriptors, TCP settings)
8. Build and start the Docker container
9. Verify the deployment
10. Display your API key and test commands

**Expected output:**
```
╔═══════════════════════════════════════════════════════════╗
║              Setup Complete! 🎉                           ║
╚═══════════════════════════════════════════════════════════╝

Service Information:
  • API URL:      http://localhost:8083
  • Health:       http://localhost:8083/health
  • Metrics:      http://localhost:8083/metrics

Your API Key:
  abc123def456...

Quick Test Commands:
  curl http://localhost:8083/health
  curl -H "X-Api-Key: abc123..." http://localhost:8083/stat | jq
  ...
```

### Step 3: Save Your API Key

The script generates a secure API key. **Save it somewhere safe!**

```bash
# View your API key
cat .env | grep PROXY_API_KEY

# Or save it to a variable
API_KEY=$(grep PROXY_API_KEY .env | cut -d= -f2)
echo $API_KEY
```

### Step 4: Wait for First Check

The service needs 1-2 minutes to:
1. Fetch proxies from sources
2. Check which ones are alive
3. Build the initial database

```bash
# Wait 2 minutes
sleep 120

# Then check statistics
curl -H "X-Api-Key: $API_KEY" http://localhost:8083/stat | jq
```

**Expected output:**
```json
{
  "total_scraped": 5000,
  "total_alive": 1523,
  "total_dead": 3477,
  "alive_percent": "30.46%",
  "last_check": "2025-10-25T12:34:56Z",
  "updated": "2025-10-25T12:35:10Z"
}
```

### Step 5: Test the API

```bash
# Get a single proxy
curl -H "X-Api-Key: $API_KEY" http://localhost:8083/get-proxy

# Get 10 proxies
curl -H "X-Api-Key: $API_KEY" "http://localhost:8083/get-proxy?limit=10"

# Get proxies in JSON format
curl -H "X-Api-Key: $API_KEY" "http://localhost:8083/get-proxy?format=json" | jq
```

---

## 🔧 Configuration (Optional)

The default configuration works well, but you can tune it based on your server's resources.

### Edit Configuration

```bash
nano ~/proxy-checker-api/config.json
```

### For Your 12-Thread Server

**Conservative (Low CPU usage, fewer proxies):**
```json
{
  "checker": {
    "concurrency_total": 10000,
    "batch_size": 1000,
    "timeout_ms": 15000
  }
}
```

**Balanced (Recommended for 12 threads):**
```json
{
  "checker": {
    "concurrency_total": 20000,
    "batch_size": 2000,
    "timeout_ms": 15000
  }
}
```

**Aggressive (Maximum performance):**
```json
{
  "checker": {
    "concurrency_total": 25000,
    "batch_size": 2500,
    "timeout_ms": 12000,
    "enable_adaptive_concurrency": true
  }
}
```

### Apply Configuration Changes

```bash
# Option 1: Hot reload (no downtime, most settings)
curl -X POST -H "X-Api-Key: $API_KEY" http://localhost:8083/reload

# Option 2: Restart container (for port/storage changes)
docker compose restart proxy-checker
```

---

## 📊 Monitoring

### View Logs

```bash
# Follow logs in real-time
docker compose logs -f proxy-checker

# Last 50 lines
docker compose logs proxy-checker --tail=50

# Search logs
docker compose logs proxy-checker | grep -i error
```

### Check Resource Usage

```bash
# Container stats
docker stats proxy-checker

# System resources
htop
free -h
df -h
```

### Monitor Metrics

```bash
# Prometheus metrics
curl http://localhost:8083/metrics

# Key metrics
curl http://localhost:8083/metrics | grep proxychecker_alive_proxies
curl http://localhost:8083/metrics | grep proxychecker_checks_total
```

### Start Grafana Dashboard (Optional)

```bash
# Start monitoring stack
docker compose --profile monitoring up -d

# Access Grafana
# URL: http://your-server-ip:3000
# Login: admin / admin
```

---

## 🔄 Daily Operations

### Start Service

```bash
docker compose up -d
```

### Stop Service

```bash
docker compose down
```

### Restart Service

```bash
docker compose restart proxy-checker
```

### Update Service

```bash
# Pull latest code
git pull origin main

# Rebuild and restart
docker compose down
docker compose build --no-cache
docker compose up -d
```

### Backup Configuration

```bash
# Backup config and API key
cp config.json config.json.backup
cp .env .env.backup

# Or create a full backup
tar -czf proxy-checker-backup-$(date +%Y%m%d).tar.gz \
  config.json .env docker-compose.yml
```

---

## 🐛 Troubleshooting

### Container Won't Start

```bash
# Check logs
docker compose logs proxy-checker --tail=50

# Check container status
docker ps -a | grep proxy-checker

# Verify config
cat config.json | jq

# Restart
docker compose restart proxy-checker
```

### No Proxies Available

```bash
# Check stats
curl -H "X-Api-Key: $API_KEY" http://localhost:8083/stat | jq

# Trigger manual reload
curl -X POST -H "X-Api-Key: $API_KEY" http://localhost:8083/reload

# Wait and check again
sleep 120
curl -H "X-Api-Key: $API_KEY" http://localhost:8083/stat | jq
```

### High CPU/Memory Usage

```bash
# Check usage
docker stats proxy-checker

# Reduce concurrency in config.json
nano config.json
# Set: "concurrency_total": 10000

# Restart
docker compose restart proxy-checker
```

### Port Already in Use

```bash
# Find what's using port 8083
sudo netstat -tulpn | grep 8083

# Change port in config.json
nano config.json
# Change: "addr": ":8083" to "addr": ":8084"

# Update docker-compose.yml
nano docker-compose.yml
# Change: - "8083:8083" to - "8084:8084"

# Restart
docker compose restart proxy-checker
```

### 401 Unauthorized Error

```bash
# Check API key
cat .env | grep PROXY_API_KEY

# Use correct key
API_KEY="your-actual-key"
curl -H "X-Api-Key: $API_KEY" http://localhost:8083/stat
```

**For complete troubleshooting, see [TROUBLESHOOTING.md](TROUBLESHOOTING.md)**

---

## 🔒 Security Hardening

### 1. Firewall Configuration

```bash
# Enable firewall
sudo ufw enable

# Allow SSH (important!)
sudo ufw allow 22/tcp

# Allow API port
sudo ufw allow 8083/tcp

# Check status
sudo ufw status
```

### 2. Use Strong API Key

```bash
# Generate a strong API key
openssl rand -hex 32

# Update .env file
nano .env
# Change PROXY_API_KEY to new value

# Restart
docker compose restart proxy-checker
```

### 3. Secure File Permissions

```bash
# Restrict .env file
chmod 600 .env

# Verify
ls -la .env
```

### 4. Set Up HTTPS with Nginx (Recommended for Production)

Create `/etc/nginx/sites-available/proxy-checker`:

```nginx
server {
    listen 80;
    server_name your-domain.com;

    location / {
        proxy_pass http://localhost:8083;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

Enable and install SSL with Let's Encrypt:

```bash
sudo ln -s /etc/nginx/sites-available/proxy-checker /etc/nginx/sites-enabled/
sudo apt install certbot python3-certbot-nginx
sudo certbot --nginx -d your-domain.com
sudo nginx -t
sudo systemctl reload nginx
```

---

## 📈 Performance Tuning

### Monitor Performance

```bash
# Check response times
time curl -H "X-Api-Key: $API_KEY" http://localhost:8083/get-proxy

# Check goroutines
curl http://localhost:8083/metrics | grep go_goroutines

# Check memory
curl http://localhost:8083/metrics | grep go_memstats
```

### Tune Based on Results

**If CPU is high (>90%):**
- Reduce `concurrency_total`
- Increase `timeout_ms`
- Enable `enable_adaptive_concurrency`

**If memory is high (>80%):**
- Reduce `batch_size`
- Reduce `concurrency_total`
- More frequent restarts

**If checks are slow:**
- Increase `timeout_ms`
- Reduce `concurrency_total`
- Check network connectivity

---

## 🔄 Maintenance Schedule

### Daily
- Check service health: `curl http://localhost:8083/health`
- Review logs: `docker compose logs proxy-checker --tail=50`

### Weekly
- Check statistics: API endpoint `/stat`
- Review resource usage: `docker stats`
- Backup configuration files

### Monthly
- Update Docker images: `docker compose pull`
- Review and update proxy sources in config.json
- Check for application updates: `git pull`

---

## 📞 Getting Help

### Quick Reference Files

1. **START_HERE.txt** - Quick start guide (plain text)
2. **QUICKREF.md** - Command reference card
3. **DOCKER_FIX.md** - Docker-compose specific fixes
4. **TROUBLESHOOTING.md** - Complete troubleshooting guide
5. **README.md** - Full documentation

### Diagnostic Commands

```bash
# Collect all diagnostic info
cat > collect-debug.sh << 'EOF'
#!/bin/bash
echo "=== Docker Version ==="
docker --version
docker compose version

echo -e "\n=== Container Status ==="
docker ps -a | grep proxy-checker

echo -e "\n=== Container Logs ==="
docker compose logs proxy-checker --tail=50

echo -e "\n=== Config ==="
cat config.json | jq

echo -e "\n=== Resource Usage ==="
docker stats proxy-checker --no-stream

echo -e "\n=== System Info ==="
free -h
df -h
ulimit -n
EOF

chmod +x collect-debug.sh
./collect-debug.sh
```

---

## ✅ Deployment Checklist

Use this checklist to ensure everything is set up correctly:

- [ ] Run `sudo bash setup-ubuntu.sh` successfully
- [ ] Service is running: `docker ps | grep proxy-checker`
- [ ] Health check passes: `curl http://localhost:8083/health`
- [ ] API key is saved: `cat .env`
- [ ] First check completed (wait 2 minutes)
- [ ] Statistics show alive proxies: `/stat` endpoint
- [ ] Can get proxies: `/get-proxy` endpoint
- [ ] Logs show no errors: `docker compose logs`
- [ ] Resource usage is acceptable: `docker stats`
- [ ] Firewall configured: `sudo ufw status`
- [ ] Backups created: `config.json.backup`, `.env.backup`
- [ ] Documentation reviewed: `README.md`

---

## 🎉 Success Criteria

Your deployment is successful when:

✅ Service responds to health check
✅ Container stays running
✅ Logs show successful proxy checks
✅ Statistics show alive proxies (>5%)
✅ API returns proxies when requested
✅ CPU usage is stable (<95%)
✅ Memory usage is stable (<80%)
✅ No error messages in logs

---

## 📝 What Changed from Original

### Fixes Applied

1. **Docker-Compose Fix**: Replaced Python version with Docker Compose Plugin v2
2. **Port Standardization**: All components now use port 8083
3. **Automated Setup**: One-command installation and configuration
4. **Documentation**: Comprehensive guides for every scenario
5. **Error Handling**: Better error messages and recovery procedures

### New Files

- `setup-ubuntu.sh` - Automated setup script
- `DOCKER_FIX.md` - Docker-compose troubleshooting
- `TROUBLESHOOTING.md` - Complete troubleshooting guide
- `QUICKREF.md` - Quick reference card
- `START_HERE.txt` - Quick start guide
- `FIXES_APPLIED.md` - List of all fixes
- `DEPLOYMENT_GUIDE.md` - This file

---

## 🚀 Ready to Deploy

You're all set! Your Proxy Checker API is now:

- ✅ Fully fixed and production-ready
- ✅ Extensively documented
- ✅ Easy to deploy with one command
- ✅ Simple to maintain and troubleshoot

Just run on your Ubuntu server:

```bash
cd ~/proxy-checker-api
sudo bash setup-ubuntu.sh
```

And you're done! 🎉

---

**Need help?** Check the documentation files or run:
```bash
cat START_HERE.txt
```

**Version:** 1.0.0  
**Last Updated:** October 25, 2025  
**Status:** Production Ready

