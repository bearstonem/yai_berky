#!/usr/bin/env bash
set -euo pipefail

BLUE='\033[0;34m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BOLD='\033[1m'
RESET='\033[0m'

info()  { echo -e "${BLUE}==>${RESET} ${BOLD}$1${RESET}"; }
ok()    { echo -e "${GREEN} ✓${RESET}  $1"; }
warn()  { echo -e "${YELLOW} !${RESET}  $1"; }
fail()  { echo -e "${RED} ✗${RESET}  $1"; exit 1; }

INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"
BINARY_NAME="helm"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

KERNEL=$(uname -s)
case "$KERNEL" in
    Linux)  OS="linux" ;;
    Darwin) OS="darwin" ;;
    *)      fail "Unsupported OS: $KERNEL" ;;
esac

# Detect package manager (Linux)
detect_pkg_manager() {
    if command -v apt-get &>/dev/null; then
        echo "apt"
    elif command -v dnf &>/dev/null; then
        echo "dnf"
    elif command -v yum &>/dev/null; then
        echo "yum"
    elif command -v pacman &>/dev/null; then
        echo "pacman"
    elif command -v apk &>/dev/null; then
        echo "apk"
    elif command -v zypper &>/dev/null; then
        echo "zypper"
    else
        echo ""
    fi
}

pkg_install() {
    local pkg_apt="${1:-}" pkg_dnf="${2:-$1}" pkg_pacman="${3:-$1}" pkg_apk="${4:-$1}" pkg_zypper="${5:-$1}"
    local mgr
    mgr=$(detect_pkg_manager)
    case "$mgr" in
        apt)     sudo apt-get update -qq && sudo apt-get install -y -qq "$pkg_apt" ;;
        dnf)     sudo dnf install -y "$pkg_dnf" ;;
        yum)     sudo yum install -y "$pkg_dnf" ;;
        pacman)  sudo pacman -S --noconfirm "$pkg_pacman" ;;
        apk)     sudo apk add --no-cache "$pkg_apk" ;;
        zypper)  sudo zypper install -y "$pkg_zypper" ;;
        *)       return 1 ;;
    esac
}

# ── Check prerequisites ──────────────────────────────────────────────

info "Checking prerequisites"

# Git
if ! command -v git &>/dev/null; then
    warn "Git not found. Installing..."
    if [ "$OS" = "darwin" ]; then
        if command -v brew &>/dev/null; then
            brew install git
        else
            warn "Installing Xcode Command Line Tools (includes git)..."
            xcode-select --install 2>/dev/null || true
            echo "    Waiting for Xcode CLT installation to complete..."
            until command -v git &>/dev/null; do sleep 5; done
        fi
    else
        pkg_install git git git git git || fail "Could not install git automatically. Install it manually and re-run."
    fi
fi
if command -v git &>/dev/null; then
    ok "Git $(git --version | sed 's/git version //')"
else
    warn "Git was installed but is not yet on PATH. You may need to restart your terminal."
fi

# Go
if ! command -v go &>/dev/null; then
    warn "Go not found. Installing..."
    if [ "$OS" = "darwin" ]; then
        if command -v brew &>/dev/null; then
            brew install go
        else
            fail "Go is not installed and Homebrew is not available.\n  Install Go from https://go.dev/dl/ or install Homebrew first."
        fi
    else
        # Try package manager first
        installed=false
        pkg_install golang golang go go go && installed=true

        # If package manager version is too old or unavailable, use official tarball
        if [ "$installed" = false ] || ! command -v go &>/dev/null; then
            warn "Installing Go from official tarball..."
            ARCH=$(uname -m)
            case "$ARCH" in
                x86_64)       GOARCH="amd64" ;;
                aarch64|arm64) GOARCH="arm64" ;;
                armv7*|armv6*) GOARCH="armv6l" ;;
                *)            fail "Unsupported architecture: $ARCH" ;;
            esac
            GO_VERSION=$(curl -fsSL "https://go.dev/VERSION?m=text" | head -1)
            GO_TAR="${GO_VERSION}.linux-${GOARCH}.tar.gz"
            curl -fsSL "https://go.dev/dl/$GO_TAR" -o "/tmp/$GO_TAR"
            sudo rm -rf /usr/local/go
            sudo tar -C /usr/local -xzf "/tmp/$GO_TAR"
            rm -f "/tmp/$GO_TAR"
            export PATH="/usr/local/go/bin:$PATH"
        fi
    fi
fi
if ! command -v go &>/dev/null; then
    fail "Go was installed but is not on PATH. Restart your terminal and re-run."
fi
ok "Go $(go version | awk '{print $3}' | sed 's/go//')"

# C compiler
cc_found=false
for cc in gcc clang cc; do
    if command -v "$cc" &>/dev/null; then
        ok "C compiler: $cc"
        cc_found=true
        break
    fi
done

if [ "$cc_found" = false ]; then
    warn "No C compiler found. Installing..."
    if [ "$OS" = "darwin" ]; then
        if ! xcode-select -p &>/dev/null; then
            warn "Installing Xcode Command Line Tools..."
            xcode-select --install 2>/dev/null || true
            echo "    Waiting for Xcode CLT installation to complete..."
            until command -v clang &>/dev/null; do sleep 5; done
        fi
    else
        pkg_install build-essential gcc base-devel build-base gcc || fail "Could not install C compiler. Install gcc manually and re-run."
    fi
    for cc in gcc clang cc; do
        if command -v "$cc" &>/dev/null; then
            ok "C compiler: $cc"
            cc_found=true
            break
        fi
    done
    if [ "$cc_found" = false ]; then
        fail "C compiler installation failed. Install gcc or clang manually and re-run."
    fi
fi

# SQLite3 development headers
sqlite3_found=false
if pkg-config --exists sqlite3 2>/dev/null; then
    sqlite3_found=true
elif [ -f /usr/include/sqlite3.h ] || [ -f /usr/local/include/sqlite3.h ]; then
    sqlite3_found=true
elif [ "$OS" = "darwin" ]; then
    # macOS includes sqlite3 with Xcode CLT / SDK
    if [ -f "$(xcrun --show-sdk-path 2>/dev/null)/usr/include/sqlite3.h" ] 2>/dev/null; then
        sqlite3_found=true
    fi
fi

if [ "$sqlite3_found" = true ]; then
    ok "SQLite3 development headers"
else
    warn "SQLite3 dev headers not found. Installing..."
    if [ "$OS" = "darwin" ]; then
        if command -v brew &>/dev/null; then
            brew install sqlite3
        else
            warn "Install Xcode CLT for sqlite3 headers: xcode-select --install"
        fi
    else
        pkg_install libsqlite3-dev sqlite-devel sqlite sqlite-dev sqlite3-devel \
            || warn "Could not install sqlite3 headers automatically."
    fi

    # Re-check
    if pkg-config --exists sqlite3 2>/dev/null; then
        ok "Installed SQLite3 development headers"
    elif [ -f /usr/include/sqlite3.h ] || [ -f /usr/local/include/sqlite3.h ]; then
        ok "Installed SQLite3 development headers"
    else
        warn "SQLite3 dev headers may still be missing. If the build fails, install manually:"
        echo "    sudo apt-get install libsqlite3-dev   # Debian/Ubuntu"
        echo "    sudo dnf install sqlite-devel          # Fedora/RHEL"
        echo "    brew install sqlite3                   # macOS"
    fi
fi

# ── Build ─────────────────────────────────────────────────────────────

info "Building Helm from source"

cd "$SCRIPT_DIR"

# On macOS with Homebrew sqlite3, help CGO find the headers
if [ "$OS" = "darwin" ] && [ -d "$(brew --prefix sqlite3 2>/dev/null)/include" ] 2>/dev/null; then
    SQLITE_PREFIX="$(brew --prefix sqlite3)"
    export CGO_CFLAGS="-I${SQLITE_PREFIX}/include"
    export CGO_LDFLAGS="-L${SQLITE_PREFIX}/lib"
fi

CGO_ENABLED=1 go build -ldflags="-s -w" -o "$BINARY_NAME" .
ok "Built $(pwd)/$BINARY_NAME"

# ── Install ───────────────────────────────────────────────────────────

info "Installing to $INSTALL_DIR"

mkdir -p "$INSTALL_DIR"
mv "$BINARY_NAME" "$INSTALL_DIR/$BINARY_NAME"
chmod +x "$INSTALL_DIR/$BINARY_NAME"
ok "Installed $INSTALL_DIR/$BINARY_NAME"

# ── Clean up old yai binary if present ────────────────────────────────

if [[ -f "$INSTALL_DIR/yai" ]]; then
    rm -f "$INSTALL_DIR/yai"
    ok "Removed old yai binary"
fi

# ── Ensure it's on PATH ──────────────────────────────────────────────

if echo "$PATH" | tr ':' '\n' | grep -qx "$INSTALL_DIR"; then
    ok "$INSTALL_DIR is already on PATH"
else
    warn "$INSTALL_DIR is not on your PATH"

    SHELL_NAME="$(basename "$SHELL")"
    case "$SHELL_NAME" in
        zsh)  RC_FILE="$HOME/.zshrc" ;;
        bash)
            if [[ -f "$HOME/.bashrc" ]]; then
                RC_FILE="$HOME/.bashrc"
            else
                RC_FILE="$HOME/.profile"
            fi
            ;;
        fish) RC_FILE="$HOME/.config/fish/config.fish" ;;
        *)    RC_FILE="" ;;
    esac

    if [[ -n "$RC_FILE" ]]; then
        if [[ "$SHELL_NAME" == "fish" ]]; then
            LINE="fish_add_path $INSTALL_DIR"
        else
            LINE="export PATH=\"$INSTALL_DIR:\$PATH\""
        fi

        if [[ -f "$RC_FILE" ]] && grep -qF "$INSTALL_DIR" "$RC_FILE" 2>/dev/null; then
            ok "PATH entry already exists in $RC_FILE"
        else
            can_write=false
            if [[ -e "$RC_FILE" ]]; then
                [[ -w "$RC_FILE" ]] && can_write=true
            else
                [[ -w "$(dirname "$RC_FILE")" ]] && can_write=true
            fi

            if [[ "$can_write" == true ]]; then
                {
                    echo ""
                    echo "# Added by Helm installer"
                    echo "$LINE"
                } >> "$RC_FILE"
                ok "Added $INSTALL_DIR to PATH in $RC_FILE"
                warn "Run: source $RC_FILE  (or restart your terminal)"
            else
                warn "Could not write to $RC_FILE. Add this to your shell config manually:"
                echo "    $LINE"
            fi
        fi
    else
        warn "Could not detect shell RC file. Add this to your shell config manually:"
        echo "    export PATH=\"$INSTALL_DIR:\$PATH\""
    fi
fi

# ── Migrate data from yai if needed (one-time) ───────────────────────

MIGRATION_MARKER="$HOME/.config/helm/.migrated_from_yai"
if [[ ! -f "$MIGRATION_MARKER" ]] && [[ -d "$HOME/.config/yai" || -f "$HOME/.config/yai.json" ]]; then
    info "Migrating data from yai → helm (one-time)"

    if [[ -f "$HOME/.config/yai.json" ]] && [[ ! -f "$HOME/.config/helm.json" ]]; then
        cp "$HOME/.config/yai.json" "$HOME/.config/helm.json"
        ok "Migrated config"
    fi

    for subdir in sessions skills agents; do
        src="$HOME/.config/yai/$subdir"
        dst="$HOME/.config/helm/$subdir"
        if [[ -d "$src" ]] && [[ -n "$(ls -A "$src" 2>/dev/null)" ]]; then
            mkdir -p "$dst"
            cp -rn "$src"/* "$dst"/ 2>/dev/null && ok "Migrated $subdir"
        fi
    done

    if [[ -f "$HOME/.config/yai/memory.db" ]] && [[ ! -f "$HOME/.config/helm/memory.db" ]]; then
        mkdir -p "$HOME/.config/helm"
        cp "$HOME/.config/yai/memory.db" "$HOME/.config/helm/memory.db"
        ok "Migrated memory.db"
    fi

    mkdir -p "$HOME/.config/helm"
    touch "$MIGRATION_MARKER"
    ok "Migration complete (won't run again)"
fi

# ── Verify ─────────────────────────────��──────────────────────────────

echo ""
if command -v helm >/dev/null 2>&1; then
    info "✦ Helm is ready!"
    echo ""
    echo -e "  ${BOLD}helm --setup${RESET}          first-time setup wizard (start here!)"
    echo -e "  ${BOLD}helm${RESET}                  interactive REPL"
    echo -e "  ${BOLD}helm --gui${RESET}            web GUI (agents, skills, settings)"
    echo -e "  ${BOLD}helm -a${RESET} <task>         agent mode (autonomous)"
    echo -e "  ${BOLD}helm -c${RESET} <question>     chat with the AI"
    echo -e "  ${BOLD}helm -e${RESET} <query>        generate a single command"
    echo -e "  ${BOLD}helm --pipe -a${RESET} <task>   headless mode (no TUI, for scripts)"
    echo ""
    echo -e "  Press ${BOLD}tab${RESET} inside the REPL to switch modes (▶ exec / 📡 chat / 🖖 agent)"
else
    info "Almost ready!"
    echo ""
    echo -e "  Run ${BOLD}source ${RC_FILE:-~/.profile}${RESET} or restart your terminal, then:"
    echo ""
    echo -e "  ${BOLD}helm --setup${RESET}          first-time setup wizard (start here!)"
    echo -e "  ${BOLD}helm${RESET}                  interactive REPL"
    echo -e "  ${BOLD}helm --gui${RESET}            web GUI (agents, skills, settings)"
    echo -e "  ${BOLD}helm -a${RESET} <task>         agent mode (autonomous)"
    echo -e "  ${BOLD}helm -c${RESET} <question>     chat with the AI"
    echo -e "  ${BOLD}helm -e${RESET} <query>        generate a single command"
    echo -e "  ${BOLD}helm --pipe -a${RESET} <task>   headless mode (no TUI, for scripts)"
    echo ""
    echo -e "  Press ${BOLD}tab${RESET} inside the REPL to switch modes (▶ exec / 📡 chat / 🖖 agent)"
fi
