package lunar

// resource_audio.go 音频资源的统一处理：登记句柄 → 转写 → 批量卸载 ASR → 回填文本
//
// 音频是唯一需要「模型解析」的资源类型：转写本身要占用 audio_transcriber 的显存，
// 因此在同一批未读上下文里应当先全部转写、再统一卸载，避免逐条加载/卸载来回抖动。

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	multimodal "LunarSubsystem/LunarHarness/engine/multimodal"
)

// resourceAudioText 音频 → 「句柄 + 转写」文本；句柄优先，入库失败退化为纯转写
func resourceAudioText(source, name, origin string) string {
	media := admitMedia(source, name, origin, "A")
	if media != nil {
		if entry := getResourceEntry(media.Handle); entry != nil && entry.Summary != "" {
			return media.Handle + " " + entry.Summary
		}
	}
	transcript, err := understandAudio(source)
	if err != nil || strings.TrimSpace(transcript) == "" {
		if media != nil {
			return media.Handle + " " + asrFallbackText
		}
		return name
	}
	if media != nil {
		setResourceSummary(media.ID, transcript, nil)
		return media.Handle + " " + transcript
	}
	return transcript
}

// understandAudio 理解音频（ASR 转写），可缓存来源按来源地址复用结果
func understandAudio(source string) (string, error) {
	cacheable := !strings.HasPrefix(source, "data:")
	if cacheable {
		if cached := getPromptFromKnowledge(source); cached != "" {
			return cached, nil
		}
	}
	wav, err := multimodal.AudioToWavBase64(source)
	if err != nil || wav == "" {
		logger.Warn(lunarCoreModule, "[语音] 音频转换失败: %v", err)
		return "", err
	}
	transcript, err := transcribeAudio(wav)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(transcript) == "" {
		return "", nil
	}
	text := "【语音转写】" + transcript
	if cacheable {
		savePromptToKnowledge(source, text)
	}
	return text, nil
}

// transcribeAudio 调用语音识别模型（audio_transcriber）
func transcribeAudio(wavBase64 string) (string, error) {
	body := map[string]any{
		"model": config.AgentASRModel,
		"messages": []map[string]any{{
			"role": "user",
			"content": []map[string]any{{
				"type": "input_audio",
				"input_audio": map[string]any{
					"data":   wavBase64,
					"format": "wav",
				},
			}},
		}},
		"temperature": 0,
	}
	result, err := chatCompletionJSON(body)
	if err != nil {
		logger.Warn(lunarCoreModule, "[语音] 语音转写请求失败: %v", err)
		return "", err
	}
	choices, _ := result["choices"].([]any)
	if len(choices) == 0 {
		return "", nil
	}
	choice, _ := choices[0].(map[string]any)
	message, _ := choice["message"].(map[string]any)
	raw, _ := message["content"].(string)
	const marker = "<asr_text>"
	if index := strings.Index(raw, marker); index >= 0 {
		return strings.TrimSpace(raw[index+len(marker):]), nil
	}
	return strings.TrimSpace(raw), nil
}

// unloadASRModel 卸载语音识别模型（幂等：未加载时卸载接口本身是空操作）
func unloadASRModel() {
	body := map[string]any{"model": config.AgentASRModel}
	data, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, config.LocalBaseURL()+"/vram/unload", bytes.NewReader(data))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		logger.Warn(lunarCoreModule, "ASR 卸载执行失败: %v", err)
		return
	}
	resp.Body.Close()
	logger.Info(lunarCoreModule, "音频转写全部完成，已卸载 ASR 模型[%s]", config.AgentASRModel)
}
