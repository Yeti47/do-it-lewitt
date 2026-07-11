package lewitt

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

func Verify(durationSec int, skipPlayback bool, monoPlayback bool) (*VerificationResult, error) {
	result := &VerificationResult{}

	tmpDir := os.TempDir()
	wavFile := filepath.Join(tmpDir, fmt.Sprintf("dilctl-verify-%d.wav", time.Now().Unix()))
	result.WavFile = wavFile

	arecordCmd := exec.Command("arecord",
		"-D", alsamPCMName,
		"-f", "S32_LE",
		"-r", "48000",
		"-c", "2",
		"-d", fmt.Sprintf("%d", durationSec),
		"-t", "wav",
		wavFile,
	)
	arecordCmd.Stderr = os.Stderr
	if err := arecordCmd.Run(); err != nil {
		result.CaptureError = err.Error()
		return result, nil
	}

	info, err := os.Stat(wavFile)
	if err != nil || info.Size() < 1000 {
		result.CaptureError = "recorded file is missing or too small"
		return result, nil
	}

	result.CaptureOK = true

	rmsDB, err := analyzeWAVRMS(wavFile)
	if err != nil {
		result.CaptureError = fmt.Sprintf("capture succeeded but WAV analysis failed: %v", err)
	} else {
		result.CaptureRMSDB = rmsDB
	}

	if skipPlayback {
		return result, nil
	}

	playbackFile := wavFile
	if monoPlayback {
		monoFile := filepath.Join(tmpDir, fmt.Sprintf("dilctl-verify-mono-%d.wav", time.Now().Unix()))
		if err := convertToMonoStereo(wavFile, monoFile); err != nil {
			result.PlaybackError = fmt.Sprintf("mono conversion failed: %v", err)
			return result, nil
		}
		playbackFile = monoFile
	}

	aplayCmd := exec.Command("aplay",
		"-D", alsamPCMName,
		playbackFile,
	)
	aplayCmd.Stderr = os.Stderr
	if err := aplayCmd.Run(); err != nil {
		result.PlaybackError = err.Error()
	} else {
		result.PlaybackOK = true
	}

	return result, nil
}

func convertToMonoStereo(input, output string) error {
	data, err := os.ReadFile(input)
	if err != nil {
		return err
	}

	header, pcmData, err := parseWAVHeader(data)
	if err != nil {
		return err
	}

	bytesPerSample := int(header.BitsPerSample) / 8
	channels := int(header.NumChannels)
	if channels < 1 {
		channels = 1
	}
	frameSize := bytesPerSample * channels
	numFrames := len(pcmData) / frameSize

	monoData := make([]byte, numFrames*bytesPerSample)
	for i := 0; i < numFrames; i++ {
		var sum int64
		for ch := 0; ch < channels; ch++ {
			offset := i*frameSize + ch*bytesPerSample
			if offset+bytesPerSample > len(pcmData) {
				break
			}
			var sample int64
			switch bytesPerSample {
			case 4:
				sample = int64(int32(binary.LittleEndian.Uint32(pcmData[offset:])))
			case 2:
				sample = int64(int16(binary.LittleEndian.Uint16(pcmData[offset:])))
			case 1:
				sample = int64(int8(pcmData[offset]))
			}
			sum += sample
		}
		avg := sum / int64(channels)

		dstOffset := i * bytesPerSample
		switch bytesPerSample {
		case 4:
			binary.LittleEndian.PutUint32(monoData[dstOffset:], uint32(int32(avg)))
		case 2:
			binary.LittleEndian.PutUint16(monoData[dstOffset:], uint16(int16(avg)))
		case 1:
			monoData[dstOffset] = byte(int8(avg))
		}
	}

	stereoData := make([]byte, numFrames*bytesPerSample*2)
	for i := 0; i < numFrames; i++ {
		srcOffset := i * bytesPerSample
		dstOffset := i * bytesPerSample * 2
		copy(stereoData[dstOffset:dstOffset+bytesPerSample], monoData[srcOffset:srcOffset+bytesPerSample])
		copy(stereoData[dstOffset+bytesPerSample:dstOffset+bytesPerSample*2], monoData[srcOffset:srcOffset+bytesPerSample])
	}

	return writeWAV(output, stereoData, 2, header.SampleRate, header.BitsPerSample)
}

func writeWAV(path string, pcmData []byte, channels uint16, sampleRate uint32, bitsPerSample uint16) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	byteRate := sampleRate * uint32(channels) * uint32(bitsPerSample) / 8
	blockAlign := channels * bitsPerSample / 8
	dataSize := uint32(len(pcmData))

	header := []byte{
		'R', 'I', 'F', 'F',
		byte(36 + dataSize), byte((36 + dataSize) >> 8), byte((36 + dataSize) >> 16), byte((36 + dataSize) >> 24),
		'W', 'A', 'V', 'E',
		'f', 'm', 't', ' ',
		16, 0, 0, 0,
		1, 0,
		byte(channels), byte(channels >> 8),
		byte(sampleRate), byte(sampleRate >> 8), byte(sampleRate >> 16), byte(sampleRate >> 24),
		byte(byteRate), byte(byteRate >> 8), byte(byteRate >> 16), byte(byteRate >> 24),
		byte(blockAlign), byte(blockAlign >> 8),
		byte(bitsPerSample), byte(bitsPerSample >> 8),
		'd', 'a', 't', 'a',
		byte(dataSize), byte(dataSize >> 8), byte(dataSize >> 16), byte(dataSize >> 24),
	}

	if _, err := file.Write(header); err != nil {
		return err
	}
	_, err = file.Write(pcmData)
	return err
}

type wavHeader struct {
	AudioFormat   uint16
	NumChannels   uint16
	SampleRate    uint32
	ByteRate      uint32
	BlockAlign    uint16
	BitsPerSample uint16
}

func parseWAVHeader(data []byte) (*wavHeader, []byte, error) {
	if len(data) < 44 {
		return nil, nil, fmt.Errorf("file too small for WAV header")
	}
	if string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return nil, nil, fmt.Errorf("not a valid RIFF/WAVE file")
	}

	offset := 12
	var header wavHeader
	var pcmData []byte

	for offset+8 <= len(data) {
		chunkID := string(data[offset : offset+4])
		chunkSize := binary.LittleEndian.Uint32(data[offset+4 : offset+8])
		chunkDataStart := offset + 8
		chunkDataEnd := chunkDataStart + int(chunkSize)

		if chunkDataEnd > len(data) {
			chunkDataEnd = len(data)
		}

		switch chunkID {
		case "fmt ":
			if chunkSize < 16 {
				return nil, nil, fmt.Errorf("fmt chunk too small")
			}
			header.AudioFormat = binary.LittleEndian.Uint16(data[chunkDataStart:])
			header.NumChannels = binary.LittleEndian.Uint16(data[chunkDataStart+2:])
			header.SampleRate = binary.LittleEndian.Uint32(data[chunkDataStart+4:])
			header.ByteRate = binary.LittleEndian.Uint32(data[chunkDataStart+8:])
			header.BlockAlign = binary.LittleEndian.Uint16(data[chunkDataStart+12:])
			header.BitsPerSample = binary.LittleEndian.Uint16(data[chunkDataStart+14:])
		case "data":
			pcmData = data[chunkDataStart:chunkDataEnd]
		}

		offset = chunkDataEnd
		if chunkSize%2 != 0 {
			offset++
		}
	}

	if pcmData == nil {
		return nil, nil, fmt.Errorf("no data chunk found in WAV")
	}

	return &header, pcmData, nil
}

func analyzeWAVRMS(path string) ([2]float64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return [2]float64{}, err
	}

	header, pcmData, err := parseWAVHeader(data)
	if err != nil {
		return [2]float64{}, err
	}

	if header.AudioFormat != 1 {
		return [2]float64{}, fmt.Errorf("unsupported audio format: %d (expected PCM=1)", header.AudioFormat)
	}

	bytesPerSample := int(header.BitsPerSample) / 8
	channels := int(header.NumChannels)
	if channels < 1 {
		channels = 1
	}
	if channels > 2 {
		channels = 2
	}

	frameSize := bytesPerSample * channels
	numFrames := len(pcmData) / frameSize

	var sums [2]float64
	var counts [2]int

	for i := 0; i < numFrames; i++ {
		for ch := 0; ch < channels; ch++ {
			offset := i*frameSize + ch*bytesPerSample
			var sample float64
			switch bytesPerSample {
			case 4:
				val := int32(binary.LittleEndian.Uint32(pcmData[offset:]))
				sample = float64(val)
			case 2:
				val := int16(binary.LittleEndian.Uint16(pcmData[offset:]))
				sample = float64(val)
			case 1:
				sample = float64(int8(pcmData[offset])) * 256 * 256 * 256
			default:
				continue
			}
			sums[ch] += sample * sample
			counts[ch]++
		}
	}

	var rmsDB [2]float64
	for ch := 0; ch < channels; ch++ {
		if counts[ch] == 0 {
			rmsDB[ch] = math.Inf(-1)
			continue
		}
		rms := math.Sqrt(sums[ch] / float64(counts[ch]))
		if rms <= 0 {
			rmsDB[ch] = math.Inf(-1)
		} else {
			rmsDB[ch] = 20 * math.Log10(rms/2147483648.0)
		}
	}

	return rmsDB, nil
}
