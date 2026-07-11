package lewitt

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os/exec"
	"sync"
)

type LevelMeter struct {
	mu     sync.Mutex
	levels [2]float64
	active bool
	stopCh chan struct{}
	cmd    *exec.Cmd
}

func NewLevelMeter() *LevelMeter {
	return &LevelMeter{}
}

func (lm *LevelMeter) Levels() [2]float64 {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	return lm.levels
}

func (lm *LevelMeter) Active() bool {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	return lm.active
}

func (lm *LevelMeter) Start() error {
	cfg := CheckConfig()
	if !cfg.ALSAMInstalled {
		return fmt.Errorf("ALSA config not installed. Run 'dilctl setup' or use the Setup button in the GUI first")
	}

	lm.mu.Lock()
	if lm.active {
		lm.mu.Unlock()
		return fmt.Errorf("level meter already running")
	}
	lm.active = true
	lm.stopCh = make(chan struct{})
	lm.mu.Unlock()

	go lm.run()
	return nil
}

func (lm *LevelMeter) Stop() {
	lm.mu.Lock()
	if !lm.active {
		lm.mu.Unlock()
		return
	}
	lm.active = false
	close(lm.stopCh)
	if lm.cmd != nil && lm.cmd.Process != nil {
		lm.cmd.Process.Kill()
	}
	lm.mu.Unlock()
}

func (lm *LevelMeter) run() {
	defer func() {
		lm.mu.Lock()
		lm.active = false
		lm.mu.Unlock()
	}()

	cmd := exec.Command("arecord",
		"-D", alsamPCMName,
		"-f", "S32_LE",
		"-r", "48000",
		"-c", "2",
		"-t", "raw",
		"--buffer-size=4096",
		"-",
	)
	cmd.Stderr = nil

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return
	}

	if err := cmd.Start(); err != nil {
		return
	}

	lm.mu.Lock()
	lm.cmd = cmd
	lm.mu.Unlock()

	defer cmd.Process.Kill()

	buf := make([]byte, 4096*8)
	for {
		select {
		case <-lm.stopCh:
			return
		default:
		}

		n, err := stdout.Read(buf)
		if err != nil {
			if err == io.EOF {
				return
			}
			continue
		}
		if n > 0 {
			lm.computeLevels(buf[:n], 2)
		}
	}
}

func (lm *LevelMeter) computeLevels(samples []byte, channels int) {
	if channels < 1 {
		channels = 1
	}
	if channels > 2 {
		channels = 2
	}

	bytesPerSample := 4
	frameSize := bytesPerSample * channels
	numFrames := len(samples) / frameSize
	if numFrames == 0 {
		return
	}

	const windowSize = 480
	if numFrames > windowSize {
		numFrames = windowSize
	}

	var sums [2]float64
	var counts [2]int

	for i := 0; i < numFrames; i++ {
		for ch := 0; ch < channels; ch++ {
			offset := i*frameSize + ch*bytesPerSample
			if offset+4 > len(samples) {
				break
			}
			val := int32(binary.LittleEndian.Uint32(samples[offset : offset+4]))
			normalized := float64(val) / 2147483648.0
			sums[ch] += normalized * normalized
			counts[ch]++
		}
	}

	var levels [2]float64
	for ch := 0; ch < channels; ch++ {
		if counts[ch] == 0 {
			levels[ch] = -120
			continue
		}
		rms := math.Sqrt(sums[ch] / float64(counts[ch]))
		if rms <= 0.000001 {
			levels[ch] = -120
		} else {
			levels[ch] = 20 * math.Log10(rms)
		}
	}

	lm.mu.Lock()
	lm.levels = levels
	lm.mu.Unlock()
}
