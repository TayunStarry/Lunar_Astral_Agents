package StarLTP

// ==== LTP9-Flash 调试信封（WebSocket ↔ 引擎桥） ====
// 前端引擎管理器页经 /ws 集线器发送 ltp9/event|call|stats|broadcast|test 信封，
// HandleInbound 把它们转交给引擎（Emit/Call/PluginStates/Probe*），回执经宿主注入的
// SetDebugPush 推送回页面。依赖宿主服务的测试动作（kokoro_tts/qwen_tts/qwen_asr）
// 经 SetTestActionHook 注入，未注入时返回错误提示。

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"LunarSubsystem/LoggerGeneral"
)

// ==== 宿主注入钩子 ====

var (
	debugPushMu    sync.RWMutex
	debugPush      func(data []byte)                                           // 回执推送（宿主 /ws 集线器）
	testActionHook func(action string, m map[string]any, ack func(any, error)) // 宿主服务测试动作
)

// SetDebugPush 注入调试回执推送函数（宿主 /ws 集线器广播）。
func SetDebugPush(fn func(data []byte)) {
	debugPushMu.Lock()
	debugPush = fn
	debugPushMu.Unlock()
}

// SetTestActionHook 注入依赖宿主服务的测试动作处理器（kokoro_tts / qwen_tts / qwen_asr）。
func SetTestActionHook(fn func(action string, m map[string]any, ack func(any, error))) {
	debugPushMu.Lock()
	testActionHook = fn
	debugPushMu.Unlock()
}

// debugEmit 非阻塞地把结果回执推送给宿主。
func debugEmit(data []byte) {
	debugPushMu.RLock()
	push := debugPush
	debugPushMu.RUnlock()
	if push == nil {
		return
	}
	push(data)
}

// HandleInbound 尝试按 LTP9-Flash 调试信封处理一条 /ws 入站消息；返回 true 表示已消费。
func HandleInbound(data []byte) bool {
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
		handleDebugEvent(m)
	case "ltp9/call":
		handleDebugCall(m)
	case "ltp9/stats":
		handleDebugStats(m)
	case "ltp9/broadcast":
		handleDebugBroadcast(m)
	case "ltp9/probe":
		handleDebugProbe(m)
	case "ltp9/test":
		handleDebugTest(m)
	default:
		LoggerGeneral.Info(ServiceName, "未识别的 ltp9 信封: %s", typ)
	}
	return true
}

// handleDebugEvent：向所有订阅该 topic 的插件派发事件。
func handleDebugEvent(m map[string]any) {
	reqID, _ := m["request_id"].(string)
	topic, _ := m["topic"].(string)
	payload := m["payload"]
	if topic == "" {
		return
	}
	result := Emit(topic, payload, reqID)
	out, _ := json.Marshal(map[string]any{"type": "ltp9/result", "request_id": reqID, "topic": topic, "result": result})
	debugEmit(out)
}

// handleDebugCall：调用目标插件导出的函数（callFunction）。
func handleDebugCall(m map[string]any) {
	reqID, _ := m["request_id"].(string)
	plugin, _ := m["plugin"].(string)
	fn, _ := m["fn"].(string)
	var args []any
	if a, ok := m["args"].([]any); ok {
		args = a
	}
	res := map[string]any{}
	if value, err := Call(plugin, fn, args); err != nil {
		res["error"] = err.Error()
	} else {
		res["value"] = value
	}
	out, _ := json.Marshal(map[string]any{"type": "ltp9/result", "request_id": reqID, "plugin": plugin, "fn": fn, "result": res})
	debugEmit(out)
}

// handleDebugStats：查询已加载插件状态 + Mini-LTP/Node-LTP 前端智能体包。
func handleDebugStats(m map[string]any) {
	reqID, _ := m["request_id"].(string)
	out, _ := json.Marshal(map[string]any{"type": "ltp9/stats_ack", "request_id": reqID, "plugins": PluginStates(), "agent_plugins": AgentPackages()})
	debugEmit(out)
}

// handleDebugBroadcast：页面发起的广播（所有插件 signal.subscribe 接收），并回执。
func handleDebugBroadcast(m map[string]any) {
	reqID, _ := m["request_id"].(string)
	if m["payload"] != nil {
		Broadcast(m["payload"])
	}
	out, _ := json.Marshal(map[string]any{"type": "ltp9/broadcast_ack", "request_id": reqID})
	debugEmit(out)
}

// handleDebugProbe：探测引擎链路：在线状态 + 已加载插件 + 前端智能体包。
func handleDebugProbe(m map[string]any) {
	reqID, _ := m["request_id"].(string)
	out, _ := json.Marshal(map[string]any{"type": "ltp9/pong", "request_id": reqID, "engine": "ltp9", "plugins": PluginStates(), "agent_plugins": AgentPackages()})
	debugEmit(out)
}

// handleDebugTest 处理 ltp9/test 信封：按 action 分发到探针接口，并回执 ltp9/test_ack。
// action 集合：agent / broadcast_target / file / db / memory / crypto(encode|decode) / base64 /
// jwt / llm / embed / emoji(图片记忆) / command / network / async / http / sleep；
// kokoro_tts / qwen_tts / qwen_asr 委托宿主注入的 testActionHook。
func handleDebugTest(m map[string]any) {
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
		debugEmit(out)
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
		v, err := Agent(appID, instruction)
		ack(v, err)
	case "broadcast_target":
		target := asString("target")
		if target == "" {
			ack(nil, fmt.Errorf("broadcast_target 需提供 target"))
			return
		}
		BroadcastTo(target, m["payload"])
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
			ack(ProbeFile("write", path, string(raw)), nil)
			return
		}
		ack(ProbeFile(op, path, data), nil)
	case "db":
		op := asString("op")
		if op == "" {
			op = "query"
		}
		var params []any
		if p, ok := m["params"].([]any); ok {
			params = p
		}
		ack(ProbeDB(op, asString("sql"), params), nil)
	case "memory":
		op := asString("op")
		if op == "" {
			op = "store"
		}
		v := asMap("params")
		if v == nil {
			v = map[string]any{}
		}
		ack(ProbeMemory(op, v), nil)
	case "crypto":
		op := asString("op")
		key := asString("key")
		if op == "decode" {
			ack(ProbeDecode(key, asString("content")), nil)
			return
		}
		ack(ProbeEncode(key, asString("content")), nil)
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
		ack(ProbeJWT(claims, asString("secret"), asString("alg"), asString("kid")), nil)
	case "llm":
		var msgs []any
		if mm, ok := m["messages"].([]any); ok {
			msgs = mm
		}
		opts := asMap("opts")
		if opts == nil {
			opts = map[string]any{}
		}
		ack(ProbeLLM(msgs, opts), nil)
	case "embed":
		opts := asMap("opts")
		if opts == nil {
			opts = map[string]any{}
		}
		ack(ProbeLLMEmbed(m["input"], opts), nil)
	case "emoji":
		// 图片记忆：旧 emoji 信封的 search/store/random 映射为 memory 的 searchImage/storeImage/randomImage
		op := asString("op")
		if op == "" {
			op = "search"
		}
		params := asMap("params")
		if params == nil {
			params = map[string]any{}
		}
		switch op {
		case "store":
			ack(ProbeMemory("storeImage", params), nil)
		case "random":
			ack(ProbeMemory("randomImage", params), nil)
		default:
			ack(ProbeMemory("searchImage", params), nil)
		}
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
			ack(CommandAll(text, context), nil)
		} else {
			ack(Command(pluginID, text, context))
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
		ack(ProbeNetwork(op, args), nil)
	case "async":
		// 异步子任务：run（可指定包+导出函数为任务体，否则耗时模拟）/ reportProgress / getStatus / list
		op := asString("op")
		if op == "" {
			ack(nil, fmt.Errorf("async 需提供 op"))
			return
		}
		ack(ProbeAsync(op, asMap("params")), nil)
	case "http":
		method := asString("method")
		if method == "" {
			method = "GET"
		}
		headers := asMap("headers")
		if headers == nil {
			headers = map[string]any{}
		}
		ack(ProbeHTTP(method, asString("url"), asString("body"), headers), nil)
	case "sleep":
		ack(ProbeSleep(asFloat("ms")), nil)
	case "kokoro_tts", "qwen_tts", "qwen_asr":
		// 依赖宿主服务（Kokoro 引擎 / 月华 TTS·ASR 代理），委托宿主注入的处理器
		debugPushMu.RLock()
		hook := testActionHook
		debugPushMu.RUnlock()
		if hook == nil {
			ack(nil, fmt.Errorf("宿主未注入 %s 服务", action))
			return
		}
		hook(action, m, ack)
	default:
		ack(nil, fmt.Errorf("未知 ltp9/test action: %s", action))
	}
}
