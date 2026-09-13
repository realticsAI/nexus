#!/bin/bash
set -euo pipefail

# Build distribution package: nexus binary + README in a tar.gz
# Usage: ./scripts/dist.sh

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
VERSION=$(git -C "$PROJECT_DIR" describe --tags --always --dirty 2>/dev/null || echo "dev")
ARCH=$(uname -m)
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
DIST_NAME="nexus-${VERSION}-${OS}-${ARCH}"
DIST_DIR="$PROJECT_DIR/dist/$DIST_NAME"

echo "=== Building Nexus Distribution ==="
echo "Version: $VERSION"
echo "Platform: $OS/$ARCH"
echo ""

# 1. Build binary
echo "[1/3] Building binary..."
cd "$PROJECT_DIR"
CGO_ENABLED=1 go build -ldflags "-X main.version=$VERSION" -o "$DIST_DIR/nexus" ./cmd/nexus
echo "     -> bin/nexus built"

# 2. Copy README + install script
echo "[2/3] Bundling files..."
cp "$PROJECT_DIR/README.md" "$DIST_DIR/"

# Create a lightweight install script that ships with the binary
cat > "$DIST_DIR/install.sh" << 'INSTALL_EOF'
#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
NEXUS_BIN="$SCRIPT_DIR/nexus"

if [ ! -f "$NEXUS_BIN" ]; then
    echo "Error: nexus binary not found in $SCRIPT_DIR"
    exit 1
fi

echo ""
echo "  ╔═══════════════════════════════════════╗"
echo "  ║         Nexus — Code Intelligence     ║"
echo "  ╚═══════════════════════════════════════╝"
echo ""
echo "  See README.md for full documentation."
echo ""

# Copy binary to ~/bin
mkdir -p "$HOME/bin"
cp "$NEXUS_BIN" "$HOME/bin/nexus"
chmod +x "$HOME/bin/nexus"
echo "  [1/2] Installed nexus to ~/bin/nexus"

# Ensure ~/bin is on PATH
if ! echo "$PATH" | grep -q "$HOME/bin"; then
    SHELL_RC="$HOME/.zshrc"
    [ -f "$HOME/.bashrc" ] && [ ! -f "$HOME/.zshrc" ] && SHELL_RC="$HOME/.bashrc"
    echo 'export PATH="$HOME/bin:$PATH"' >> "$SHELL_RC"
    export PATH="$HOME/bin:$PATH"
    echo "  [+] Added ~/bin to PATH in $(basename $SHELL_RC)"
fi

echo "  [2/2] Ready."
echo ""
echo "  Next steps:"
echo ""
echo "    1. Clone your repos under one directory:"
echo "       mkdir -p ~/my-repos && cd ~/my-repos"
echo "       gh repo clone my-org/service-a"
echo ""
echo "    2. Run one-click setup:"
echo "       nexus init ."
echo ""
echo "    3. Open a new Claude Code session and try:"
echo '       "Use nexus to search for UserService"'
echo ""
INSTALL_EOF
chmod +x "$DIST_DIR/install.sh"
echo "     -> README.md + install.sh bundled"

# 3. Create tar.gz
echo "[3/3] Creating archive..."
cd "$PROJECT_DIR/dist"
tar -czf "${DIST_NAME}.tar.gz" "$DIST_NAME"
SIZE=$(du -h "${DIST_NAME}.tar.gz" | cut -f1)
echo "     -> dist/${DIST_NAME}.tar.gz ($SIZE)"

echo ""
echo "=== Distribution Ready ==="
echo ""
echo "Share this with users:"
echo "  dist/${DIST_NAME}.tar.gz"
echo ""
echo "User runs:"
echo "  tar xzf ${DIST_NAME}.tar.gz"
echo "  cd ${DIST_NAME}"
echo "  ./install.sh          # Part 1: installs binary + shows README"
echo "  nexus init ~/repos    # Part 2: one-click setup"
echo ""
