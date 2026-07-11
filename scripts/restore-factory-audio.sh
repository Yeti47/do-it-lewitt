#!/usr/bin/env bash
#
# restore-factory-audio.sh
#
# Reverts ALL custom audio plumbing and restores Linux Mint's factory
# PipeWire/WirePlumber setup.
#
# Run with:  sudo bash restore-factory-audio.sh

set -euo pipefail

if [[ "${EUID}" -ne 0 ]]; then
    echo "This script must be run as root (use: sudo bash $0)" >&2
    exit 1
fi

echo "==> 1/6 Unmasking PipeWire/WirePlumber units at global scope"
systemctl --user --global unmask \
    pipewire.service pipewire.socket \
    pipewire-pulse.service pipewire-pulse.socket \
    wireplumber.service 2>/dev/null || true

echo "==> 2/6 Re-enabling PipeWire/WirePlumber units at global scope"
systemctl --user --global enable \
    pipewire.service pipewire.socket \
    pipewire-pulse.service pipewire-pulse.socket \
    wireplumber.service 2>/dev/null || true

echo "==> 3/6 Reinstalling pipewire-alsa + pipewire-audio"
apt-get update -qq
apt-get install -y pipewire-alsa pipewire-audio

echo "==> 4/6 Removing custom /etc/asound.conf"
rm -f /etc/asound.conf

echo "==> 5/6 Removing any custom WirePlumber config"
rm -f /etc/wireplumber/wireplumber.lua.d/51-lewitt-ignore.lua 2>/dev/null || true
# Remove the directory only if empty; leave it if other files exist
rmdir /etc/wireplumber/wireplumber.lua.d 2>/dev/null || true

echo "==> 6/6 Reloading systemd"
systemctl --user --global daemon-reload 2>/dev/null || true

echo
echo "Done. Audio stack reverted to Linux Mint factory defaults."
echo
echo "Reboot (or log out and back in) for the user services to auto-start cleanly."
echo "Or start them now as your normal user:"
echo "  systemctl --user start pipewire.socket pipewire-pulse.socket wireplumber.service"
