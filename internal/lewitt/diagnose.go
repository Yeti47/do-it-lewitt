package lewitt

import (
	"fmt"
	"os/exec"
	"strings"
)

func Diagnose() (string, error) {
	var sb strings.Builder

	sb.WriteString("=== dilctl diagnostic dump ===\n\n")

	sb.WriteString("--- /proc/asound/cards ---\n")
	if out, err := runCommand("cat", "/proc/asound/cards"); err == nil {
		sb.WriteString(out)
	} else {
		sb.WriteString(fmt.Sprintf("error: %v\n", err))
	}
	sb.WriteString("\n")

	dev, err := Detect()
	if err != nil {
		sb.WriteString(fmt.Sprintf("Device detection error: %v\n\n", err))
		return sb.String(), nil
	}

	if !dev.Connected {
		sb.WriteString("Lewitt CONNECT 2 not detected.\n")
		return sb.String(), nil
	}

	sb.WriteString(fmt.Sprintf("--- Device: card %d, id %s ---\n", dev.CardIndex, dev.CardID))
	sb.WriteString(fmt.Sprintf("USB: %s:%s  serial: %s\n", dev.VendorID, dev.ProductID, dev.Serial))
	sb.WriteString(fmt.Sprintf("Product: %s (by %s)\n\n", dev.Product, dev.Manufacturer))

	sb.WriteString("--- /proc/asound/" + dev.CardID + "/stream0 ---\n")
	if out, err := runCommand("cat", fmt.Sprintf("/proc/asound/%s/stream0", dev.CardID)); err == nil {
		sb.WriteString(out)
	} else {
		sb.WriteString(fmt.Sprintf("error: %v\n", err))
	}
	sb.WriteString("\n")

	sb.WriteString(fmt.Sprintf("--- /proc/asound/%s/pcm0c/sub0/hw_params ---\n", dev.CardID))
	if out, err := runCommand("cat", fmt.Sprintf("/proc/asound/%s/pcm0c/sub0/hw_params", dev.CardID)); err == nil {
		sb.WriteString(out)
	} else {
		sb.WriteString(fmt.Sprintf("error: %v\n", err))
	}
	sb.WriteString("\n")

	sb.WriteString(fmt.Sprintf("--- amixer -c %d ---\n", dev.CardIndex))
	if out, err := runCommand("amixer", "-c", fmt.Sprintf("%d", dev.CardIndex)); err == nil {
		sb.WriteString(out)
	} else {
		sb.WriteString(fmt.Sprintf("error: %v\n", err))
	}
	sb.WriteString("\n")

	sb.WriteString("--- lsusb -v -d 29c2:0004 ---\n")
	if out, err := runCommand("lsusb", "-v", "-d", "29c2:0004"); err == nil {
		sb.WriteString(out)
	} else {
		sb.WriteString(fmt.Sprintf("error: %v\n", err))
	}
	sb.WriteString("\n")

	sb.WriteString("--- udevadm info ---\n")
	if out, err := runCommand("udevadm", "info", fmt.Sprintf("/sys/class/sound/card%d", dev.CardIndex)); err == nil {
		sb.WriteString(out)
	} else {
		sb.WriteString(fmt.Sprintf("error: %v\n", err))
	}
	sb.WriteString("\n")

	sb.WriteString("--- dmesg (snd-usb related) ---\n")
	dmesgCmd := exec.Command("dmesg")
	grepCmd := exec.Command("grep", "-i", "snd-usb")
	pipe, err := dmesgCmd.StdoutPipe()
	if err == nil {
		grepCmd.Stdin = pipe
		if err := dmesgCmd.Start(); err == nil {
			if out, err := grepCmd.CombinedOutput(); err == nil {
				sb.WriteString(string(out))
			}
			dmesgCmd.Wait()
		}
	}
	sb.WriteString("\n")

	sb.WriteString("--- Config status ---\n")
	status := CheckConfig()
	sb.WriteString(fmt.Sprintf("ALSA config installed: %v", status.ALSAMInstalled))
	if status.ALSAMInstalled {
		sb.WriteString(fmt.Sprintf(" (%s)", status.ALSAMPath))
	}
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("WirePlumber ignore rule: %v", status.WPIgnoreInstalled))
	if status.WPIgnoreInstalled {
		sb.WriteString(fmt.Sprintf(" (%s)", status.WPIgnorePath))
	}
	sb.WriteString("\n\n")

	sb.WriteString("--- WirePlumber status (if running) ---\n")
	if out, err := runCommand("wpctl", "status"); err == nil {
		sb.WriteString(out)
	} else {
		sb.WriteString("WirePlumber not running or wpctl unavailable\n")
	}

	return sb.String(), nil
}

func runCommand(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
