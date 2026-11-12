package storyboard

// voice.go 逐镜配音：用分镜对应的故事原文合成语音，落盘为 WAV 并记录时长。
//
// 约定：
//   - 朗读文本一律取本镜的原文片段（Original），而不是分镜叙事；
//   - WAV 写入 audios/generated/<标题>/，与图片目录同名，幻灯片 HTML 用相对路径引用；
//   - 时长以 WAV 头的 data 长度 ÷ 每秒字节数为准，幻灯片展示时长直接用它。

import (
	"context"
	"encoding/binary"
	"fmt"
	"path"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/persistence"
	"LunarSubsystem/LunarHarness/engine/resource"
	"LunarSubsystem/LunarHarness/internal/textutil"
)

// voiceResult 单镜配音的落盘结果。
type voiceResult struct {
	// FilePath 相对 LocalDir 的 WAV 路径
	FilePath string
	// Handle 音频资源句柄
	Handle string
	// DurationMs 音频时长（毫秒）
	DurationMs int
}

// synthesizeShotVoice 用本镜原文合成语音，失败时按上限重试。
func synthesizeShotVoice(ctx context.Context, shot plannedShot, outputFolder string) (voiceResult, error) {
	if VoiceHook == nil {
		return voiceResult{}, fmt.Errorf("语音合成能力未接入，无法配音")
	}
	text := voiceText(shot)
	if text == "" {
		return voiceResult{}, fmt.Errorf("第 %d 镜没有可朗读的原文", shot.Index)
	}
	var lastErr error
	for attempt := 1; attempt <= maxVoiceAttempts; attempt++ {
		if ctx.Err() != nil {
			return voiceResult{}, fmt.Errorf("任务已取消或超时：%w", ctx.Err())
		}
		attemptCtx, cancel := context.WithTimeout(ctx, shotVoiceTimeout)
		result, err := VoiceHook(attemptCtx, VoiceRequest{
			Text: text,
			Task: fmt.Sprintf("演绎链第 %d 镜（%s）旁白朗读", shot.Index, shot.Scene),
		})
		cancel()
		if err == nil && len(result.Audio) > 0 {
			saved, saveErr := saveShotVoice(shot.Index, outputFolder, result.Audio)
			if saveErr == nil {
				return saved, nil
			}
			err = saveErr
		}
		if err == nil {
			err = fmt.Errorf("未返回音频数据")
		}
		lastErr = err
		logger.Warn(Module, "第 %d 镜第 %d 次配音失败: %v", shot.Index, attempt, err)
		if attempt < maxVoiceAttempts {
			select {
			case <-ctx.Done():
				return voiceResult{}, fmt.Errorf("任务已取消或超时：%w", ctx.Err())
			case <-time.After(retryBackoff):
			}
		}
	}
	return voiceResult{}, fmt.Errorf("第 %d 镜配音失败（已重试 %d 次）：%v", shot.Index, maxVoiceAttempts, lastErr)
}

// voiceText 取本镜朗读文本：优先原文，缺失时退回分镜叙事，并限制长度。
func voiceText(shot plannedShot) string {
	text := textutil.TruncateRunes(strings.TrimSpace(shot.Original), maxVoiceRunes)
	if strings.TrimSpace(text) != "" {
		return text
	}
	return textutil.TruncateRunes(strings.TrimSpace(shot.Narration), maxVoiceRunes)
}

// saveShotVoice 把 WAV 写入产出目录并登记资源句柄。
func saveShotVoice(index int, outputFolder string, audio []byte) (voiceResult, error) {
	folder := strings.TrimSpace(outputFolder)
	if folder == "" {
		folder = "未命名演绎"
	}
	rel := path.Join(voiceOutputRoot, folder, fmt.Sprintf("voice-%02d.wav", index))
	if err := persistence.WriteFileAtomic(rel, audio); err != nil {
		return voiceResult{}, fmt.Errorf("配音落盘失败：%w", err)
	}
	handle, _ := resource.UpsertPath(rel)
	return voiceResult{
		FilePath:   rel,
		Handle:     handle,
		DurationMs: wavDurationMs(audio),
	}, nil
}

// wavDurationMs 按 WAV 头计算时长；无法解析时返回 0（由调用方回退到估算时长）。
func wavDurationMs(data []byte) int {
	if len(data) < 12 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return 0
	}
	byteRate := 0
	dataSize := 0
	for offset := 12; offset+8 <= len(data); {
		id := string(data[offset : offset+4])
		size := int(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		body := offset + 8
		switch id {
		case "fmt ":
			if body+12 <= len(data) {
				byteRate = int(binary.LittleEndian.Uint32(data[body+8 : body+12]))
			}
		case "data":
			dataSize = size
			if body+dataSize > len(data) {
				dataSize = len(data) - body
			}
		}
		if size < 0 {
			break
		}
		offset = body + size + size%2
	}
	if byteRate <= 0 || dataSize <= 0 {
		return 0
	}
	return dataSize * 1000 / byteRate
}

// estimateSpeechMs 无配音时按字数估算展示时长，保证幻灯片仍能按节奏推进。
func estimateSpeechMs(text string) int {
	count := len([]rune(strings.TrimSpace(text)))
	if count <= 0 {
		return fallbackSlideMs
	}
	ms := count * perRuneSpeechMs
	if ms < fallbackSlideMs {
		return fallbackSlideMs
	}
	if ms > maxFallbackSlideMs {
		return maxFallbackSlideMs
	}
	return ms
}
