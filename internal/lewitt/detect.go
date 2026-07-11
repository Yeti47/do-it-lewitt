package lewitt

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func Detect() (*DeviceInfo, error) {
	data, err := os.ReadFile("/proc/asound/cards")
	if err != nil {
		return nil, fmt.Errorf("cannot read /proc/asound/cards: %w", err)
	}

	lines := strings.Split(string(data), "\n")
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}

		bracketStart := strings.Index(line, "[")
		bracketEnd := strings.Index(line, "]")
		if bracketStart < 0 || bracketEnd < 0 || bracketEnd < bracketStart {
			continue
		}

		idxStr := strings.TrimSpace(line[:bracketStart])
		idx, err := strconv.Atoi(idxStr)
		if err != nil {
			continue
		}

		id := strings.TrimSpace(line[bracketStart+1 : bracketEnd])

		if !strings.Contains(line, "USB-Audio") {
			continue
		}

		dev := &DeviceInfo{CardIndex: idx, CardID: id}

		sysfsBase := fmt.Sprintf("/sys/class/sound/card%d", idx)
		usbParent, err := resolveUSBParent(sysfsBase)
		if err != nil {
			continue
		}

		vendor, _ := os.ReadFile(filepath.Join(usbParent, "idVendor"))
		vID := strings.TrimSpace(string(vendor))
		if vID != VendorID {
			continue
		}

		product, _ := os.ReadFile(filepath.Join(usbParent, "idProduct"))
		if strings.TrimSpace(string(product)) != ProductID {
			continue
		}

		dev.VendorID = vID
		dev.ProductID = ProductID

		serial, _ := os.ReadFile(filepath.Join(usbParent, "serial"))
		dev.Serial = strings.TrimSpace(string(serial))

		manufacturer, _ := os.ReadFile(filepath.Join(usbParent, "manufacturer"))
		dev.Manufacturer = strings.TrimSpace(string(manufacturer))

		prodName, _ := os.ReadFile(filepath.Join(usbParent, "product"))
		dev.Product = strings.TrimSpace(string(prodName))

		dev.CardName = strings.TrimSpace(line)
		dev.Connected = true

		return dev, nil
	}

	return &DeviceInfo{Connected: false}, nil
}

func resolveUSBParent(sysfsBase string) (string, error) {
	devicePath := filepath.Join(sysfsBase, "device")
	realPath, err := filepath.EvalSymlinks(devicePath)
	if err != nil {
		return "", err
	}
	return filepath.Dir(realPath), nil
}

func ReadStreamInfo(cardID string) (*CaptureStreamInfo, *PlaybackStreamInfo, error) {
	path := fmt.Sprintf("/proc/asound/%s/stream0", cardID)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot read stream info: %w", err)
	}

	content := string(data)
	capture, playback := parseStream(content)
	return capture, playback, nil
}

func parseStream(content string) (*CaptureStreamInfo, *PlaybackStreamInfo) {
	var capture *CaptureStreamInfo
	var playback *PlaybackStreamInfo

	lines := strings.Split(content, "\n")
	currentSection := ""
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		lower := strings.ToLower(line)

		if strings.HasPrefix(lower, "capture:") {
			currentSection = "capture"
			capture = &CaptureStreamInfo{Stream: StreamInfo{}}
			continue
		} else if strings.HasPrefix(lower, "playback:") {
			currentSection = "playback"
			playback = &PlaybackStreamInfo{Stream: StreamInfo{}}
			continue
		}

		switch currentSection {
		case "capture":
			fillStreamInfo(&capture.Stream, line)
		case "playback":
			fillStreamInfo(&playback.Stream, line)
		}
	}

	return capture, playback
}

func fillStreamInfo(s *StreamInfo, line string) {
	trimmed := strings.TrimSpace(line)
	parts := strings.SplitN(trimmed, ":", 2)
	if len(parts) != 2 {
		return
	}
	key := strings.TrimSpace(parts[0])
	value := strings.TrimSpace(parts[1])

	switch key {
	case "Format":
		s.Format = value
	case "Channels":
		if ch, err := strconv.Atoi(value); err == nil {
			s.Channels = ch
		}
	case "Rates":
		for _, r := range strings.Split(value, ",") {
			r = strings.TrimSpace(r)
			if r != "" {
				s.Rates = append(s.Rates, r)
			}
		}
	case "Bits":
		if b, err := strconv.Atoi(value); err == nil {
			s.Bits = b
		}
	case "Channel map":
		s.ChannelMap = value
	case "Status":
		s.StreamState = strings.TrimSpace(value)
	}
}

func CheckConfig() *ConfigStatus {
	status := &ConfigStatus{}

	paths := []string{
		alsamSystemConfPath,
		expandUserPath("~/.asoundrc"),
		expandUserPath("~/.config/alsa/asoundrc"),
	}
	for _, p := range paths {
		if fileExists(p) {
			content, err := os.ReadFile(p)
			if err == nil && strings.Contains(string(content), alsamPCMName) {
				status.ALSAMInstalled = true
				status.ALSAMPath = p
				break
			}
		}
	}

	wpPaths := []string{
		wpSystemConfDir + "/" + wpRuleFileBasename,
		expandUserPath("~/.config/wireplumber/main.lua.d/" + wpRuleFileBasename),
	}
	for _, p := range wpPaths {
		if fileExists(p) {
			status.WPIgnoreInstalled = true
			status.WPIgnorePath = p
			break
		}
	}

	return status
}

func expandUserPath(path string) string {
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[1:])
	}
	return path
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
