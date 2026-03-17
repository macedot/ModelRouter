#!/bin/sh
# This script installs ModelRouter on Linux
# It detects OS and architecture, downloads the binary, and installs it

set -eu

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m'
RESET='\033[0m'

# Helper functions
info() {
    echo "${GREEN}>>> $*${NC}"
}

warn() {
    echo "${YELLOW}! $*${NC}"
}

error() {
    echo "${RED}ERROR: $*${NC}" >&2
    exit 1
}

# Check if running as root
if [ "$(id -u)" -eq 0 ]; then
    SUDO=""
else
    SUDO="sudo"
fi

# OS detection
OS="$(uname -s)"
if [ "$OS" != "Linux" ]; then
    error "This script only supports Linux. Detected: $OS"
fi

# Architecture detection
ARCH=$(uname -m)
case "$ARCH" in
    x86_64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) error "Unsupported architecture: $ARCH" ;;
esac

info "Detected Linux $ARCH"

# Get latest version from GitHub API
info "Fetching latest release..."
VERSION=$(curl -s https://api.github.com/repos/macedot/ModelRouter/releases/latest | grep -o '"tag_name": "[0-9]*' | sed 's/"//g')

if [ -z "$VERSION" ]; then
    error "Failed to fetch version from GitHub"
fi

# Allow version override
if [ -n "$OPENMODEL_VERSION" ]; then
    VERSION="$OPENMODEL_VERSION"
    info "Using OPENMODEL_VERSION=$VERSION"
fi

# Download URL
DOWNLOAD_URL="https://github.com/macedot/ModelRouter/releases/download/${VERSION}/ModelRouter-linux-${ARCH}"

# Download binary
info "Downloading ModelRouter ${VERSION} for ${ARCH}..."
if ! curl -fsSL --progress-bar -o /usr/local/bin/ModelRouter "$DOWNLOAD_URL"; then
    error "Failed to download ModelRouter"
fi

# Make executable
chmod +x /usr/local/bin/ModelRouter

# Create systemd service (optional)
if [ -d /etc/systemd/system ]; then
    info "Creating systemd service..."
    cat <<EOF | $SUDO tee /etc/systemd/system/ModelRouter.service > /dev/null
[Unit]
Description=ModelRouter API proxy server
After=network.target.wants network-online.target
[Service]
Type=simple
ExecStart=/usr/local/bin/ModelRouter
Restart=on-failure
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF
    
    info "Systemd service created. Enable with: sudo systemctl enable ModelRouter"
fi

# Update README
info "Updating README..."

# Success message
echo ""
info "Installation complete!"
info ""
info "ModelRouter hasVERSION} has been installed to /usr/local/bin/ModelRouter"
info ""
if [ -d /etc/systemd/system/ModelRouter.service ]; then
    info "To start the server:"
    info "  sudo systemctl start ModelRouter"
    info ""
    info "To check status:"
    info "  sudo systemctl status ModelRouter"
fi
info ""
info "Run 'ModelRouter' to start the server."
echo ""
