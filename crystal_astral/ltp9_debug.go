package main

// ==== LTP9 调试信封（WebSocket ↔ StarLTP 引擎桥） ====
// 前端引擎管理器页经 /ws 集线器发送 ltp9/event|call|stats|broadcast 信封，
// 本文件把它们转交给 StarLTP 引擎（Emit/Call/PluginStates），并把结果回执经 /ws 广播给页面。

import (
	"encoding/json"
	"fmt"
	"strings"

	star "CrystalAstral/agent/StarLTP"
	"LunarSubsystem/LoggerGeneral"
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

// ltp9HandleStats 查询 LTP9 已加载插件状态。
func ltp9HandleStats(m map[string]any) {
	reqID, _ := m["request_id"].(string)
	plugins := star.PluginStates()
	out, _ := json.Marshal(map[string]any{"type": "ltp9/stats_ack", "request_id": reqID, "plugins": plugins})
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

// ltp9HandleProbe 探测 LTP9 引擎链路：返回在线状态 + 已加载插件。
func ltp9HandleProbe(m map[string]any) {
	reqID, _ := m["request_id"].(string)
	plugins := star.PluginStates()
	out, _ := json.Marshal(map[string]any{"type": "ltp9/pong", "request_id": reqID, "engine": "ltp9", "plugins": plugins})
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
// action 集合：agent / broadcast_target / file / db / memory / crypto(encode|decode) / jwt / llm / http / sleep。
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
		ack(star.ProbeFile(op, asString("path"), asString("data")), nil)
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
	default:
		ack(nil, fmt.Errorf("未知 ltp9/test action: %s", action))
	}
}