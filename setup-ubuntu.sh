#!/bin/bash

# Proxy Checker API - Ubuntu Server Setup Script
# This script fixes all common issues and sets up the service properly

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}╔═══════════════════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║    Proxy Checker API - Ubuntu Setup & Fix Script         ║${NC}"
echo -e "${BLUE}╚═══════════════════════════════════════════════════════════╝${NC}"
echo ""

# Check if running as root
if [ "$EUID" -ne 0 ]; then
    echo -e "${RED}Error: This script must be run as root${NC}"
    echo "Run: sudo bash setup-ubuntu.sh"
    exit 1
fi

echo -e "${YELLOW}[1/8] Checking Docker installation...${NC}"
if ! command -v docker &> /dev/null; then
    echo -e "${RED}Docker not found. Installing Docker...${NC}"
    apt-get update
    apt-get install -y ca-certificates curl gnupg lsb-release
    
    # Add Docker's official GPG key
    mkdir -p /etc/apt/keyrings
    curl -fsSL https://download.docker.com/linux/ubuntu/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg
    
    # Set up repository
    echo \
      "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu \
      $(lsb_release -cs) stable" | tee /etc/apt/sources.list.d/docker.list > /dev/null
    
    # Install Docker
    apt-get update
    apt-get install -y docker-ce docker-ce-cli containerd.io
    
    # Start and enable Docker
    systemctl start docker
    systemctl enable docker
    
    echo -e "${GREEN}✓ Docker installed successfully${NC}"
else
    echo -e "${GREEN}✓ Docker already installed: $(docker --version)${NC}"
fi

echo ""
echo -e "${YELLOW}[2/8] Fixing docker-compose issue...${NC}"

# Remove old Python-based docker-compose if it exists
if [ -f /usr/bin/docker-compose ] && [ -f /usr/local/lib/python*/dist-packages/compose/__init__.py 2>/dev/null ]; then
    echo "Removing old Python-based docker-compose..."
    apt-get remove -y docker-compose 2>/dev/null || true
    pip3 uninstall -y docker-compose 2>/dev/null || true
fi

# Install Docker Compose Plugin (v2)
if ! docker compose version &> /dev/null; then
    echo "Installing Docker Compose Plugin v2..."
    apt-get update
    apt-get install -y docker-compose-plugin
    echo -e "${GREEN}✓ Docker Compose Plugin installed${NC}"
else
    echo -e "${GREEN}✓ Docker Compose Plugin already installed: $(docker compose version)${NC}"
fi

echo ""
echo -e "${YELLOW}[3/9] Installing required utilities...${NC}"
apt-get install -y curl jq wget net-tools openssl
echo -e "${GREEN}✓ Utilities installed${NC}"

echo ""
echo -e "${YELLOW}[4/9] Installing and configuring Zmap...${NC}"

# Install zmap and dependencies
apt-get install -y zmap libpcap-dev

# Verify zmap installation
if ! command -v zmap &> /dev/null; then
    echo -e "${RED}✗ Zmap installation failed${NC}"
    exit 1
fi

ZMAP_VERSION=$(zmap --version 2>&1 | head -n1)
echo -e "${GREEN}✓ Zmap installed: ${ZMAP_VERSION}${NC}"

# Create directories for zmap
mkdir -p /etc/proxy-checker
mkdir -p /var/log/proxy-checker
echo -e "${GREEN}✓ Directories created${NC}"

# Set capabilities on zmap binary
echo "Setting capabilities on zmap..."
ZMAP_PATH=$(which zmap)
setcap 'cap_net_raw,cap_net_admin=+eip' "$ZMAP_PATH" 2>/dev/null || {
    echo -e "${YELLOW}⚠ Could not set capabilities (Docker will handle this)${NC}"
}

# Verify capabilities
if getcap "$ZMAP_PATH" 2>/dev/null | grep -q "cap_net_raw"; then
    echo -e "${GREEN}✓ Capabilities set on zmap${NC}"
else
    echo -e "${YELLOW}⚠ Capabilities not set (will work in Docker with cap_add)${NC}"
fi

# Additional system tuning for zmap
echo "Applying zmap network tuning..."
sysctl -w net.core.rmem_max=134217728 > /dev/null 2>&1 || true
sysctl -w net.core.wmem_max=134217728 > /dev/null 2>&1 || true
sysctl -w net.ipv4.tcp_rmem='4096 87380 67108864' > /dev/null 2>&1 || true
sysctl -w net.ipv4.tcp_wmem='4096 65536 67108864' > /dev/null 2>&1 || true

echo -e "${GREEN}✓ Zmap configuration complete${NC}"

echo ""
echo -e "${YELLOW}[5/9] Setting up configuration files...${NC}"

# Navigate to the project directory
cd ~/proxy-checker-api 2>/dev/null || cd /root/proxy-checker-api || {
    echo -e "${RED}Error: Cannot find proxy-checker-api directory${NC}"
    echo "Please run this script from the proxy-checker-api directory"
    exit 1
}

# Create config.json if it doesn't exist
if [ ! -f config.json ]; then
    if [ -f config.example.json ]; then
        cp config.example.json config.json
        echo -e "${GREEN}✓ Created config.json from example${NC}"
    else
        echo -e "${RED}Error: config.example.json not found${NC}"
        exit 1
    fi
else
    echo -e "${GREEN}✓ config.json already exists${NC}"
fi

# Create or update .env file
if [ ! -f .env ]; then
    API_KEY=$(openssl rand -hex 16)
    cat > .env <<EOF
PROXY_API_KEY=${API_KEY}
TZ=UTC
EOF
    echo -e "${GREEN}✓ Created .env file with API key: ${API_KEY}${NC}"
    echo -e "${YELLOW}⚠ IMPORTANT: Save this API key!${NC}"
else
    echo -e "${GREEN}✓ .env file already exists${NC}"
    # Show existing API key
    existing_key=$(grep PROXY_API_KEY .env | cut -d= -f2)
    if [ -n "$existing_key" ]; then
        echo -e "${BLUE}Your existing API key: ${existing_key}${NC}"
    fi
fi

echo ""
echo -e "${YELLOW}[6/9] Applying system tuning...${NC}"

# Set file descriptor limit
if ! grep -q "proxy-checker file limits" /etc/security/limits.conf; then
    cat >> /etc/security/limits.conf <<EOF

# proxy-checker file limits
* soft nofile 65535
* hard nofile 65535
EOF
    echo -e "${GREEN}✓ File descriptor limits configured${NC}"
else
    echo -e "${GREEN}✓ File descriptor limits already configured${NC}"
fi

# Set current session limit
ulimit -n 65535 2>/dev/null || true

# TCP tuning
echo -e "Applying TCP tuning..."
sysctl -w net.ipv4.ip_local_port_range="10000 65535" > /dev/null
sysctl -w net.ipv4.tcp_max_syn_backlog=8192 > /dev/null
sysctl -w net.ipv4.tcp_tw_reuse=1 > /dev/null
sysctl -w net.core.somaxconn=8192 > /dev/null
sysctl -w net.ipv4.tcp_fin_timeout=30 > /dev/null

# Make TCP tuning permanent
if ! grep -q "proxy-checker network tuning" /etc/sysctl.conf; then
    cat >> /etc/sysctl.conf <<EOF

# proxy-checker network tuning
net.ipv4.ip_local_port_range = 10000 65535
net.ipv4.tcp_max_syn_backlog = 8192
net.ipv4.tcp_tw_reuse = 1
net.core.somaxconn = 8192
net.ipv4.tcp_fin_timeout = 30
EOF
    echo -e "${GREEN}✓ TCP tuning configured${NC}"
else
    echo -e "${GREEN}✓ TCP tuning already configured${NC}"
fi

echo ""
echo -e "${YELLOW}[7/9] Stopping any existing containers...${NC}"
docker compose down 2>/dev/null || docker-compose down 2>/dev/null || true
echo -e "${GREEN}✓ Stopped existing containers${NC}"

echo ""
echo -e "${YELLOW}[8/9] Building and starting services...${NC}"
docker compose build --no-cache
docker compose up -d

echo -e "${GREEN}✓ Services started successfully${NC}"

echo ""
echo -e "${YELLOW}[9/9] Verifying deployment...${NC}"

# Wait for service to be ready
echo "Waiting for service to start..."
sleep 5

# Check if container is running
if docker ps | grep -q proxy-checker; then
    echo -e "${GREEN}✓ Container is running${NC}"
else
    echo -e "${RED}✗ Container is not running${NC}"
    echo "Checking logs..."
    docker compose logs proxy-checker
    exit 1
fi

# Check health endpoint
if curl -s http://localhost:8083/health | grep -q "ok"; then
    echo -e "${GREEN}✓ Health check passed${NC}"
else
    echo -e "${YELLOW}⚠ Health check not responding yet (this is normal on first start)${NC}"
fi

echo ""
echo -e "${GREEN}╔═══════════════════════════════════════════════════════════╗${NC}"
echo -e "${GREEN}║              Setup Complete! 🎉                           ║${NC}"
echo -e "${GREEN}╚═══════════════════════════════════════════════════════════╝${NC}"
echo ""
echo -e "${BLUE}Service Information:${NC}"
echo "  • API URL:      http://localhost:8083"
echo "  • Health:       http://localhost:8083/health"
echo "  • Metrics:      http://localhost:8083/metrics"
echo ""

# Get API key
API_KEY=$(grep PROXY_API_KEY .env | cut -d= -f2)
echo -e "${BLUE}Your API Key:${NC}"
echo -e "${YELLOW}  ${API_KEY}${NC}"
echo ""

echo -e "${BLUE}Quick Test Commands:${NC}"
echo ""
echo "  # Check health"
echo "  curl http://localhost:8083/health"
echo ""
echo "  # Check statistics (wait 1-2 minutes for first check to complete)"
echo "  curl -H \"X-Api-Key: ${API_KEY}\" http://localhost:8083/stat | jq"
echo ""
echo "  # Check zmap statistics"
echo "  curl -H \"X-Api-Key: ${API_KEY}\" http://localhost:8083/stats/zmap | jq"
echo ""
echo "  # Get a proxy"
echo "  curl -H \"X-Api-Key: ${API_KEY}\" http://localhost:8083/get-proxy"
echo ""
echo "  # Get 10 proxies"
echo "  curl -H \"X-Api-Key: ${API_KEY}\" 'http://localhost:8083/get-proxy?limit=10'"
echo ""
echo "  # Trigger manual reload"
echo "  curl -X POST -H \"X-Api-Key: ${API_KEY}\" http://localhost:8083/reload"
echo ""

echo -e "${BLUE}Useful Commands:${NC}"
echo ""
echo "  # View logs"
echo "  docker compose logs -f proxy-checker"
echo ""
echo "  # Restart service"
echo "  docker compose restart"
echo ""
echo "  # Stop service"
echo "  docker compose down"
echo ""
echo "  # Start service"
echo "  docker compose up -d"
echo ""

echo -e "${BLUE}🚀 Zmap Integration:${NC}"
echo -e "  • Zmap is ${GREEN}ENABLED${NC} by default (port 8080 only)"
echo -e "  • Scan time: ${GREEN}5 minutes${NC} per cycle (repeats every 15 min)"
echo -e "  • Rate: 5000 pps, optimized for quality"
echo -e "  • Expected: ${GREEN}~300-500 fresh proxies per scan!${NC}"

echo ""
echo -e "${BLUE}⚡ Performance Optimizations:${NC}"
echo -e "  • HTTP concurrency: ${GREEN}8000${NC} (adaptive, max 70% CPU)"
echo -e "  • Fast filter: ${GREEN}10k${NC} TCP connections (1.5s timeout)"
echo -e "  • Batch size: ${GREEN}1000${NC} proxies per batch"
echo -e "  • SOCKS check: ${GREEN}500${NC} concurrent (TCP-only, fast)"
echo -e "  • Memory usage: ${GREEN}optimized${NC} for high throughput"
echo ""
echo -e "${YELLOW}Note:${NC} Wait 1-2 minutes for the first proxy check cycle to complete."
echo "Then test the API endpoints above."
echo ""
echo -e "For monitoring: ${BLUE}docker compose --profile monitoring up -d${NC}"
echo -e "Then access Grafana at: ${BLUE}http://localhost:8088${NC} (admin/admin)"
echo ""
echo -e "For more information, see:"
echo -e "  • ${BLUE}README.md${NC} - General documentation"
echo -e "  • ${BLUE}ZMAP_QUICKSTART.md${NC} - Zmap quick start guide"
echo -e "  • ${BLUE}ZMAP_INTEGRATION_COMPLETE.md${NC} - Integration details"
echo ""
echo -e "${YELLOW}⚠️  LEGAL WARNING:${NC}"
echo "Network scanning may be illegal without authorization."
echo "By default, zmap will scan your configured target ranges only."
echo "See ZMAP_INTEGRATION_SUMMARY.md for legal guidelines."
echo ""

