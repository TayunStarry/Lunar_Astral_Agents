package LunarPersonality

// resource_audio.go 音频接入与转写：不落盘，直接生成带描述的悬空句柄。
//
// 规范变更：
//   - 音频字节不再写入 audios/ 或 multimedia/，接入阶段即调用 audio_transcriber；
//   - 转写文本写入资源目录的悬空句柄（@A.nnn），句柄本身只是一条描述记录；
//   - 上下文直接拿到转写文本；只有用户引用包含音频的消息时，才按句柄读取描述。

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	multimodal "LunarSubsystem/LunarHarness/engine/multimodal"
	"LunarSubsystem/LunarHarness/engine/resource"
)

// ASRFallbackText 转写失败时的中性提示
const ASRFallbackText = "（这段音频没有转写出可用文本）"

// audioParseTextOf 读取音频句柄的解析文本；缺失返回空串
func audioParseTextOf(handle string) string {
	res, ok := resource.Lookup(handle)
	if !ok || res.ParseState != resource.ParseReady {
		return ""
	}
	return strings.TrimSpace(res.ParseText)
}

// ingestAudioSource 音频接入：转码 → ASR 转写 → 登记悬空句柄。
// 返回规范句柄与包含转写的描述文本；音频本体在转码临时文件中使用后即销毁。
func ingestAudioSource(source, name, origin string) (string, string, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return "", "", fmt.Errorf("音频来源为空")
	}
	// 本机只读路由补全为服务地址，交给多模态转码器直接读取
	if strings.HasPrefix(source, fileReadPath) {
		source = config.LocalBaseURL() + source
	}
	wav, err := multimodal.AudioToWavBase64(source)
	if err != nil || wav == "" {
		logger.Warn(lunarCoreModule, "[语音] 音频转码失败: %v", err)
		return "", "", err
	}
	transcript, err := transcribeAudio(wav)
	if err != nil {
		logger.Warn(lunarCoreModule, "[语音] 转写失败: %v", err)
		transcript = ""
	}
	text := "【语音转写】" + strings.TrimSpace(transcript)
	if strings.TrimSpace(transcript) == "" {
		text = ASRFallbackText
	}
	hash := sha256.Sum256([]byte(source))
	hashText := fmt.Sprintf("%x", hash)[:16]
	result, err := resource.AdmitAudioTranscript(resource.AudioTranscriptInput{
		Name:        name,
		Transcript:  text,
		ContentHash: hashText,
		Origin:      origin,
	})
	if err != nil {
		logger.Warn(lunarCoreModule, "[语音] 悬空句柄登记失败: %v", err)
		return "", text, err
	}
	logger.Info(lunarCoreModule, "[语音] 已转写并登记音频句柄 %s（%s）", result.Handle, truncateRunes(text, 80))
	return result.Handle, text, nil
}

// audioHandleContextText 音频句柄进入模型上下文时的统一文本形态。
// 解析文本优先；句柄只作为可追溯的描述标识，不读取任何音频字节。
func audioHandleContextText(handle, fallback string) string {
	handle = strings.TrimSpace(handle)
	text := strings.TrimSpace(fallback)
	if text == "" {
		text = audioParseTextOf(handle)
	}
	if text == "" {
		text = ASRFallbackText
	}
	if handle == "" {
		return text
	}
	return fmt.Sprintf("%s（%s）", handle, text)
}

// audioHandleContextItem 构造音频句柄内容项
func audioHandleContextItem(handle, text, name string) ContentItem {
	return ContentItem{Type: ContentAudioHandle, AudioHandle: &AudioHandlePayload{
		Handle: handle,
		Text:   text,
		Name:   name,
	}}
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
