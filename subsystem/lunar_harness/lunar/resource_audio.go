package lunar

// resource_audio.go 音频句柄的转写：解析文本优先，缺失才调用 audio_transcriber
//
// 规范三：音频解读在 use_resource 时发生；同一批次的音频全部转写完成后统一卸载 ASR，
// 避免逐条加载/卸载来回抖动。接入阶段只下载建句柄，不转写。

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	multimodal "LunarSubsystem/LunarHarness/engine/multimodal"
	"LunarSubsystem/LunarHarness/engine/resource"
)

// ASRFallbackText 转写失败时的中性提示
const ASRFallbackText = "（这段音频没有转写出可用文本）"

// errASRSourceMissing 音频本体不在本地且无法下载
var errASRSourceMissing = errors.New("音频本体缺失")

// audioParseTextOf 读取音频句柄的解析文本；缺失返回空串
func audioParseTextOf(handle string) string {
	res, ok := resource.Lookup(handle)
	if !ok || res.ParseState != resource.ParseReady {
		return ""
	}
	return strings.TrimSpace(res.ParseText)
}

// transcribeAudioResource 转写一个音频句柄并写回唯一解析文本
func transcribeAudioResource(handle string) (string, error) {
	res, err := resource.Resolve(handle)
	if err != nil {
		return "", err
	}
	if res.ParseState == resource.ParseReady && strings.TrimSpace(res.ParseText) != "" {
		return strings.TrimSpace(res.ParseText), nil
	}
	if res.Kind != resource.KindAudio {
		return "", nil
	}
	sourcePath := ""
	if res.BlobPath != "" {
		sourcePath = filepath.Join(config.LocalDir, filepath.FromSlash(res.BlobPath))
	} else if resource.IsRemoteURL(res.URL) {
		// 接入时下载失败的远程音频：解读阶段补下载一次
		if admitted, err := resource.Admit(resource.AdmitInput{
			SourceKind: resource.SourceKindURL,
			Source:     res.URL,
			Name:       res.Name,
			KindHint:   resource.KindAudio,
			Origin:     res.Origin,
		}); err == nil && admitted.BlobPath != "" {
			sourcePath = filepath.Join(config.LocalDir, filepath.FromSlash(admitted.BlobPath))
		}
	}
	if sourcePath == "" {
		return "", errASRSourceMissing
	}
	wav, err := multimodal.AudioToWavBase64(sourcePath)
	if err != nil || wav == "" {
		logger.Warn(lunarCoreModule, "[语音] 音频转换失败: %v", err)
		return "", err
	}
	transcript, err := transcribeAudio(wav)
	if err != nil {
		return "", err
	}
	text := "【语音转写】" + strings.TrimSpace(transcript)
	if strings.TrimSpace(transcript) == "" {
		text = ASRFallbackText
	}
	if err := resource.SetParseText(res.URL, text); err != nil {
		logger.Warn(lunarCoreModule, "[语音] 转写写入解析文本失败: %v", err)
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
