# Do it, Lewitt!

A utility for making the [Lewitt CONNECT 2](https://www.lewitt-audio.com/connect-2) USB audio interface work correctly under Linux.

## The Problem

The Lewitt CONNECT 2 is a UAC2-compliant device that works under Linux, but it exposes its ALSA endpoints in a way that causes issues for reliable recording:

- **Capture** is exposed as a **4-channel** stream (FL FR FC LFE), but only channels 0 (FL) and 1 (FR) are the real XLR mic inputs. Channels 2 and 3 are spurious.
- **WirePlumber** (the PipeWire session manager) auto-selects the wrong profile and holds the device open, preventing direct ALSA access by applications like Audacity.

## The Solution

This utility:

1. **Installs an ALSA PCM** (`lewitt_connect_2`) that routes the 4-channel hardware capture down to the correct 2-channel (FL/FR) inputs, dropping the spurious channels.
2. **Installs a WirePlumber rule** that tells WirePlumber to ignore the Lewitt device entirely, keeping it free for direct ALSA access.
3. **Provides diagnostics and verification** to confirm the device is working correctly.

## Prerequisites

- Linux with ALSA and (optionally) PipeWire/WirePlumber
- Go 1.21+ (for building from source)
- The Lewitt CONNECT 2 plugged in

## Quick Start (End Users)

### 1. Install

```sh
./scripts/install.sh
```

This builds both `dilctl` (CLI) and `dil` (GUI), installs them to `~/.local/bin/` (or `/usr/local/bin/` with sudo), and sets up `dil` as a systemd user service that auto-starts on login.

Run `sudo ./scripts/install.sh` for a system-wide install instead.

Flags:
- `--no-gui` — CLI only, skip building/installing `dil`
- `--no-autostart` — don't set up the systemd autostart service
- `--uninstall` — remove binaries and service

If the GUI build fails due to missing dependencies, run `sudo bash scripts/install-gui-deps.sh` first, then re-run `install.sh`.

### 2. Check device status

```sh
dilctl status
```

This detects the CONNECT 2 and shows its current state:

```
Lewitt CONNECT 2
─────────────────────────────────────────────
  Card:        C2 (card 2)
  USB:         29c2:0004  serial C244ZML00076
  Product:     CONNECT 2 (by Lewitt GmbH)

  Capture:     4ch S32_LE 24-bit @ [44100, 48000, 96000] kHz
               channel map: FL FR FC LFE
  Playback:    2ch S32_LE 24-bit @ [44100, 48000, 96000] kHz
               channel map: FL FR

  ALSA config: lewitt_connect_2 PCM  — NOT installed
  WirePlumber: ignore rule  — NOT configured
```

### 3. Run setup

```sh
dilctl setup --user
```

This installs:
- An ALSA config at `~/.config/alsa/asoundrc` defining the `lewitt_connect_2` PCM
- A WirePlumber rule at `~/.config/wireplumber/main.lua.d/51-lewitt-ignore.lua` that prevents WirePlumber from grabbing the device

Use `sudo ./dilctl setup` (without `--user`) to install system-wide at `/etc/alsa/conf.d/` and `/etc/wireplumber/` instead.

### 4. Verify it works

```sh
dilctl verify
```

Records a 2-second clip from the `lewitt_connect_2` PCM, analyzes signal levels per channel, and plays it back through the headphone output:

```
Recording 2 second(s) from lewitt_connect_2...
(Make some sound into the microphone!)

Verification results:
─────────────────────────────────────────────
  Capture:     PASS
  Channel FL:  -42.3 dB
  Channel FR:  -38.1 dB

  Playback:    PASS
  (You should have heard the recording through headphones.)
```

### 5. Use in Audacity

In Audacity's device toolbar, select **`lewitt_connect_2`** as the recording device. It will record in stereo (2 channels) from the correct XLR inputs.

### GUI

The GUI (`dil`) runs as a system tray application. If you used `install.sh` with autostart enabled, it's already running — check your system tray. Otherwise, launch it manually:

```sh
dil
```

Click **"Open GUI"** in the tray menu to open the web interface, which provides:

- **Status tab** — device info and configuration state with Setup/Teardown buttons
- **Verify tab** — live level meters and a record/playback test with a mono playback toggle
- **Diagnostics tab** — full system diagnostic dump

## CLI Reference

```
Usage:
  dilctl [command]

Available Commands:
  status      Show current device and configuration status
  setup       Install ALSA config and WirePlumber ignore rule
  verify      Record and playback test to confirm the device works
  diagnose    Dump full diagnostic information
  teardown    Remove dilctl config and restore WirePlumber management

Flags:
  -h, --help   help for dilctl

Use "dilctl [command] --help" for more information about a command.
```

### setup flags

| Flag | Description |
|------|-------------|
| `--user` | Install to user config (`~/.config/`) instead of system-wide (`/etc/`) |
| `--dry-run` | Show what would be done without writing anything |

### verify flags

| Flag | Description |
|------|-------------|
| `-d, --duration` | Recording duration in seconds (default: 2) |
| `--no-playback` | Skip the playback test |
| `--mono` | Mix playback to mono on both channels (for single-mic use) |

## Teardown

To undo the setup and let WirePlumber manage the device again:

```sh
dilctl teardown
```

## Factory Reset

If the audio stack gets into a bad state, a factory reset script is provided to restore the Linux Mint default PipeWire/WirePlumber configuration:

```sh
sudo bash scripts/restore-factory-audio.sh
```

This unmask, re-enables, and reinstalls all PipeWire/WirePlumber packages and removes any custom ALSA or WirePlumber configuration. Reboot after running it.

## For Developers

### Project structure

```
do-it-lewitt/
├── cmd/
│   ├── dilctl/main.go          # CLI entrypoint (cobra)
│   └── dil/
│       ├── main.go             # Systray + embedded HTTP server
│       ├── web/index.html      # Web UI (embedded via go:embed)
│       ├── icon.png            # Tray icon (connected)
│       └── icon_off.png        # Tray icon (disconnected)
├── internal/lewitt/
│   ├── types.go                # Constants (VID/PID, card ID, templates)
│   ├── detect.go               # Device detection via sysfs + /proc/asound
│   ├── config.go               # ALSA conf + WirePlumber rule generation
│   ├── verify.go               # arecord/aplay subprocess + WAV RMS analysis
│   ├── diagnose.go             # Full diagnostic dump
│   └── audio.go                # Subprocess-based level meter capture
├── scripts/
│   ├── install.sh             # Build, install binaries, set up autostart
│   ├── install-gui-deps.sh    # Install GTK3 dev deps for building dil
│   └── restore-factory-audio.sh # Factory reset for audio stack
├── go.mod
├── go.sum
└── .gitignore
```

### Building

```sh
# CLI (no CGo required, fully static binary)
CGO_ENABLED=0 go build -o dilctl ./cmd/dilctl

# GUI (requires CGo + GTK3 dev packages)
sudo bash scripts/install-gui-deps.sh
CGO_ENABLED=1 go build -o dil ./cmd/dil
```

### How device detection works

The utility scans `/proc/asound/cards` for a USB audio card, then resolves the sysfs symlink (`/sys/class/sound/cardN/device`) to find the USB parent directory. It reads `idVendor` and `idProduct` from sysfs to match the Lewitt CONNECT 2 (VID `29c2`, PID `0004`).

All ALSA configs reference the device by its stable card ID (`C2`), not by card index (which can reshuffle across reboots). The WirePlumber rule matches by USB VID/PID, so it works regardless of card ordering.

### How the ALSA PCM works

The `lewitt_connect_2` PCM uses ALSA's `asym` plugin to handle capture and playback separately:

- **Capture**: `plug` → `route` (4ch hw → 2ch, extracting FL and FR via ttable)
- **Playback**: `plug` → `hw` (native 2ch, headphone out)

## License

[MIT](LICENSE)
