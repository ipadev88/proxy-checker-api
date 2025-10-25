#!/bin/bash

set -e

echo "╔═══════════════════════════════════════════════════════════╗"
echo "║    Proxy Checker API - Quick Start Setup                 ║"
echo "╚═══════════════════════════════════════════════════════════╝"
echo ""

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Check if running as root
if [ "$EUID" -eq 0 ]; then
    echo -e "${RED}Error: Do not run this script as root${NC}"
    echo "Run as normal user. It will prompt for sudo when needed."
    exit 1
fi

# Check prerequisites
echo "Checking prerequisites..."

# Check Docker
if command -v docker &> /dev/null; then
    echo -e "${GREEN}✓${NC} Docker found: $(docker --version)"
    DOCKER_AVAILABLE=1
else
    echo -e "${YELLOW}⚠${NC} Docker not found (optional)"
    DOCKER_AVAILABLE=0
fi

# Check Go
if command -v go &> /dev/null; then
    echo -e "${GREEN}✓${NC} Go found: $(go version)"
    GO_AVAILABLE=1
else
    echo -e "${YELLOW}⚠${NC} Go not found (required for build from source)"
    GO_AVAILABLE=0
fi

echo ""
echo "Select installation method:"
echo "  1) Docker Compose (recommended)"
echo "  2) Build from source"
echo "  3) System tuning only"
read -p "Choice [1]: " INSTALL_METHOD
INSTALL_METHOD=${INSTALL_METHOD:-1}

# Docker installation
if [ "$INSTALL_METHOD" -eq 1 ]; then
    if [ $DOCKER_AVAILABLE -eq 0 ]; then
        echo -e "${RED}Error: Docker not available${NC}"
        exit 1
    fi

    echo ""
    echo "Setting up Docker environment..."

    # Copy config
    if [ ! -f config.json ]; then
        cp config.example.json config.json
        echo -e "${GREEN}✓${NC} Created config.json"
    fi

    # Create .env file
    if [ ! -f .env ]; then
        API_KEY=$(openssl rand -hex 16 2>/dev/null || cat /dev/urandom | tr -dc 'a-zA-Z0-9' | fold -w 32 | head -n 1)
        cat > .env <<EOF
PROXY_API_KEY=${API_KEY}
TZ=UTC
EOF
        echo -e "${GREEN}✓${NC} Created .env with API key: ${API_KEY}"
        echo -e "${YELLOW}⚠${NC} Save this API key! You'll need it to access the API."
    fi

    echo ""
    echo "Building and starting services..."
    docker-compose up -d

    echo ""
    echo -e "${GREEN}✓${NC} Docker services started!"
    echo ""
    echo "Service URLs:"
    echo "  • API:        http://localhost:8080"
    echo "  • Health:     http://localhost:8080/health"
    echo "  • Metrics:    http://localhost:8080/metrics"
    echo ""
    echo "To view logs:"
    echo "  docker-compose logs -f"
    echo ""
    echo "To get your API key:"
    echo "  cat .env | grep PROXY_API_KEY"

# Build from source
elif [ "$INSTALL_METHOD" -eq 2 ]; then
    if [ $GO_AVAILABLE -eq 0 ]; then
        echo -e "${RED}Error: Go not available${NC}"
        exit 1
    fi

    echo ""
    echo "Building from source..."

    # Install dependencies
    echo "Downloading dependencies..."
    go mod download

    # Build
    echo "Compiling..."
    mkdir -p build
    CGO_ENABLED=1 go build -o build/proxy-checker ./cmd/main.go

    # Setup config
    if [ ! -f config.json ]; then
        cp config.example.json config.json
        echo -e "${GREEN}✓${NC} Created config.json"
    fi

    # Generate API key
    API_KEY=$(openssl rand -hex 16 2>/dev/null || cat /dev/urandom | tr -dc 'a-zA-Z0-9' | fold -w 32 | head -n 1)
    export PROXY_API_KEY="${API_KEY}"

    echo -e "${GREEN}✓${NC} Build complete!"
    echo ""
    echo "Your API key: ${API_KEY}"
    echo ""
    echo "To run the service:"
    echo "  export PROXY_API_KEY=\"${API_KEY}\""
    echo "  ./build/proxy-checker"

# System tuning only
elif [ "$INSTALL_METHOD" -eq 3 ]; then
    echo ""
    echo "Applying system tuning..."
    
    # File descriptors
    echo "Current ulimit: $(ulimit -n)"
    ulimit -n 65535 2>/dev/null && echo -e "${GREEN}✓${NC} Set ulimit to 65535" || echo -e "${YELLOW}⚠${NC} Could not set ulimit (try: ulimit -n 65535)"

    # TCP tuning
    echo ""
    echo "Applying TCP tuning (requires sudo)..."
    sudo sysctl -w net.ipv4.ip_local_port_range="10000 65535"
    sudo sysctl -w net.ipv4.tcp_max_syn_backlog=8192
    sudo sysctl -w net.ipv4.tcp_tw_reuse=1
    sudo sysctl -w net.core.somaxconn=8192
    
    echo -e "${GREEN}✓${NC} System tuning applied"
    echo ""
    echo "To make permanent, see OPS_CHECKLIST.md"
else
    echo -e "${RED}Invalid choice${NC}"
    exit 1
fi

echo ""
echo "═══════════════════════════════════════════════════════════"
echo ""
echo "Quick test (after service starts):"
echo ""
echo "  # Health check"
echo "  curl http://localhost:8080/health"
echo ""
echo "  # Get statistics"
echo '  curl -H "X-Api-Key: YOUR_KEY" http://localhost:8080/stat | jq'
echo ""
echo "  # Get a proxy"
echo '  curl -H "X-Api-Key: YOUR_KEY" http://localhost:8080/get-proxy'
echo ""
echo "For more information, see README.md"
echo ""

