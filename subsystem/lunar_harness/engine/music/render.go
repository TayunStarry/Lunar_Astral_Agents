package music

// render.go 标准 MIDI 写入与 fluidsynth 渲染。
// 乐谱由 AMT 解码直接得到，本包写成 SMF format 1，再交给 fluidsynth 生成 WAV。

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
)

// Render 把解码后的乐谱写成 MIDI 并渲染 WAV。
func Render(score *parsedScore, source string) (RenderResult, error) {
	result := RenderResult{Source: source}
	name := ScoreFingerprint(score) + ".wav"
	outputDir := config.MusicOutputDir
	if outputDir == "" {
		outputDir = filepath.Join(config.LocalDir, "audios")
	}
	wavPath := filepath.Join(outputDir, name)
	result.AudioPath = wavPath
	result.AudioURL = "/file/read/audios/" + name
	result.DurationSeconds = scoreDurationSeconds(score)

	if info, statErr := os.Stat(wavPath); statErr == nil && info.Size() > 0 {
		return result, nil
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return result, fmt.Errorf("创建音频输出目录失败: %w", err)
	}
	midiPath, err := writeTempMIDI(score)
	if err != nil {
		return result, err
	}
	defer os.Remove(midiPath)
	if err := renderWithFluidsynth(midiPath, wavPath); err != nil {
		result.AudioURL = ""
		return result, err
	}
	return result, nil
}

// scoreDurationSeconds 估算演奏时长（按最后一个音符结束时间）。
func scoreDurationSeconds(score *parsedScore) float64 {
	endTick := 0
	for _, voice := range score.Voices {
		for _, note := range voice.Notes {
			if finish := note.Tick + note.Duration; finish > endTick {
				endTick = finish
			}
		}
	}
	if endTick == 0 || score.Tempo <= 0 {
		return 0
	}
	return float64(endTick) / float64(PPQ) * 60 / float64(score.Tempo)
}

// writeTempMIDI 把乐谱写成临时 .mid 文件，返回路径。
func writeTempMIDI(score *parsedScore) (string, error) {
	raw, err := encodeSMF(score)
	if err != nil {
		return "", err
	}
	file, err := os.CreateTemp("", "lunar-music-*.mid")
	if err != nil {
		return "", fmt.Errorf("创建临时 MIDI 文件失败: %w", err)
	}
	path := file.Name()
	if _, err := file.Write(raw); err != nil {
		file.Close()
		os.Remove(path)
		return "", fmt.Errorf("写入临时 MIDI 文件失败: %w", err)
	}
	if err := file.Close(); err != nil {
		os.Remove(path)
		return "", fmt.Errorf("关闭临时 MIDI 文件失败: %w", err)
	}
	return path, nil
}

// encodeSMF 生成 SMF format 1 字节流。
func encodeSMF(score *parsedScore) ([]byte, error) {
	var buffer bytes.Buffer
	// MThd：format 1、轨道数 1 + 声部数、PPQ 480
	buffer.WriteString("MThd")
	binary.Write(&buffer, binary.BigEndian, uint32(6))
	binary.Write(&buffer, binary.BigEndian, uint16(1))
	binary.Write(&buffer, binary.BigEndian, uint16(1+len(score.Voices)))
	binary.Write(&buffer, binary.BigEndian, uint16(PPQ))

	tempo := score.Tempo
	if tempo <= 0 {
		tempo = defaultTempo
	}
	conductor := make([]byte, 0, 64)
	conductor = append(conductor, metaTrackName(score.Title)...)
	micros := uint32(60000000 / tempo)
	conductor = append(conductor, 0x00, 0xFF, 0x51, 0x03, byte(micros>>16), byte(micros>>8), byte(micros))
	denPower := uint8(0)
	for value := score.MeterDen; value > 1; value /= 2 {
		denPower++
	}
	conductor = append(conductor, 0x00, 0xFF, 0x58, 0x04,
		byte(score.MeterNum), denPower, 0x18, 0x08)
	conductor = append(conductor, 0x00, 0xFF, 0x2F, 0x00)
	writeChunk(&buffer, "MTrk", conductor)

	for _, voice := range score.Voices {
		channel := voice.Channel
		if channel < 0 || channel > 15 {
			channel = 0
		}
		events := buildVoiceEvents(voice, byte(channel))
		writeChunk(&buffer, "MTrk", events)
	}
	return buffer.Bytes(), nil
}

// buildVoiceEvents 把单声部音符转成带 delta 的 MIDI 轨道事件。
func buildVoiceEvents(voice midiVoice, channel byte) []byte {
	type trackEvent struct {
		tick    int
		payload []byte
		order   int // 0=note-off 1=note-on，同一 tick 先关后开
	}
	events := make([]trackEvent, 0, len(voice.Notes)*2)
	for _, note := range voice.Notes {
		events = append(events,
			trackEvent{tick: note.Tick, order: 1, payload: []byte{0x90 | channel, byte(note.Note), byte(note.Velocity)}},
			trackEvent{tick: note.Tick + note.Duration, order: 0, payload: []byte{0x90 | channel, byte(note.Note), 0x00}},
		)
	}
	sort.Slice(events, func(i, j int) bool {
		if events[i].tick != events[j].tick {
			return events[i].tick < events[j].tick
		}
		return events[i].order < events[j].order
	})

	out := make([]byte, 0, len(events)*4+32)
	out = append(out, metaTrackName(voice.Name)...)
	program := voice.Program
	if program < 0 || program > 127 {
		program = 0
	}
	out = append(out, 0x00, 0xC0|channel, byte(program))
	lastTick := 0
	for _, event := range events {
		delta := event.tick - lastTick
		if delta < 0 {
			delta = 0
		}
		out = append(out, vlq(delta)...)
		out = append(out, event.payload...)
		lastTick = event.tick
	}
	out = append(out, 0x00, 0xFF, 0x2F, 0x00)
	return out
}

// metaTrackName 生成轨道名 meta 事件。
func metaTrackName(name string) []byte {
	clean := []byte(strings.TrimSpace(name))
	if len(clean) == 0 {
		clean = []byte("Lunar")
	}
	if len(clean) > 120 {
		clean = clean[:120]
	}
	out := []byte{0x00, 0xFF, 0x03, byte(len(clean))}
	return append(out, clean...)
}

// writeChunk 写入 MTrk 块（含长度）。
func writeChunk(buffer *bytes.Buffer, id string, data []byte) {
	buffer.WriteString(id)
	binary.Write(buffer, binary.BigEndian, uint32(len(data)))
	buffer.Write(data)
}

// vlq 编码 MIDI 可变长度数值。
func vlq(value int) []byte {
	if value < 0 {
		value = 0
	}
	encoded := []byte{byte(value & 0x7F)}
	for value >>= 7; value > 0; value >>= 7 {
		encoded = append(encoded, byte(value&0x7F)|0x80)
	}
	for left, right := 0, len(encoded)-1; left < right; left, right = left+1, right-1 {
		encoded[left], encoded[right] = encoded[right], encoded[left]
	}
	return encoded
}

// renderWithFluidsynth 调用本机 fluidsynth 生成 44.1kHz WAV。
func renderWithFluidsynth(midiPath, wavPath string) error {
	engine := config.MusicFluidSynth
	if engine == "" {
		engine = filepath.Join(config.LocalDir, "models", "music_generation", "fluidsynth", "fluidsynth.exe")
	}
	soundfont := config.MusicSoundFont
	if soundfont == "" {
		soundfont = filepath.Join(config.LocalDir, "models", "music_generation", "soundfonts", "general.sf2")
	}
	if _, err := os.Stat(engine); err != nil {
		return fmt.Errorf("未找到 fluidsynth: %s", engine)
	}
	if _, err := os.Stat(soundfont); err != nil {
		return fmt.Errorf("未找到音色库: %s", soundfont)
	}
	timeout := time.Duration(config.MusicRenderTimeout) * time.Second
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	command := exec.CommandContext(ctx, engine,
		"-i",
		"-T", "wav",
		"-r", "44100",
		"-g", "1.0",
		"-o", "synth.reverb.active=1",
		"-o", "synth.chorus.active=1",
		"-F", wavPath,
		soundfont,
		midiPath,
	)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("fluidsynth 渲染失败: %w（%s）", err, strings.TrimSpace(string(output)))
	}
	if info, statErr := os.Stat(wavPath); statErr != nil || info.Size() == 0 {
		return fmt.Errorf("fluidsynth 未生成有效音频: %s", strings.TrimSpace(string(output)))
	}
	return nil
}
