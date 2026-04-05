#!/usr/bin/env bash
set -euo pipefail

IMAGE_NAME="helm:latest"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"

# ── Helpers ──────────────────────────────────────────────────────────

info()  { printf '\033[1;34m==> \033[0;1m%s\033[0m\n' "$1"; }
ok()    { printf '\033[1;32m +  \033[0m%s\n' "$1"; }
warn()  { printf '\033[1;33m !  \033[0m%s\n' "$1"; }
fail()  { printf '\033[1;31m X  \033[0m%s\n' "$1" >&2; exit 1; }

KERNEL=$(uname -s)
case "$KERNEL" in
    Linux)  OS="linux" ;;
    Darwin) OS="darwin" ;;
    *)      fail "Unsupported OS: $KERNEL" ;;
esac

# ── Check prerequisites ─────────────────────────────────────────────

info "Checking prerequisites"

# Docker
if ! command -v docker &>/dev/null; then
    if [ "$OS" = "linux" ]; then
        warn "Docker not found. Installing via official install script..."
        curl -fsSL https://get.docker.com | sh
        if command -v docker &>/dev/null; then
            ok "Installed Docker"
            # Add current user to docker group so sudo isn't needed
            if [ "$(id -u)" -ne 0 ] && ! groups | grep -qw docker; then
                sudo usermod -aG docker "$USER" 2>/dev/null || true
                warn "Added $USER to docker group. You may need to log out and back in for this to take effect."
            fi
        else
            fail "Docker installation failed. Install manually: https://docs.docker.com/engine/install/"
        fi
    elif [ "$OS" = "darwin" ]; then
        if command -v brew &>/dev/null; then
            warn "Docker not found. Installing Docker Desktop via Homebrew..."
            brew install --cask docker
            if [ -d "/Applications/Docker.app" ]; then
                ok "Installed Docker Desktop"
            else
                fail "Docker Desktop installation failed. Install manually: https://www.docker.com/products/docker-desktop/"
            fi
        else
            fail "Docker is not installed and Homebrew is not available to install it automatically.\n  Install Docker Desktop from https://www.docker.com/products/docker-desktop/"
        fi
    fi
fi

if ! command -v docker &>/dev/null; then
    fail "Docker was installed but is not on PATH. Restart your terminal and re-run this script."
fi
ok "Docker $(docker --version | sed 's/Docker version //' | sed 's/,.*//')"

# Verify Docker daemon is running — try to start if not
if ! docker info &>/dev/null; then
    if [ "$OS" = "darwin" ]; then
        warn "Docker daemon is not running. Starting Docker Desktop..."
        open -a Docker
    elif [ "$OS" = "linux" ]; then
        warn "Docker daemon is not running. Attempting to start..."
        sudo systemctl start docker 2>/dev/null || sudo service docker start 2>/dev/null || true
    fi

    retries=30
    running=false
    for (( i=0; i<retries; i++ )); do
        if docker info &>/dev/null; then
            running=true
            break
        fi
        printf "."
        sleep 2
    done
    echo ""

    if [ "$running" = false ]; then
        fail "Docker daemon did not start after 60 seconds. Start it manually and re-run this script."
    fi
fi
ok "Docker daemon is running"

# ── Build image ─────────────────────────────────────────────────────

info "Building Docker image"

cd "$SCRIPT_DIR"
docker build -t "$IMAGE_NAME" .
ok "Built image $IMAGE_NAME"

# ── Create wrapper script ───────────────────────────────────────────

info "Creating wrapper script"

ENV_FLAG=""
if [ -f "$SCRIPT_DIR/.env" ]; then
    ENV_FLAG="--env-file \"$SCRIPT_DIR/.env\""
fi

WRAPPER="$INSTALL_DIR/helm"
WRAPPER_CONTENT="#!/usr/bin/env bash
exec docker run --rm -it \\
    -v helm-data:/root/.config/helm \\
    $ENV_FLAG \\
    -p 8080:8080 \\
    $IMAGE_NAME \"\$@\"
"

if [ -w "$INSTALL_DIR" ]; then
    printf '%s' "$WRAPPER_CONTENT" > "$WRAPPER"
    chmod +x "$WRAPPER"
else
    sudo tee "$WRAPPER" > /dev/null <<< "$WRAPPER_CONTENT"
    sudo chmod +x "$WRAPPER"
fi
ok "Created $WRAPPER"

# ── Verify PATH ─────────────────────────────────────────────────────

if ! echo "$PATH" | tr ':' '\n' | grep -qx "$INSTALL_DIR"; then
    warn "$INSTALL_DIR is not on your PATH. Add it to your shell profile:"
    echo "    export PATH=\"$INSTALL_DIR:\$PATH\""
else
    ok "$INSTALL_DIR is on PATH"
fi

# ── Done ────────────────────────────────────────────────────────────

echo ""
info "Helm (Docker) is ready!"
echo ""
echo "  helm --setup          first-time setup wizard (start here!)"
echo "  helm                  interactive REPL"
echo "  helm --gui            web GUI (agents, skills, settings)"
echo "  helm -a <task>        agent mode (autonomous)"
echo "  helm -c <question>    chat with the AI"
echo "  helm -e <query>       generate a single command"
echo ""
echo "  Data is persisted in Docker volume helm-data"
echo ""
