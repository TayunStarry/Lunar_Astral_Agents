package main

// ==== LTP9 宿主侧接线（StarLTP 引擎的宿主服务胶水） ====
// 引擎本体与调试信封处理位于 engine/9.1-Flash（package StarLTP）；
// 本文件只承载依赖宿主服务的能力：
//   - 前端智能体通道（agent.synergy → Mini-LTP / Node-LTP，经 /ws ltpx_call 链路）
//   - 调试回执推送（/ws 集线器广播）
//   - 宿主服务测试动作（kokoro_tts / qwen_tts / qwen_asr）

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"CrystalAstral/engine/9.1-Flash"
	"CrystalAstral/engine/KokoroTTS"
	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/LoggerGeneral"
	Multimodal "LunarSubsystem/MultimodalAnalysis/module"
)

// BridgeLTP9 初始化 LTP9-Flash 引擎并注入宿主通道（幂等）。
func BridgeLTP9() error {
	// 引擎初始化 + 前端智能体真实通道 + 单向通报日志
	err := StarLTP.Bridge(ltpInvokeFrontAgent, func(topic string, payload any) {
		LoggerGeneral.Info("StarLTP", "插件单向通报 %s: %v", topic, payload)
	})
	// 调试回执推送：经 /ws 集线器广播给引擎管理器页
	StarLTP.SetDebugPush(func(data []byte) {
		if StudioHubInstance == nil {
			return
		}
		select {
		case StudioHubInstance.Broadcast <- data:
		default:
		}
	})
	// 宿主服务测试动作：Kokoro TTS / 月华 TTS·ASR 代理
	StarLTP.SetTestActionHook(ltp9HostTestAction)
	return err
}

// ltpInvokeFrontAgent 复用 LTPX 前端包调用链路：登记待定调用 → /ws 广播 ltpx_call → 等待包回执。
// 等价于 ltpRemoteCallHandler 的「前端包工具」分支，供 agent.synergy 调用 Mini-LTP / Node-LTP。
func ltpInvokeFrontAgent(appID, instruction string) (string, error) {
	if StudioHubInstance == nil {
		return "", fmt.Errorf("%s 包拒绝响应（前端总线未就绪）", appID)
	}
	if appID == "" {
		return "", fmt.Errorf("包 ID 为空")
	}
	requestID := fmt.Sprintf("ltp9-ag-%d", time.Now().UnixNano())
	done := make(chan LTPXRemoteCallResponse, 1)
	ltpPendingMutex.Lock()
	ltpPendingCalls[requestID] = done
	ltpPendingMutex.Unlock()

	msg, _ := json.Marshal(map[string]any{
		"type":       "ltpx_call",
		"request_id": requestID,
		"app_id":     appID,
		"tool":       "",
		"arguments":  map[string]any{"instruction": instruction},
	})
	// 非阻塞广播：总线拥塞时撤销登记并按调用失败处理，不阻塞插件事件循环/inbound 消费者
	select {
	case StudioHubInstance.Broadcast <- msg:
	default:
		ltpPendingMutex.Lock()
		delete(ltpPendingCalls, requestID)
		ltpPendingMutex.Unlock()
		return "", fmt.Errorf("%s 包拒绝响应（前端总线广播缓冲已满）", appID)
	}

	select {
	case resp := <-done:
		if !resp.Success {
			return resp.Text, fmt.Errorf("%s 包执行失败: %s", appID, ltp9FirstNonEmpty(resp.Error, "未知错误"))
		}
		return resp.Text, nil
	case <-time.After(ltpCallTimeout):
		ltpPendingMutex.Lock()
		delete(ltpPendingCalls, requestID)
		ltpPendingMutex.Unlock()
		return "", fmt.Errorf("%s 包拒绝响应（前端调用超时 %s）", appID, ltpCallTimeout)
	}
}

// ltp9FirstNonEmpty 返回首个非空字符串。
func ltp9FirstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// ltp9HostTestAction 宿主服务测试动作分发（经 star.SetTestActionHook 注入引擎调试信封）。
func ltp9HostTestAction(action string, m map[string]any, ack func(any, error)) {
	asString := func(k string) string { s, _ := m[k].(string); return s }
	asFloat := func(k string) float64 { v, _ := m[k].(float64); return v }

	switch action {
	case "kokoro_tts":
		// Kokoro 语音合成：text → base64 WAV 音频（voice/speed/lang 可选，mix 可多音色混合）
		text := asString("text")
		if text == "" {
			ack(nil, fmt.Errorf("kokoro_tts 需提供 text"))
			return
		}
		if err := ensureKokoro(); err != nil {
			ack(nil, err)
			return
		}
		keng := KokoroTTS.GetEngine()
		if keng == nil {
			ack(nil, fmt.Errorf("Kokoro 引擎未就绪"))
			return
		}
		var mix []KokoroTTS.MixVoice
		if raw, ok := m["mix"].([]any); ok {
			for _, item := range raw {
				if im, ok := item.(map[string]any); ok {
					mv, _ := im["voice"].(string)
					var w float64
					if f, ok := im["weight"].(float64); ok {
						w = f
					}
					if mv != "" {
						mix = append(mix, KokoroTTS.MixVoice{Voice: mv, Weight: w})
					}
				}
			}
		}
		samples, phonemes, serr := keng.Synthesize(text, asString("voice"), float32(asFloat("speed")), asString("lang"), mix)
		if serr != nil {
			ack(nil, serr)
			return
		}
		ack(map[string]any{
			"success":     true,
			"audio":       base64.StdEncoding.EncodeToString(KokoroTTS.EncodePCMToWAV(samples, KokoroTTS.SampleRate)),
			"phonemes":    phonemes,
			"voice":       asString("voice"),
			"sample_rate": KokoroTTS.SampleRate,
		}, nil)
	case "qwen_tts":
		// Qwen 语音合成：text → 经月华 /tts 代理合成 base64 音频（需月华服务在线）
		text := asString("text")
		if text == "" {
			ack(nil, fmt.Errorf("qwen_tts 需提供 text"))
			return
		}
		target := fmt.Sprintf("http://127.0.0.1:%d/tts", *GeneralConfig.EnginePort)
		reqBody, _ := json.Marshal(map[string]any{"text": text, "ref_audio": asString("ref_audio")})
		resp, err := http.Post(target, "application/json", strings.NewReader(string(reqBody)))
		if err != nil {
			ack(nil, fmt.Errorf("请求月华 /tts 失败: %w", err))
			return
		}
		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		var parsed map[string]any
		if json.Unmarshal(data, &parsed) == nil && parsed != nil {
			parsed["status"] = resp.StatusCode
			ack(parsed, nil)
			return
		}
		ack(map[string]any{"status": resp.StatusCode, "body": string(data)}, nil)
	case "qwen_asr":
		// 语音识别：base64 音频 → 经月华 system-asr（Qwen3-ASR）HTTP 接口转写为文本
		// wav/mp3/flac/ogg 由 llama-server 音频解码器直接识别；其他格式先经 ffmpeg 转为 16kHz 单声道 WAV
		audioB64 := asString("audio")
		if strings.TrimSpace(audioB64) == "" {
			ack(nil, fmt.Errorf("qwen_asr 需提供 audio (base64)"))
			return
		}
		// 兼容 data:audio/<格式>;base64,… data URI（TTS/均衡器节点输出即此形态），剥离前缀并按 URI 声明补全格式
		audioB64, uriFmt := stripDataURIPrefix(audioB64)
		if strings.TrimSpace(audioB64) == "" {
			ack(nil, fmt.Errorf("qwen_asr 音频数据为空（data URI 前缀剥离后）"))
			return
		}
		format := asString("format")
		if format == "" {
			format = uriFmt
		}
		if format == "" {
			format = "wav"
		}
		// 以真实音频头（magic bytes）校正格式：data URI 声明或用户填写可能与实际数据不符（如 webm 记为 wav），
		// 避免把一种格式的音频按另一种格式发给 llama-server 导致 400
		if real := sniffAudioFormat(audioB64); real != "" {
			format = real
		}
		if !slices.Contains([]string{"wav", "mp3", "flac", "ogg"}, format) {
			data, derr := base64.StdEncoding.DecodeString(audioB64)
			if derr != nil {
				ack(nil, fmt.Errorf("音频 base64 解码失败: %w", derr))
				return
			}
			tempFile, terr := os.CreateTemp("", "ltp9_asr_*."+format)
			if terr != nil {
				ack(nil, fmt.Errorf("创建临时音频文件失败: %w", terr))
				return
			}
			if _, werr := tempFile.Write(data); werr != nil {
				tempFile.Close()
				os.Remove(tempFile.Name())
				ack(nil, fmt.Errorf("写入临时音频文件失败: %w", werr))
				return
			}
			tempFile.Close()
			converted, cerr := Multimodal.AudioToWavBase64(tempFile.Name())
			os.Remove(tempFile.Name())
			if cerr != nil {
				ack(nil, fmt.Errorf("音频转码失败: %w", cerr))
				return
			}
			audioB64 = converted
			format = "wav"
		}
		text, aerr := transcribeViaYuehua(audioB64, format)
		if aerr != nil {
			ack(nil, aerr)
			return
		}
		ack(map[string]any{"text": text, "audio_format": format}, nil)
	default:
		ack(nil, fmt.Errorf("未知宿主测试动作: %s", action))
	}
}

// stripDataURIPrefix 剥离 data:audio/<格式>;base64,… 前缀，返回纯 base64 与 URI 声明的格式
func stripDataURIPrefix(s string) (b64 string, mimeFormat string) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "data:") {
		if idx := strings.Index(s, ";base64,"); idx >= 0 {
			mime := strings.TrimPrefix(s[:idx], "data:")
			if i := strings.Index(mime, "/"); i >= 0 {
				mimeFormat = mime[i+1:]
			}
			b64 = s[idx+len(";base64,"):]
		} else {
			b64 = s
		}
	} else {
		b64 = s
	}
	return strings.TrimSpace(b64), mimeFormat
}

// sniffAudioFormat 按音频 magic bytes 识别真实格式（wav/mp3/flac/ogg/webm），识别失败返回空串
func sniffAudioFormat(b64 string) string {
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil || len(data) < 12 {
		return ""
	}
	switch {
	case data[0] == 'R' && data[1] == 'I' && data[2] == 'F' && data[3] == 'F' && string(data[8:12]) == "WAVE":
		return "wav"
	case string(data[:4]) == "fLaC":
		return "flac"
	case string(data[:4]) == "OggS":
		return "ogg"
	case string(data[:3]) == "ID3" || (data[0] == 0xFF && (data[1]&0xE0) == 0xE0):
		return "mp3"
	case data[0] == 0x1A && data[1] == 0x45 && data[2] == 0xDF && data[3] == 0xA3:
		return "webm"
	default:
		return ""
	}
}

// transcribeViaYuehua 经 HTTP 调用月华的语音识别接口（system-asr，Qwen3-ASR）转写音频
// audioB64 为 llama-server 音频解码器原生支持格式（wav/mp3/flac/ogg）的 base64 编码
// 返回剥离 "language xxx<asr_text>" 前缀后的转写正文
func transcribeViaYuehua(audioB64 string, format string) (string, error) {
	target := strings.TrimSuffix(*GeneralConfig.AgentMultimodalURL, "/") + "/chat/completions"
	reqBody, _ := json.Marshal(map[string]any{
		"model": *GeneralConfig.AgentASRModel,
		"messages": []map[string]any{{
			"role": "user",
			"content": []map[string]any{{
				"type":        "input_audio",
				"input_audio": map[string]string{"data": audioB64, "format": format},
			}},
		}},
		"temperature": 0,
	})
	req, err := http.NewRequest("POST", target, strings.NewReader(string(reqBody)))
	if err != nil {
		return "", fmt.Errorf("构造语音识别请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if key := *GeneralConfig.AgentMultimodalKey; key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	// 首次请求会触发月华侧 ASR 模型按需加载，放宽超时
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求月华语音识别接口失败: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		body := string(data)
		if len(body) > 300 {
			body = body[:300] + "…"
		}
		LoggerGeneral.SubError("LunarCore", "LTP9", "月华语音识别返回 %d: %s", resp.StatusCode, string(data))
		return "", fmt.Errorf("月华语音识别接口返回状态码 %d: %s", resp.StatusCode, body)
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil || len(parsed.Choices) == 0 {
		return "", fmt.Errorf("月华语音识别响应解析失败: %w", err)
	}
	raw := parsed.Choices[0].Message.Content
	// 剥离 ASR 输出的语言标记前缀，仅保留转写正文
	const marker = "<asr_text>"
	if idx := strings.Index(raw, marker); idx >= 0 {
		raw = raw[idx+len(marker):]
	}
	return strings.TrimSpace(raw), nil
}
