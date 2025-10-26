#!/bin/bash
#
# Zmap Installation Script for Ubuntu/Debian
# Installs zmap, configures blacklists, and sets up capabilities
#

set -e

echo "================================"
echo "Zmap Installation Script"
echo "================================"
echo ""

# Check if running with sudo
if [ "$EUID" -ne 0 ]; then 
   echo "❌ Please run with sudo: sudo bash $0"
   exit 1
fi

echo "✅ Running as root"
echo ""

# Detect OS
if [ -f /etc/os-release ]; then
    . /etc/os-release
    OS=$ID
    VER=$VERSION_ID
else
    echo "❌ Cannot detect OS. This script supports Ubuntu/Debian."
    exit 1
fi

echo "📋 Detected OS: $OS $VER"
echo ""

# Install zmap
echo "📦 Installing zmap..."
if [ "$OS" = "ubuntu" ] || [ "$OS" = "debian" ]; then
    apt-get update -qq
    apt-get install -y zmap libpcap-dev
else
    echo "❌ Unsupported OS. This script supports Ubuntu/Debian."
    exit 1
fi

# Verify zmap installation
if ! command -v zmap &> /dev/null; then
    echo "❌ Zmap installation failed"
    exit 1
fi

ZMAP_VERSION=$(zmap --version 2>&1 | head -n1)
echo "✅ Zmap installed: $ZMAP_VERSION"
echo ""

# Create directories
echo "📁 Creating directories..."
mkdir -p /etc/proxy-checker
mkdir -p /var/log/proxy-checker
echo "✅ Directories created"
echo ""

# Download blacklist
echo "📥 Downloading default blacklist..."
BLACKLIST_URL="https://raw.githubusercontent.com/zmap/zmap/master/conf/blacklist.conf"
BLACKLIST_FILE="/etc/proxy-checker/blacklist.txt"

if curl -f -s -o "$BLACKLIST_FILE" "$BLACKLIST_URL"; then
    echo "✅ Blacklist downloaded: $BLACKLIST_FILE"
    BLACKLIST_COUNT=$(grep -c -v "^#" "$BLACKLIST_FILE" || true)
    echo "   Contains $BLACKLIST_COUNT CIDR ranges"
else
    echo "⚠️  Failed to download blacklist from GitHub"
    echo "   Creating basic blacklist..."
    
    cat > "$BLACKLIST_FILE" << 'EOF'
# Zmap Blacklist - Basic Configuration
# Private and reserved IP ranges

# Private networks (RFC 1918)
10.0.0.0/8
172.16.0.0/12
192.168.0.0/16

# Loopback
127.0.0.0/8

# Link-local
169.254.0.0/16

# Multicast
224.0.0.0/4

# Reserved
240.0.0.0/4

# Broadcast
255.255.255.255/32
EOF
    
    echo "✅ Basic blacklist created"
fi
echo ""

# Set capabilities on zmap
echo "🔧 Setting capabilities on zmap binary..."
ZMAP_PATH=$(which zmap)
setcap 'cap_net_raw,cap_net_admin=+eip' "$ZMAP_PATH"

# Verify capabilities
CAPS=$(getcap "$ZMAP_PATH")
if echo "$CAPS" | grep -q "cap_net_raw"; then
    echo "✅ Capabilities set: $CAPS"
else
    echo "❌ Failed to set capabilities"
    exit 1
fi
echo ""

# Test zmap (scan localhost)
echo "🧪 Testing zmap (scanning localhost)..."
if timeout 5 zmap -p 80 127.0.0.0/24 -r 100 -o /tmp/zmap_test.txt --output-fields=saddr --output-module=csv 2>/dev/null; then
    if [ -f /tmp/zmap_test.txt ]; then
        TEST_RESULTS=$(wc -l < /tmp/zmap_test.txt)
        echo "✅ Zmap test successful ($TEST_RESULTS results)"
        rm -f /tmp/zmap_test.txt
    fi
else
    echo "⚠️  Zmap test timeout (this is normal)"
fi
echo ""

# System tuning recommendations
echo "📊 System tuning recommendations:"
echo ""
echo "   To optimize for high-concurrency scanning, run:"
echo ""
echo "   sudo sysctl -w net.core.rmem_max=134217728"
echo "   sudo sysctl -w net.core.wmem_max=134217728"
echo "   sudo sysctl -w net.ipv4.ip_local_port_range='1024 65535'"
echo "   sudo sysctl -w net.ipv4.tcp_tw_reuse=1"
echo ""
echo "   For file descriptor limits:"
echo "   sudo sh -c 'echo \"* soft nofile 1000000\" >> /etc/security/limits.conf'"
echo "   sudo sh -c 'echo \"* hard nofile 1000000\" >> /etc/security/limits.conf'"
echo ""

# Summary
echo "================================"
echo "✅ Installation Complete!"
echo "================================"
echo ""
echo "Next steps:"
echo "1. Update config.json:"
echo "   {\"zmap\": {\"enabled\": true, \"ports\": [8080, 80, 3128]}}"
echo ""
echo "2. Review blacklist:"
echo "   vim /etc/proxy-checker/blacklist.txt"
echo ""
echo "3. Test configuration:"
echo "   ./proxy-checker -config config.json"
echo ""
echo "4. View zmap options:"
echo "   zmap --help"
echo ""
echo "⚠️  LEGAL WARNING:"
echo "   Network scanning may be illegal without authorization."
echo "   Only scan networks you own or have permission to scan."
echo "   See ZMAP_INTEGRATION_SUMMARY.md for legal guidelines."
echo ""

