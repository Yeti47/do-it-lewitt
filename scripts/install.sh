#!/usr/bin/env bash
#
# install.sh
#
# Builds dilctl and dil, installs them to a bin directory, and optionally
# sets up dil as a systemd user service that starts on login.
#
# Run with:  ./scripts/install.sh        (user install)
#            sudo ./scripts/install.sh   (system install)
#
# Flags:
#   --no-gui          Skip building/installing dil (CLI only)
#   --no-autostart    Don't set up systemd autostart for dil
#   --uninstall       Remove installed binaries and service

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

NO_GUI=false
NO_AUTOSTART=false
UNINSTALL=false

for arg in "$@"; do
    case "$arg" in
        --no-gui)       NO_GUI=true ;;
        --no-autostart) NO_AUTOSTART=true ;;
        --uninstall)    UNINSTALL=true ;;
        *)
            echo "Unknown flag: $arg" >&2
            echo "Usage: $0 [--no-gui] [--no-autostart] [--uninstall]" >&2
            exit 1
            ;;
    esac
done

# --- Uninstall ---
if [[ "$UNINSTALL" == true ]]; then
    echo "==> Uninstalling do-it-lewitt"

    if [[ "$EUID" -eq 0 ]]; then
        rm -f /usr/local/bin/dilctl /usr/local/bin/dil
        echo "Removed binaries from /usr/local/bin/"
    else
        rm -f ~/.local/bin/dilctl ~/.local/bin/dil
        echo "Removed binaries from ~/.local/bin/"
    fi

    systemctl --user stop dil.service 2>/dev/null || true
    systemctl --user disable dil.service 2>/dev/null || true
    rm -f ~/.config/systemd/user/dil.service
    systemctl --user daemon-reload 2>/dev/null || true
    echo "Removed systemd user service"

    echo "Done."
    exit 0
fi

# --- Determine install prefix ---
if [[ "$EUID" -eq 0 ]]; then
    BIN_DIR="/usr/local/bin"
    SYSTEM_INSTALL=true
else
    BIN_DIR="$HOME/.local/bin"
    SYSTEM_INSTALL=false
    mkdir -p "$BIN_DIR"
fi

# --- Build ---
echo "==> Building dilctl (CLI)"
CGO_ENABLED=0 go build -o "$PROJECT_DIR/dilctl" ./cmd/dilctl
echo "Built: $PROJECT_DIR/dilctl"

if [[ "$NO_GUI" == false ]]; then
    echo "==> Building dil (GUI)"

    if ! pkg-config --exists ayatana-appindicator3-0.1 2>/dev/null; then
        echo "Warning: GUI build dependencies not found." >&2
        echo "Run: sudo bash scripts/install-gui-deps.sh" >&2
        echo "Then re-run this script." >&2
        echo "" >&2
        echo "Continuing with CLI-only install..." >&2
        NO_GUI=true
    else
        CGO_ENABLED=1 go build -o "$PROJECT_DIR/dil" ./cmd/dil
        echo "Built: $PROJECT_DIR/dil"
    fi
fi

# --- Install binaries ---
echo "==> Installing binaries to $BIN_DIR"

install -m 755 "$PROJECT_DIR/dilctl" "$BIN_DIR/dilctl"
echo "Installed: $BIN_DIR/dilctl"

if [[ "$NO_GUI" == false ]]; then
    install -m 755 "$PROJECT_DIR/dil" "$BIN_DIR/dil"
    echo "Installed: $BIN_DIR/dil"
fi

# --- PATH check (user install only) ---
if [[ "$SYSTEM_INSTALL" == false ]]; then
    case ":$PATH:" in
        *":$BIN_DIR:"*)
            ;;
        *)
            echo "" >&2
            echo "Warning: $BIN_DIR is not in your PATH." >&2
            echo "Add this to your ~/.bashrc or ~/.profile:" >&2
            echo "" >&2
            echo "    export PATH=\"\$HOME/.local/bin:\$PATH\"" >&2
            echo "" >&2
            echo "Then run: source ~/.bashrc" >&2
            ;;
    esac
fi

# --- Autostart (systemd user service for dil) ---
if [[ "$NO_GUI" == false ]] && [[ "$NO_AUTOSTART" == false ]]; then
    echo "==> Setting up systemd user service for dil"

    SERVICE_DIR="$HOME/.config/systemd/user"
    mkdir -p "$SERVICE_DIR"

    cat > "$SERVICE_DIR/dil.service" <<EOF
[Unit]
Description=Do it, Lewitt! — System Tray GUI
After=graphical-session.target

[Service]
Type=simple
ExecStart=$BIN_DIR/dil
Restart=always
RestartSec=5

[Install]
WantedBy=default.target
EOF

    systemctl --user daemon-reload
    rm -f "$SERVICE_DIR/graphical-session.target.wants/dil.service"
    systemctl --user enable dil.service
    systemctl --user start dil.service 2>/dev/null || true

    echo "Installed: $SERVICE_DIR/dil.service"
    echo "dil will auto-start on login. Current status:"
    systemctl --user is-active dil.service 2>&1 || true
fi

echo ""
echo "==> Done!"
echo ""
if [[ "$NO_GUI" == true ]]; then
    echo "CLI installed:  dilctl"
    echo "Run 'dilctl status' to check your device."
else
    echo "CLI installed:  dilctl"
    echo "GUI installed:  dil"
    if [[ "$NO_AUTOSTART" == false ]]; then
        echo "Autostart:      enabled (systemd user service)"
    fi
    echo ""
    echo "Run 'dilctl status' to check your device."
    echo "Run 'dil' to launch the GUI manually, or check the system tray."
fi
echo ""
echo "To uninstall:  $0 --uninstall"
