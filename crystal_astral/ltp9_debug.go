package main

// ==== LTP9 调试信封（WebSocket ↔ StarLTP 引擎桥） ====
// 前端引擎管理器页经 /ws 集线器发送 ltp9/event|call|stats|broadcast 信封，
// 本文件把它们转交给 StarLTP 引擎（Emit/Call/PluginStates），并把结果回执经 /ws 广播给页面。

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

	star "CrystalAstral/agent/StarLTP"
	kokoro "CrystalAstral/kokoro_tts"
	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/LoggerGeneral"
	ipmodule "LunarSubsystem/MultimodalAnalysis/module"
)

// ltp9HandleInbound 尝试按 LTP9 调试信封处理一条 /ws 入站消息；返回 true 表示已消费（不再交给 LTP3）。
func ltp9HandleInbound(data []byte) bool {
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return false
	}
	typ, _ := m["type"].(string)
	if !strings.HasPrefix(typ, "ltp9/") {
		return false
	}
	switch typ {
	case "ltp9/event":
		ltp9HandleEvent(m)
	case "ltp9/call":
		ltp9HandleCall(m)
	case "ltp9/tool":
		ltp9HandleTool(m)
	case "ltp9/stats":
		ltp9HandleStats(m)
	case "ltp9/broadcast":
		ltp9HandleBroadcast(m)
	case "ltp9/probe":
		ltp9HandleProbe(m)
	case "ltp9/test":
		ltp9HandleTest(m)
	default:
		LoggerGeneral.Info("StarLTP", "未识别的 ltp9 信封: %s", typ)
	}
	return true
}

// ltp9HandleEvent engine_emit：向 LTP9 所有订阅该 topic 的插件派发事件。
func ltp9HandleEvent(m map[string]any) {
	reqID, _ := m["request_id"].(string)
	topic, _ := m["topic"].(string)
	payload := m["payload"]
	if topic == "" {
		return
	}
	result := star.Emit(topic, payload, reqID)
	out, _ := json.Marshal(map[string]any{"type": "ltp9/result", "request_id": reqID, "topic": topic, "result": result})
	ltp9Broadcast(out)
}

// ltp9HandleCall engine.call：调用目标插件导出的函数。
func ltp9HandleCall(m map[string]any) {
	reqID, _ := m["request_id"].(string)
	plugin, _ := m["plugin"].(string)
	fn, _ := m["fn"].(string)
	var args []any
	if a, ok := m["args"].([]any); ok {
		args = a
	}
	res := map[string]any{}
	if value, err := star.Call(plugin, fn, args); err != nil {
		res["error"] = err.Error()
	} else {
		res["value"] = value
	}
	out, _ := json.Marshal(map[string]any{"type": "ltp9/result", "request_id": reqID, "plugin": plugin, "fn": fn, "result": res})
	ltp9Broadcast(out)
}

// ltp9HandleTool engine.tool：调用目标插件注册的工具。
func ltp9HandleTool(m map[string]any) {
	reqID, _ := m["request_id"].(string)
	plugin, _ := m["plugin"].(string)
	tool, _ := m["tool"].(string)
	params, _ := m["params"].(map[string]any)
	if params == nil {
		params = map[string]any{}
	}
	res := map[string]any{}
	if value, err := star.CallTool(plugin, tool, params); err != nil {
		res["error"] = err.Error()
	} else {
		res["value"] = value
	}
	out, _ := json.Marshal(map[string]any{"type": "ltp9/result", "request_id": reqID, "plugin": plugin, "tool": tool, "result": res})
	ltp9Broadcast(out)
}

// ltp9HandleStats 查询 LTP9 已加载插件状态 + Mini-LTP/Node-LTP 前端智能体包。
func ltp9HandleStats(m map[string]any) {
	reqID, _ := m["request_id"].(string)
	plugins := star.PluginStates()
	out, _ := json.Marshal(map[string]any{"type": "ltp9/stats_ack", "request_id": reqID, "plugins": plugins, "agent_plugins": star.AgentPackages()})
	ltp9Broadcast(out)
}

// ltp9HandleBroadcast 页面发起的 LTP9 广播：转交引擎 engine.signal（所有插件 frontEvent.signal 接收），并回执。
func ltp9HandleBroadcast(m map[string]any) {
	reqID, _ := m["request_id"].(string)
	if m["payload"] != nil {
		star.Broadcast(m["payload"])
	}
	out, _ := json.Marshal(map[string]any{"type": "ltp9/broadcast_ack", "request_id": reqID})
	ltp9Broadcast(out)
}

// ltp9HandleProbe 探测 LTP9 引擎链路：返回在线状态 + 已加载插件 + 前端智能体包。
func ltp9HandleProbe(m map[string]any) {
	reqID, _ := m["request_id"].(string)
	plugins := star.PluginStates()
	out, _ := json.Marshal(map[string]any{"type": "ltp9/pong", "request_id": reqID, "engine": "ltp9", "plugins": plugins, "agent_plugins": star.AgentPackages()})
	ltp9Broadcast(out)
}

// ltp9Broadcast 非阻塞地把结果回执推送到 /ws 集线器。
func ltp9Broadcast(data []byte) {
	if StudioHubInstance == nil {
		return
	}
	select {
	case StudioHubInstance.Broadcast <- data:
	default:
	}
}

// ltp9HandleTest 处理 ltp9/test 信封：按 action 分发到 StarLTP 探针接口，并回执 ltp9/test_ack。
// action 集合：agent / broadcast_target / file / db / memory / crypto(encode|decode) / jwt / llm / command / http / sleep。
func ltp9HandleTest(m map[string]any) {
	reqID, _ := m["request_id"].(string)
	action, _ := m["action"].(string)
	ack := func(value any, err error) {
		res := map[string]any{"type": "ltp9/test_ack", "request_id": reqID, "action": action}
		if err != nil {
			res["ok"] = false
			res["error"] = err.Error()
		} else {
			res["ok"] = true
			res["value"] = value
		}
		out, _ := json.Marshal(res)
		ltp9Broadcast(out)
	}

	asString := func(k string) string { s, _ := m[k].(string); return s }
	asMap := func(k string) map[string]any { v, _ := m[k].(map[string]any); return v }
	asFloat := func(k string) float64 { v, _ := m[k].(float64); return v }

	switch action {
	case "agent":
		appID := asString("plugin")
		instruction := asString("instruction")
		if instruction == "" {
			instruction = asString("text")
		}
		if appID == "" {
			ack(nil, fmt.Errorf("agent 需提供 plugin"))
			return
		}
		v, err := star.Agent(appID, instruction)
		ack(v, err)
	case "broadcast_target":
		target := asString("target")
		if target == "" {
			ack(nil, fmt.Errorf("broadcast_target 需提供 target"))
			return
		}
		star.BroadcastTo(target, m["payload"])
		ack(nil, nil)
	case "file":
		op := asString("op")
		if op == "" {
			op = "write"
		}
		path := asString("path")
		data := asString("data")
		if op == "write_b64" {
			// write_b64：把 base64（兼容 data URI 前缀与空白）解码为二进制后落盘，
			// 供「保存二进制文件」场景使用；Go string→[]byte 无损，可直接写入原始字节。
			payload := strings.TrimSpace(data)
			if strings.HasPrefix(payload, "data:") {
				if i := strings.Index(payload, ","); i >= 0 {
					payload = payload[i+1:]
				}
			}
			payload = strings.Map(func(r rune) rune {
				switch r {
				case ' ', '\t', '\r', '\n':
					return -1
				}
				return r
			}, payload)
			raw, err := base64.StdEncoding.DecodeString(payload)
			if err != nil {
				ack(nil, fmt.Errorf("base64 解码失败: %w", err))
				return
			}
			ack(star.ProbeFile("write", path, string(raw)), nil)
			return
		}
		ack(star.ProbeFile(op, path, data), nil)
	case "db":
		op := asString("op")
		if op == "" {
			op = "query"
		}
		var params []any
		if p, ok := m["params"].([]any); ok {
			params = p
		}
		ack(star.ProbeDB(op, asString("sql"), params), nil)
	case "memory":
		op := asString("op")
		if op == "" {
			op = "store"
		}
		v := asMap("params")
		if v == nil {
			v = map[string]any{}
		}
		ack(star.ProbeMemory(op, v), nil)
	case "crypto":
		op := asString("op")
		key := asString("key")
		if op == "decode" {
			v := star.ProbeDecode(key, asString("content"))
			ack(v, nil)
			return
		}
		v := star.ProbeEncode(key, asString("content"))
		ack(v, nil)
	case "base64":
		// Base64 编解码：encode 文本 → base64 字符串；decode base64 → 文本。
		// 解码兼容 data URI（data:audio/wav;base64,...）：自动剥离前缀，并容忍粘贴混入的空白/换行。
		op := asString("op")
		if op == "" {
			op = "encode"
		}
		content := asString("content")
		if op == "decode" {
			payload := strings.TrimSpace(content)
			if strings.HasPrefix(payload, "data:") {
				if i := strings.Index(payload, ","); i >= 0 {
					payload = payload[i+1:]
				}
			}
			payload = strings.Map(func(r rune) rune {
				switch r {
				case ' ', '\t', '\r', '\n':
					return -1
				}
				return r
			}, payload)
			v, err := base64.StdEncoding.DecodeString(payload)
			if err != nil {
				ack(nil, fmt.Errorf("base64 解码失败: %w", err))
				return
			}
			ack(map[string]any{"ok": true, "value": string(v)}, nil)
			return
		}
		ack(map[string]any{"ok": true, "value": base64.StdEncoding.EncodeToString([]byte(content))}, nil)
	case "jwt":
		claims := asMap("claims")
		if claims == nil {
			claims = map[string]any{}
		}
		v := star.ProbeJWT(claims, asString("secret"), asString("alg"), asString("kid"))
		ack(v, nil)
	case "llm":
		var msgs []any
		if mm, ok := m["messages"].([]any); ok {
			msgs = mm
		}
		opts := asMap("opts")
		if opts == nil {
			opts = map[string]any{}
		}
		ack(star.ProbeLLM(msgs, opts), nil)
	case "embed":
		opts := asMap("opts")
		if opts == nil {
			opts = map[string]any{}
		}
		ack(star.ProbeLLMEmbed(m["input"], opts), nil)
	case "tool":
		pluginID := asString("plugin")
		tool := asString("tool")
		if pluginID == "" || tool == "" {
			ack(nil, fmt.Errorf("tool 需提供 plugin 与 tool"))
			return
		}
		params := asMap("params")
		if params == nil {
			params = map[string]any{}
		}
		ack(star.ProbeTool(pluginID, tool, params), nil)
	case "emoji":
		op := asString("op")
		if op == "" {
			op = "search"
		}
		params := asMap("params")
		if params == nil {
			params = map[string]any{}
		}
		ack(star.ProbeEmoji(op, params), nil)
	case "platform":
		method := asString("method")
		if method == "" {
			method = asString("op")
		}
		if method == "" {
			method = "getName"
		}
		params := asMap("params")
		if params == nil {
			params = map[string]any{}
		}
		ack(star.ProbePlatform(method, params), nil)
	case "command":
		// 指令触发：向目标插件（plugin 留空则向全部插件）派发指令文本
		pluginID := asString("plugin")
		text := asString("text")
		if text == "" {
			ack(nil, fmt.Errorf("command 需提供 text"))
			return
		}
		context := asMap("context")
		if pluginID == "" {
			ack(star.CommandAll(text, context), nil)
		} else {
			ack(star.Command(pluginID, text, context))
		}
	case "network":
		// 裸 TCP/UDP/DNS：resolveDNS/SRV 即时返回；connect/listen 建立套接字并登记句柄；sock* 操作句柄
		op := asString("op")
		if op == "" {
			ack(nil, fmt.Errorf("network 需提供 op"))
			return
		}
		args, _ := m["args"].(map[string]any)
		if args == nil {
			args = map[string]any{}
		}
		ack(star.ProbeNetwork(op, args), nil)
	case "async":
		// 异步子任务：run（可指定包+导出函数为任务体，否则耗时模拟）/ reportProgress / getStatus / list
		op := asString("op")
		if op == "" {
			ack(nil, fmt.Errorf("async 需提供 op"))
			return
		}
		params := asMap("params")
		ack(star.ProbeAsync(op, params), nil)
	case "http":
		method := asString("method")
		if method == "" {
			method = "GET"
		}
		headers := asMap("headers")
		if headers == nil {
			headers = map[string]any{}
		}
		ack(star.ProbeHTTP(method, asString("url"), asString("body"), headers), nil)
	case "sleep":
		ack(star.ProbeSleep(asFloat("ms")), nil)
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
		keng := kokoro.GetEngine()
		if keng == nil {
			ack(nil, fmt.Errorf("Kokoro 引擎未就绪"))
			return
		}
		var mix []kokoro.MixVoice
		if raw, ok := m["mix"].([]any); ok {
			for _, item := range raw {
				if im, ok := item.(map[string]any); ok {
					mv, _ := im["voice"].(string)
					var w float64
					if f, ok := im["weight"].(float64); ok {
						w = f
					}
					if mv != "" {
						mix = append(mix, kokoro.MixVoice{Voice: mv, Weight: w})
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
			"audio":       base64.StdEncoding.EncodeToString(kokoro.EncodePCMToWAV(samples, kokoro.SampleRate)),
			"phonemes":    phonemes,
			"voice":       asString("voice"),
			"sample_rate": kokoro.SampleRate,
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
		format := asString("format")
		if format == "" {
			format = "wav"
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
			converted, cerr := ipmodule.AudioToWavBase64(tempFile.Name())
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
		ack(nil, fmt.Errorf("未知 ltp9/test action: %s", action))
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
		LoggerGeneral.SubError("LunarCore", "LTP9", "月华语音识别返回 %d: %s", resp.StatusCode, string(data))
		return "", fmt.Errorf("月华语音识别接口返回状态码 %d", resp.StatusCode)
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
