#!/bin/sh
# This script installs modelrouter on Linux
# It detects OS and architecture, downloads the binary, and installs it to ~/.local/bin

set -eu

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m'
RESET='\033[0m'

# Installation directory - user local bin
INSTALL_DIR="${HOME}/.local/bin"

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

# Ensure ~/.local/bin exists and is in PATH
if [ ! -d "$INSTALL_DIR" ]; then
    info "Creating $INSTALL_DIR..."
    mkdir -p "$INSTALL_DIR"
fi

# Add to PATH if not already there
case ":$PATH:" in
    *":$INSTALL_DIR:"*) ;;
    *) export PATH="$INSTALL_DIR:$PATH" ;;
esac

# Get latest version from GitHub API
info "Fetching latest release..."
VERSION=$(curl -s https://api.github.com/repos/macedot/modelrouter/releases/latest | grep -o '"tag_name": "[0-9]*' | sed 's/"//g')

if [ -z "$VERSION" ]; then
    error "Failed to fetch version from GitHub"
fi

# Allow version override
if [ -n "$MODELROUTER_VERSION" ]; then
    VERSION="$MODELROUTER_VERSION"
    info "Using MODELROUTER_VERSION=$VERSION"
fi

# Download URL
DOWNLOAD_URL="https://github.com/macedot/modelrouter/releases/download/${VERSION}/modelrouter-linux-${ARCH}"

# Download binary
info "Downloading modelrouter ${VERSION} for ${ARCH}..."
if ! curl -fsSL --progress-bar -o "$INSTALL_DIR/modelrouter" "$DOWNLOAD_URL"; then
    error "Failed to download modelrouter"
fi

# Make executable
chmod +x "$INSTALL_DIR/modelrouter"

# Create systemd service (optional, for user-level service)
if [ -d "${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user" ]; then
    info "Creating user systemd service..."
    mkdir -p "${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"
    cat <<EOF > "${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user/modelrouter.service"
[Unit]
Description=modelrouter API proxy server
After=network.target.wants network-online.target

[Service]
Type=simple
ExecStart=$INSTALL_DIR/modelrouter
Restart=on-failure
RestartSec=3

[Install]
WantedBy=default.target
EOF

    info "User systemd service created. Enable with: systemctl --user enable modelrouter"
fi

# Update README
info "Updating README..."

# Success message
echo ""
info "Installation complete!"
info ""
info "modelrouter has been installed to $INSTALL_DIR/modelrouter"
info ""
if [ -d "${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user" ]; then
    info "To start the server:"
    info "  systemctl --user start modelrouter"
    info ""
    info "To check status:"
    info "  systemctl --user status modelrouter"
    info ""
    info "To start on login:"
    info "  systemctl --user enable modelrouter"
fi
info ""
info "Make sure $INSTALL_DIR is in your PATH."
info "Run 'modelrouter' to start the server."
echo ""
