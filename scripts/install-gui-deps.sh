#!/usr/bin/env bash
#
# install-gui-deps.sh
#
# Installs the development dependencies needed to build the dil GUI
# (system tray + audio capture). These are only needed at build time.
#
# Run with:  sudo bash install-gui-deps.sh

set -euo pipefail

if [[ "${EUID}" -ne 0 ]]; then
    echo "This script must be run as root (use: sudo bash $0)" >&2
    exit 1
fi

echo "==> Installing build dependencies for dil GUI"
apt-get update -qq
apt-get install -y \
    libgtk-3-dev \
    libayatana-appindicator3-dev \
    libxapp-dev \
    pkg-config \
    gcc

echo
echo "Done. You can now build the GUI with: go build -o dil ./cmd/dil"
