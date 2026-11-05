package resource

// audio_ingest.go 音频接入：转码 → ASR 转写 → 登记悬空句柄。
//
// 与图片 / 视频的 Admit 同级：音频在接入阶段完成理解（转写），
// 句柄只保留「有描述、无实体」的记录；渲染侧只读取句柄的 ParseText。

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/multimodal"
	"LunarSubsystem/LunarHarness/internal/textutil"
	"LunarSubsystem/LunarHarness/protocol/modellease"
)

// IngestAudio 音频接入：转码 → ASR 转写 → 登记悬空句柄。
// 返回规范句柄与包含转写的描述文本；音频本体在转码临时文件中使用后即销毁。
func IngestAudio(source, name, origin string) (string, string, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return "", "", fmt.Errorf("音频来源为空")
	}
	// 本机只读路由补全为服务地址，交给多模态转码器直接读取
	if strings.HasPrefix(source, FileReadPrefix) {
		source = config.LocalBaseURL() + source
	}
	wav, err := multimodal.AudioToWavBase64(source)
	if err != nil || wav == "" {
		logger.Warn(audioModule, "[语音] 音频转码失败: %v", err)
		return "", "", err
	}
	transcript, err := transcribeAudio(wav)
	if err != nil {
		logger.Warn(audioModule, "[语音] 转写失败: %v", err)
		transcript = ""
	}
	text := "【语音转写】" + strings.TrimSpace(transcript)
	if strings.TrimSpace(transcript) == "" {
		text = ASRFallbackText
	}
	hash := sha256.Sum256([]byte(source))
	hashText := fmt.Sprintf("%x", hash)[:16]
	result, err := AdmitAudioTranscript(AudioTranscriptInput{
		Name:        name,
		Transcript:  text,
		ContentHash: hashText,
		Origin:      origin,
	})
	if err != nil {
		logger.Warn(audioModule, "[语音] 悬空句柄登记失败: %v", err)
		return "", text, err
	}
	logger.Info(audioModule, "[语音] 已转写并登记音频句柄 %s（%s）", result.Handle, textutil.TruncateRunes(text, 512))
	return result.Handle, text, nil
}

// UnloadASRModel 卸载语音识别模型（幂等：未加载时为空操作）。
// 定向卸载经 protocol/modellease 的单一入口统一执行。
func UnloadASRModel() {
	if _, err := modellease.Unload(config.AgentASRModel); err != nil {
		logger.Warn(audioModule, "ASR 卸载执行失败: %v", err)
		return
	}
	logger.Info(audioModule, "音频转写全部完成，已卸载 ASR 模型[%s]", config.AgentASRModel)
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

// chatCompletionJSON 非流式模型调用（语音识别等短请求）
func chatCompletionJSON(body map[string]any) (map[string]any, error) {
	url := config.AgentMultimodalURL + "/chat/completions"
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: time.Duration(config.SyncFetchTimeout) * time.Second, Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	responseRaw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var parsed map[string]any
	if err := json.Unmarshal(responseRaw, &parsed); err != nil {
		return nil, fmt.Errorf("模型响应解析失败: %w", err)
	}
	if errValue, ok := parsed["error"]; ok {
		return nil, fmt.Errorf("模型服务错误 [%d]: %v", resp.StatusCode, errValue)
	}
	return parsed, nil
}
