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

# ── Check prerequisites ──────────────────────────────────────────────

info "Checking prerequisites"

command -v go >/dev/null 2>&1 || fail "Go is not installed. Install it from https://go.dev/dl/"
ok "Go $(go version | awk '{print $3}' | sed 's/go//')"

# ── Build ─────────────────────────────────────────────────────────────

info "Building helm from source"

cd "$SCRIPT_DIR"
go build -ldflags="-s -w" -o "$BINARY_NAME" .
ok "Built $(pwd)/$BINARY_NAME"

# ── Install ───────────────────────────────────────────────────────────

info "Installing to $INSTALL_DIR"

mkdir -p "$INSTALL_DIR"
mv "$BINARY_NAME" "$INSTALL_DIR/$BINARY_NAME"
chmod +x "$INSTALL_DIR/$BINARY_NAME"
ok "Installed $INSTALL_DIR/$BINARY_NAME"

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
            # Avoid failing the install if the user's RC file isn't writable
            can_write=false
            if [[ -e "$RC_FILE" ]]; then
                [[ -w "$RC_FILE" ]] && can_write=true
            else
                [[ -w "$(dirname "$RC_FILE")" ]] && can_write=true
            fi

            if [[ "$can_write" == true ]]; then
                {
                    echo ""
                    echo "# Added by helm installer"
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

# ── Verify ────────────────────────────────────────────────────────────

echo ""
if command -v helm >/dev/null 2>&1; then
    info "Ready to go!"
    echo ""
    echo "  ${BOLD}helm${RESET}              open the interactive REPL"
    echo "  ${BOLD}helm -a${RESET} <task>     run agent mode (autonomous)"
    echo "  ${BOLD}helm -e${RESET} <query>    generate a single command"
    echo "  ${BOLD}helm -c${RESET} <question> chat with the AI"
    echo ""
    echo "  Press ${BOLD}tab${RESET} inside the REPL to switch modes (exec / chat / agent)"
else
    info "Almost ready!"
    echo ""
    echo "  Run ${BOLD}source $RC_FILE${RESET} or restart your terminal, then:"
    echo ""
    echo "  ${BOLD}helm${RESET}              open the interactive REPL"
    echo "  ${BOLD}helm -a${RESET} <task>     run agent mode (autonomous)"
    echo "  ${BOLD}helm -e${RESET} <query>    generate a single command"
    echo "  ${BOLD}helm -c${RESET} <question> chat with the AI"
    echo ""
    echo "  Press ${BOLD}tab${RESET} inside the REPL to switch modes (exec / chat / agent)"
fi
