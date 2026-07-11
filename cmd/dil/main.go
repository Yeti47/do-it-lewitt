package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"do-it-lewitt/internal/lewitt"

	"github.com/getlantern/systray"
)

//go:embed web/index.html
var webFS embed.FS

//go:embed icon.png
var iconConnected []byte

//go:embed icon_off.png
var iconDisconnected []byte

var (
	levelMeter  *lewitt.LevelMeter
	server      *http.Server
	serverPort  int
	lastWavFile string
)

func main() {
	levelMeter = lewitt.NewLevelMeter()

	systray.Run(onReady, onExit)
}

func onReady() {
	systray.SetIcon(iconDisconnected)
	systray.SetTitle("Do it, Lewitt!")
	systray.SetTooltip("Do it, Lewitt!")

	mStatus := systray.AddMenuItem("Status: checking...", "")
	mStatus.Disable()
	systray.AddSeparator()

	mOpen := systray.AddMenuItem("Open GUI", "")
	mSetup := systray.AddMenuItem("Setup", "")
	mVerify := systray.AddMenuItem("Quick Verify", "")
	mDiagnose := systray.AddMenuItem("Diagnostics", "")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit", "")

	go func() {
		for {
			select {
			case <-mOpen.ClickedCh:
				openBrowser()
			case <-mSetup.ClickedCh:
				execSetup()
				updateTrayIcon()
				updateTrayStatus(mStatus)
			case <-mVerify.ClickedCh:
				execVerify()
			case <-mDiagnose.ClickedCh:
				output, _ := lewitt.Diagnose()
				notify("Do it, Lewitt!: Diagnostics", "Copied to clipboard")
				copyToClipboard(output)
			case <-mQuit.ClickedCh:
				systray.Quit()
				return
			}
		}
	}()

	go startHTTPServer()
	go periodicTrayUpdate(mStatus)
}

func onExit() {
	if levelMeter != nil {
		levelMeter.Stop()
	}
	if server != nil {
		server.Close()
	}
}

func updateTrayIcon() {
	dev, err := lewitt.Detect()
	if err != nil || !dev.Connected {
		systray.SetIcon(iconDisconnected)
		systray.SetTooltip("Do it, Lewitt! — not connected")
		return
	}

	cfg := lewitt.CheckConfig()
	if cfg.ALSAMInstalled && cfg.WPIgnoreInstalled {
		systray.SetIcon(iconConnected)
		systray.SetTooltip("Do it, Lewitt! — connected, configured")
	} else {
		systray.SetIcon(iconDisconnected)
		systray.SetTooltip("Do it, Lewitt! — connected, NOT configured (click Setup)")
	}
}

func updateTrayStatus(m *systray.MenuItem) {
	dev, err := lewitt.Detect()
	if err != nil || !dev.Connected {
		m.SetTitle("Status: NOT CONNECTED")
		return
	}
	cfg := lewitt.CheckConfig()
	if cfg.ALSAMInstalled && cfg.WPIgnoreInstalled {
		m.SetTitle("Status: connected, configured ✓")
	} else {
		m.SetTitle("Status: connected, NOT configured")
	}
}

func periodicTrayUpdate(m *systray.MenuItem) {
	updateTrayIcon()
	updateTrayStatus(m)
	for {
		time.Sleep(10 * time.Second)
		updateTrayIcon()
		updateTrayStatus(m)
	}
}

func startHTTPServer() {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		data, _ := webFS.ReadFile("web/index.html")
		w.Header().Set("Content-Type", "text/html")
		w.Write(data)
	})

	mux.HandleFunc("/api/status", handleStatus)
	mux.HandleFunc("/api/setup", handleSetup)
	mux.HandleFunc("/api/teardown", handleTeardown)
	mux.HandleFunc("/api/verify", handleVerify)
	mux.HandleFunc("/api/playback", handlePlayback)
	mux.HandleFunc("/api/diagnose", handleDiagnose)
	mux.HandleFunc("/api/levels", handleLevels)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Printf("failed to start HTTP server: %v", err)
		return
	}
	serverPort = listener.Addr().(*net.TCPAddr).Port
	server = &http.Server{Handler: mux}
	log.Printf("GUI running on http://127.0.0.1:%d", serverPort)
	server.Serve(listener)
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	dev, err := lewitt.Detect()
	if err != nil {
		jsonError(w, err)
		return
	}

	if !dev.Connected {
		jsonOK(w, map[string]interface{}{"connected": false})
		return
	}

	capture, playback, _ := lewitt.ReadStreamInfo(dev.CardID)
	cfg := lewitt.CheckConfig()

	resp := map[string]interface{}{
		"connected":      true,
		"card_index":     dev.CardIndex,
		"card_id":        dev.CardID,
		"vendor_id":      dev.VendorID,
		"product_id":     dev.ProductID,
		"serial":         dev.Serial,
		"product":        dev.Product,
		"alsa_installed": cfg.ALSAMInstalled,
		"wp_configured":  cfg.WPIgnoreInstalled,
	}
	if capture != nil {
		resp["capture_channels"] = capture.Stream.Channels
		resp["capture_format"] = capture.Stream.Format
		resp["capture_bits"] = capture.Stream.Bits
		resp["capture_rates"] = strings.Join(capture.Stream.Rates, ", ")
		resp["capture_channel_map"] = capture.Stream.ChannelMap
	}
	if playback != nil {
		resp["playback_channels"] = playback.Stream.Channels
		resp["playback_format"] = playback.Stream.Format
		resp["playback_bits"] = playback.Stream.Bits
		resp["playback_rates"] = strings.Join(playback.Stream.Rates, ", ")
	}

	jsonOK(w, resp)
}

func handleSetup(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, fmt.Errorf("POST required"))
		return
	}
	target := lewitt.InstallUser
	if os.Geteuid() == 0 {
		target = lewitt.InstallSystem
	}
	if err := lewitt.InstallConfig(target, false); err != nil {
		jsonError(w, err)
		return
	}
	jsonOK(w, map[string]interface{}{"ok": true, "message": "ALSA config and WirePlumber rule installed."})
}

func handleTeardown(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, fmt.Errorf("POST required"))
		return
	}
	if levelMeter != nil {
		levelMeter.Stop()
	}
	if err := lewitt.TeardownConfig(false); err != nil {
		jsonError(w, err)
		return
	}
	jsonOK(w, map[string]interface{}{"ok": true, "message": "Configuration removed. WirePlumber re-managing devices."})
}

func handleVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, fmt.Errorf("POST required"))
		return
	}

	cfg := lewitt.CheckConfig()
	if !cfg.ALSAMInstalled {
		jsonError(w, fmt.Errorf("ALSA config not installed. Click Setup first"))
		return
	}

	var body struct {
		Mono bool `json:"mono"`
	}
	json.NewDecoder(r.Body).Decode(&body)

	result, err := lewitt.Verify(2, false, body.Mono)
	if err != nil {
		jsonError(w, err)
		return
	}
	if result.WavFile != "" {
		lastWavFile = result.WavFile
	}
	fl := result.CaptureRMSDB[0]
	fr := result.CaptureRMSDB[1]
	if math.IsInf(fl, -1) || fl <= -120 {
		fl = -120
	}
	if math.IsInf(fr, -1) || fr <= -120 {
		fr = -120
	}
	resp := map[string]interface{}{
		"capture_ok":     result.CaptureOK,
		"capture_rms_db": [2]float64{fl, fr},
		"capture_error":  result.CaptureError,
		"playback_ok":    result.PlaybackOK,
		"playback_error": result.PlaybackError,
	}
	jsonOK(w, resp)
}

func handlePlayback(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, fmt.Errorf("POST required"))
		return
	}
	if lastWavFile == "" {
		jsonError(w, fmt.Errorf("no recording available. Run a verify test first"))
		return
	}
	cmd := exec.Command("aplay", "-D", "lewitt_connect_2", lastWavFile)
	if err := cmd.Run(); err != nil {
		jsonError(w, fmt.Errorf("playback failed: %w", err))
		return
	}
	jsonOK(w, map[string]interface{}{"ok": true, "file": lastWavFile})
}

func handleDiagnose(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, fmt.Errorf("POST required"))
		return
	}
	output, err := lewitt.Diagnose()
	if err != nil {
		jsonError(w, err)
		return
	}
	jsonOK(w, map[string]interface{}{"output": output})
}

func handleLevels(w http.ResponseWriter, r *http.Request) {
	cfg := lewitt.CheckConfig()
	if !cfg.ALSAMInstalled {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"error\":\"not configured\"}\n\n")
		w.(http.Flusher).Flush()
		return
	}

	if err := levelMeter.Start(); err != nil {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"error\":\"%s\"}\n\n", err.Error())
		w.(http.Flusher).Flush()
		return
	}
	defer levelMeter.Stop()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			levels := levelMeter.Levels()
			fl := levels[0]
			fr := levels[1]
			if math.IsInf(fl, -1) || fl <= -120 {
				fl = -120
			}
			if math.IsInf(fr, -1) || fr <= -120 {
				fr = -120
			}
			data, _ := json.Marshal(map[string]float64{"fl": fl, "fr": fr})
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

func execSetup() {
	notify("Do it, Lewitt!: Setup", "Installing ALSA config + WirePlumber rule...")
	target := lewitt.InstallUser
	if os.Geteuid() == 0 {
		target = lewitt.InstallSystem
	}
	if err := lewitt.InstallConfig(target, false); err != nil {
		notify("Do it, Lewitt!: Setup failed", err.Error())
	} else {
		notify("Do it, Lewitt!: Setup complete", "Lewitt CONNECT 2 configured for direct ALSA access.")
	}
}

func execVerify() {
	cfg := lewitt.CheckConfig()
	if !cfg.ALSAMInstalled {
		notify("Do it, Lewitt!: Cannot verify", "ALSA config not installed. Click Setup first.")
		return
	}
	notify("Do it, Lewitt!: Verify", "Recording 2 seconds... (make some sound!)")
	result, err := lewitt.Verify(2, false, false)
	if err != nil {
		notify("Do it, Lewitt!: Verify failed", err.Error())
		return
	}
	if result.CaptureOK && result.PlaybackOK {
		notify("Do it, Lewitt!: Verify passed", fmt.Sprintf("Capture ✓ Playback ✓ (FL: %.1fdB, FR: %.1fdB)", result.CaptureRMSDB[0], result.CaptureRMSDB[1]))
	} else {
		notify("Do it, Lewitt!: Verify", "Some checks failed — see GUI for details")
	}
}

func openBrowser() {
	url := fmt.Sprintf("http://127.0.0.1:%d", serverPort)
	exec.Command("xdg-open", url).Start()
}

func notify(title, body string) {
	exec.Command("notify-send", title, body).Start()
}

func copyToClipboard(text string) {
	cmd := exec.Command("xclip", "-selection", "clipboard")
	cmd.Stdin = strings.NewReader(text)
	cmd.Start()
}

func jsonOK(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func jsonError(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "error": err.Error()})
}
