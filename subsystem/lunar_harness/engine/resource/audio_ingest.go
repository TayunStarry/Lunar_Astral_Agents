package resource

// audio_ingest.go 音频接入：落盘 audios → 强制类型校验 → 转码 → ASR 转写。
//
// 音频与其他模态共用同一套文件管理系统与句柄体系：实体按内容哈希去重落盘，
// 转写结果写回同一句柄的 ParseText，渲染侧只读取句柄文本。

import (
	"bytes"
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

// AdmitAudio 音频接入：落盘到 audios → 强制类型校验 → 转写 → 句柄携带转写文本。
//
// 音频与其他模态一样走统一文件管理系统（按内容哈希去重），转写结果写回同一句柄，
// 不再创建「有描述、无实体」的第二套悬空句柄。
func AdmitAudio(source, name, origin, declaredMIME, sourceKind string) (AdmitResult, string, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return AdmitResult{}, "", fmt.Errorf("音频来源为空")
	}
	admitted, err := Admit(AdmitInput{
		SourceKind: sourceKind,
		Source:     source,
		Name:       name,
		KindHint:   KindAudio,
		MIME:       declaredMIME,
		Origin:     origin,
		Dedup:      true,
	})
	if err != nil {
		return AdmitResult{}, "", err
	}
	if admitted.BlobPath != "" {
		if verifyErr := VerifyStored(KindAudio, AbsPathOfBlob(admitted.BlobPath), declaredMIME); verifyErr != nil {
			if !admitted.Deduped {
				_ = Delete(admitted.Handle)
			}
			return AdmitResult{}, "", verifyErr
		}
	}
	// 内容哈希命中说明这颗音频之前已经接入并完成过转写：
	// 直接复用既有转写文本，避免历史消息每轮重放都重跑一次 ASR。
	if admitted.Deduped {
		if text := ParseTextOfResource(admitted.Handle); text != "" {
			return admitted, text, nil
		}
	}
	local := AbsPathOfBlob(admitted.BlobPath)
	if local == "" {
		local = source
	}
	wav, err := multimodal.AudioToWavBase64(local)
	if err != nil || wav == "" {
		logger.Warn(audioModule, "[语音] 音频转码失败: %v", err)
		return admitted, ASRFallbackText, nil
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
	if admitted.URL != "" {
		if err := archiveDB.setURLParseText(admitted.URL, text); err != nil {
			logger.Warn(audioModule, "[语音] 转写写回句柄失败: %v", err)
		}
	}
	logger.Info(audioModule, "[语音] 已落盘并转写音频句柄 %s（%s）", admitted.Handle, textutil.TruncateRunes(text, 512))
	return admitted, text, nil
}

// IngestAudio 兼容旧入口：按来源自动判定 SourceKind，走同一套落盘与转写流程。
func IngestAudio(source, name, origin string) (string, string, error) {
	admitted, text, err := AdmitAudio(source, name, origin, "", classifySourceKind(source))
	if err != nil || admitted.Handle == "" {
		return "", text, err
	}
	return admitted.Handle, text, nil
}

// classifySourceKind 按来源形态给出 Admit 需要的 SourceKind
func classifySourceKind(source string) string {
	lower := strings.ToLower(strings.TrimSpace(source))
	switch {
	case strings.HasPrefix(lower, "data:"):
		return SourceKindData
	case strings.HasPrefix(lower, "http://"), strings.HasPrefix(lower, "https://"):
		return SourceKindURL
	default:
		return SourceKindPath
	}
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
