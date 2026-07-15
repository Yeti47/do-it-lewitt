package lewitt

import (
	"strings"
	"testing"
)

func TestALSAConfigUsesDirectHardwareStreams(t *testing.T) {
	config := alsamSystemConfTemplate
	if strings.Contains(config, "dmix") || strings.Contains(config, "dsnoop") {
		t.Fatal("ALSA template must not add shared dmix/dsnoop hardware streams")
	}

	if !strings.Contains(config, "slave.format S32_LE") || !strings.Contains(config, "slave.rate 48000") {
		t.Fatal("ALSA template must keep the hardware stream at native format and rate")
	}
}
