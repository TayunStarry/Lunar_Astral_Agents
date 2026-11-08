package AudioTranscriber

// agent.go 语音转写智能体：把 WAV 音频转成文本。
// 模型调用与推理实况统一走 protocol/agentbase。

import (
	"context"
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
	"LunarSubsystem/LunarHarness/protocol/agentinfo"
)

// AgentID 故障归属 ID
const AgentID = "audio_transcriber"

// transcribeMarker ASR 模型输出的正文标记
const transcribeMarker = "<asr_text>"

// Transcribe 调用语音识别模型转写 WAV 音频，返回纯文本。
func Transcribe(wavBase64 string) (text string, err error) {
	defer func() { agentbase.ReportRun(AgentID, err) }()
	ctx := context.Background()

	zero := float64(0)
	reply, err := agentbase.Stream(ctx, agentbase.StreamRequest{
		Agent: AgentID,
		Role:  agentinfo.NameAudioTranscriber,
		Model: config.AgentASRModel,
		Messages: []map[string]any{{
			"role": "user",
			"content": []map[string]any{{
				"type": "input_audio",
				"input_audio": map[string]any{
					"data":   wavBase64,
					"format": "wav",
				},
			}},
		}},
		Temperature: &zero,
		Timeout:     time.Duration(config.SyncFetchTimeout) * time.Second,
		InsecureTLS: true,
	})
	if err != nil {
		return "", fmt.Errorf("语音转写失败: %w", err)
	}

	raw := reply.Text
	if index := strings.Index(raw, transcribeMarker); index >= 0 {
		return strings.TrimSpace(raw[index+len(transcribeMarker):]), nil
	}
	return strings.TrimSpace(raw), nil
}
