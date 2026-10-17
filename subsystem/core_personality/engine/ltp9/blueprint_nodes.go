package StarLTP

// ==== 蓝图节点执行函数表 ====
// 三类实现（对齐前端 execute.js runNode + envelope.js buildEnvelope + websocket.js coreOut）：
//   A. 本地直执行：start/wait/transform/extract/display/image_display/limit/event_start/clock_start/composite
//   B. 浏览器协作：microphone/speaker/audio_eq/image_confuse/video_keyframe → ltp9/bp_exec 请求前端执行
//   C. 引擎信封类：构建信封后直调引擎（Emit/Call/Broadcast/execTestAction），不经 WS 自环

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// bpNodePorts 节点端口表（同步自 assets/planning/js/node-types.js，仅端口名不含表单）。
// 每类节点始终额外提供「顺序」gate 端口；数据端口缺失时 gate 即唯一端口。
// composite 为动态端口（inMap/outMap），见 bpPortExists。
type bpPortDef struct {
	ins  []string
	outs []string
}

var bpNodePorts = map[string]bpPortDef{
	"start":            {},
	"event_start":      {ins: nil, outs: []string{"payload"}},
	"clock_start":      {ins: nil, outs: []string{"tick"}},
	"limit":            {},
	"event":            {ins: []string{"payload"}, outs: []string{"result"}},
	"broadcast_all":    {ins: []string{"payload"}, outs: []string{"result"}},
	"broadcast_target": {ins: []string{"payload"}, outs: []string{"result"}},
	"call":             {ins: []string{"input"}, outs: []string{"result"}},
	"command":          {ins: []string{"text", "context"}, outs: []string{"result"}},
	"agent":            {ins: []string{"instruction"}, outs: []string{"text"}},
	"db":               {ins: []string{"sql", "params"}, outs: []string{"rows"}},
	"memory":           {ins: []string{"params"}, outs: []string{"result"}},
	"file":             {ins: []string{"path", "data"}, outs: []string{"text"}},
	"crypto":           {ins: []string{"content", "key"}, outs: []string{"text"}},
	"base64":           {ins: []string{"content"}, outs: []string{"text"}},
	"jwt":              {ins: []string{"claims", "kid"}, outs: []string{"token"}},
	"llm":              {ins: []string{"content"}, outs: []string{"answer"}},
	"http":             {ins: []string{"url", "body", "headers"}, outs: []string{"resp"}},
	"network":          {ins: []string{"handle", "data"}, outs: []string{"result"}},
	"async":            {ins: []string{"taskId", "progress"}, outs: []string{"result"}},
	"transform":        {ins: []string{"in1", "in2", "in3", "in4"}, outs: []string{"text"}},
	"extract":          {ins: []string{"input"}, outs: []string{"out1", "out2", "out3", "out4"}},
	"display":          {ins: []string{"text"}, outs: []string{"text"}},
	"image_display":    {ins: []string{"image"}, outs: []string{"image"}},
	"image_confuse":    {ins: []string{"image"}, outs: []string{"image"}},
	"video_keyframe":   {ins: []string{"url"}, outs: []string{"gif"}},
	"wait":             {},
	"stats":            {ins: nil, outs: []string{"rows"}},
	"emoji":            {ins: []string{"query", "image"}, outs: []string{"result"}},
	"embed":            {ins: []string{"input"}, outs: []string{"result"}},
	"kokoro_tts":       {ins: []string{"text"}, outs: []string{"audio"}},
	"qwen_tts":         {ins: []string{"text"}, outs: []string{"audio"}},
	"omnivoice_tts":    {ins: []string{"text"}, outs: []string{"audio"}},
	"qwen_asr":         {ins: []string{"audio"}, outs: []string{"text"}},
	"microphone":       {ins: nil, outs: []string{"audio"}},
	"speaker":          {ins: []string{"audio"}, outs: nil},
	"audio_eq":         {ins: []string{"audio"}, outs: []string{"audio"}},
}

// bpPrimaryOutKey 节点的主输出端口名（coreOut 主值字段提取候选第一位）。
func bpPrimaryOutKey(nodeType string) string {
	if d, ok := bpNodePorts[nodeType]; ok && len(d.outs) > 0 {
		return d.outs[0]
	}
	return ""
}

// bpPortExists 端口有效性判定（复合子图快照连线清洗用，对齐前端 portExists）。
func bpPortExists(spec bpNodeSpec, side, port string) bool {
	if spec.Type == "composite" {
		if side == "in" {
			if m, ok := spec.Params["inMap"].([]any); ok && len(m) > 0 {
				for _, x := range m {
					if mm, ok := x.(map[string]any); ok {
						if k, _ := mm["k"].(string); k == port {
							return true
						}
					}
				}
				return false
			}
			return port == "gate"
		}
		if _, has := spec.Params["outMap"]; has {
			return port == "out"
		}
		return port == "gate"
	}
	d, ok := bpNodePorts[spec.Type]
	if !ok {
		return port == "gate"
	}
	ports := d.ins
	if side == "out" {
		ports = d.outs
	}
	if len(ports) == 0 {
		return port == "gate"
	}
	for _, k := range ports {
		if k == port {
			return true
		}
	}
	return false
}

// ==== 节点分发 ====

// runNodeInner 单次节点执行：本地节点直执行、浏览器节点协作、其余走引擎信封路径。
func (r *bpRunner) runNodeInner(n *bpNodeRun) bpOutcome {
	p := n.spec.Params
	switch n.spec.Type {
	case "start":
		return bpOutcome{status: "ok", out: map[string]any{"started": true}, hadOut: true}
	case "wait":
		ms := bpNum(p, "ms", 500)
		if ms < 0 {
			ms = 0
		}
		select {
		case <-time.After(time.Duration(ms) * time.Millisecond):
			return bpOutcome{status: "ok", out: map[string]any{"slept_ms": ms}, hadOut: true}
		case <-r.haltCh:
			return bpOutcome{status: "skip"}
		}
	case "transform":
		return bpTransform(n)
	case "extract":
		return bpExtract(n)
	case "display":
		return bpDisplay(n)
	case "image_display":
		return bpImageDisplay(n)
	case "limit":
		max := int(bpNum(p, "max", 300))
		if max < 1 {
			max = 1
		}
		r.stMu.Lock()
		r.maxCalls = max
		r.stMu.Unlock()
		return bpOutcome{status: "ok", out: map[string]any{"maxCalls": max}, hadOut: true}
	case "event_start":
		return r.bpEventStart(n)
	case "clock_start":
		return r.bpClockStart(n)
	case "composite":
		return r.runComposite(n)
	case "microphone", "speaker", "audio_eq", "image_confuse", "video_keyframe":
		return r.bpExecBrowser(n)
	default:
		return r.bpEnvelopeNode(n)
	}
}

// ==== A. 本地直执行节点 ====

// bpTransform 提取合并（对齐前端 execute.js transform）：注入后的 in1..in4 参数按 e1..e4
// 路径提取，以 {{inN.path}} 占位符模板组合为单路输出。null 输出空串、对象 JSON 序列化、字符串原样；
// 占位符 root 不在 values 中时保留原文。
func bpTransform(n *bpNodeRun) bpOutcome {
	p := n.spec.Params
	template := strings.TrimSpace(bpStr(p, "template"))
	if template == "" {
		msg := "输出模板不能为空"
		return bpOutcome{status: "err", errText: msg, out: map[string]any{"error": msg}, hadOut: true}
	}
	values := map[string]any{}
	for _, port := range []string{"in1", "in2", "in3", "in4"} {
		v, has := p[port]
		if !has || v == nil {
			continue
		}
		path := strings.TrimSpace(bpStr(p, "e"+port[2:]))
		values[port] = bpExtractField(bpParseMaybeJSON(v), path)
	}
	out := bpTplRe.ReplaceAllStringFunc(template, func(match string) string {
		key := match[2 : len(match)-2]
		parts := bpPathKeys(key)
		if len(parts) == 0 {
			return match
		}
		v, ok := values[parts[0]]
		if !ok {
			return match
		}
		for _, seg := range parts[1:] {
			if m, ok := v.(map[string]any); ok {
				v = m[seg]
				continue
			}
			if arr, ok := v.([]any); ok {
				idx, err := strconv.Atoi(seg)
				if err == nil && idx >= 0 && idx < len(arr) {
					v = arr[idx]
					continue
				}
			}
			return "" // 路径中断（对齐前端 v==null → ''）
		}
		if v == nil {
			return ""
		}
		if s, ok := v.(string); ok {
			return s
		}
		b, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return string(b)
	})
	return bpOutcome{status: "ok", out: out, hadOut: true}
}

// bpTplRe transform 模板占位符（{{inN.path}}）。
var bpTplRe = regexp.MustCompile(`\{\{([^}]+)\}\}`)

// bpExtract 提取拆分（对齐前端 extract）：注入后的 input 参数按 e1..e4 路径拆分为多路输出
// （留空=原样透传整值）；out1 同时作为节点默认输出；input 回写参数供浮窗查看。
func bpExtract(n *bpNodeRun) bpOutcome {
	p := n.spec.Params
	v := p["input"]
	pv := bpParseMaybeJSON(v)
	multi := map[string]any{}
	for i, k := range []string{"e1", "e2", "e3", "e4"} {
		multi[fmt.Sprintf("out%d", i+1)] = bpExtractField(pv, strings.TrimSpace(bpStr(p, k)))
	}
	return bpOutcome{
		status: "ok", out: multi["out1"], hadOut: true, multi: multi,
		patch: map[string]any{"input": v},
	}
}

// bpDisplay 文本显示（对齐前端 display）：取注入后的 text 参数，原样输出并回写参数；
// 激活有内容时挂气泡（前端运行收尾统一弹出）。
func bpDisplay(n *bpNodeRun) bpOutcome {
	val := n.spec.Params["text"]
	out := bpOutcome{status: "ok", out: val, hadOut: true}
	if val != nil {
		out.patch = map[string]any{"text": val}
		if s, ok := val.(string); !ok || s != "" {
			out.pop = map[string]any{"kind": "text", "v": val}
		}
	}
	return out
}

// bpImageDisplay 图像显示（对齐前端 image_display）：识别图像（data URI / 裸 base64 magic bytes），
// 回写 data URI 并挂气泡；无图像但为非空字符串则原样透传；否则报错。
func bpImageDisplay(n *bpNodeRun) bpOutcome {
	val := n.spec.Params["image"]
	patch := map[string]any{}
	out := bpOutcome{patch: patch}
	if img := bpFindImage(val); img != nil {
		uri, _ := img["image"].(string)
		patch["image"] = uri
		out.status = "ok"
		out.out = uri
		out.hadOut = true
		out.pop = map[string]any{"kind": "image", "v": uri}
		return out
	}
	if s, ok := val.(string); ok && strings.TrimSpace(s) != "" {
		patch["image"] = s
		out.status = "ok"
		out.out = s
		out.hadOut = true
		return out
	}
	msg := "无图像输入（请连线或在参数中填入 base64）"
	out.status = "err"
	out.errText = msg
	out.out = map[string]any{"error": msg}
	out.hadOut = true
	return out
}

// bpEventStart 事件启动（对齐前端 event_start）：监测到目标事件 → 输出事件载荷并激活后续；
// 超时 → err；运行停止 → skip。
func (r *bpRunner) bpEventStart(n *bpNodeRun) bpOutcome {
	topic := strings.TrimSpace(bpStr(n.spec.Params, "topic"))
	timeout := time.Duration(bpNum(n.spec.Params, "timeout", 8000)) * time.Millisecond
	if timeout <= 0 {
		timeout = 8000 * time.Millisecond
	}
	w := r.registerEventWait(topic)
	defer r.removeEventWait(w)
	select {
	case hit := <-w.resultCh:
		payload := any(map[string]any{})
		if m, ok := hit.(map[string]any); ok {
			if pv, has := m["payload"]; has && pv != nil {
				payload = pv
			}
		}
		return bpOutcome{status: "ok", out: payload, hadOut: true}
	case <-time.After(timeout):
		msg := "等待事件超时"
		return bpOutcome{status: "err", errText: msg, out: map[string]any{"error": msg}, hadOut: true}
	case <-r.haltCh:
		return bpOutcome{status: "skip"}
	}
}

// bpClockStart 时钟启动（对齐前端 clock_start）：到达周期即激活后续节点，直至手动停止；
// 时钟节点自身立即以 ok 出池（等待周期投递不算节点执行）。
func (r *bpRunner) bpClockStart(n *bpNodeRun) bpOutcome {
	ms := bpNum(n.spec.Params, "interval", 2000)
	if ms < 50 {
		ms = 50
	}
	t := time.NewTicker(time.Duration(ms) * time.Millisecond)
	if !r.addClock(t) {
		return bpOutcome{status: "skip"}
	}
	go func() {
		for {
			select {
			case <-t.C:
				r.dispatchSuccessors(n.spec.ID)
			case <-r.haltCh:
				return
			}
		}
	}()
	return bpOutcome{status: "ok", out: map[string]any{"tick": true}, hadOut: true}
}

// ==== C. 引擎信封类节点 ====

// bpEnvelopeNode 引擎信封类节点：构建信封（buildEnvelope 移植）→ 直调引擎（不经 WS 自环）
// → coreOut 提取核心输出；失败判定对齐前端（ack.ok===false / result.error / result.value.ok===false）。
func (r *bpRunner) bpEnvelopeNode(n *bpNodeRun) bpOutcome {
	env, errMsg := bpBuildEnvelope(n.spec.Type, n.spec.Params)
	if env == nil {
		return bpOutcome{status: "err", errText: errMsg, out: map[string]any{"error": errMsg}, hadOut: true}
	}
	ack := r.bpExecEnvelope(env)
	if ack == nil {
		return bpOutcome{status: "skip"} // 运行已停止
	}
	if bpEnvelopeFailed(ack) {
		return bpOutcome{status: "err", errText: bpAckErrorText(ack), out: bpCoreOut(ack, n.spec.Type), hadOut: true}
	}
	return bpOutcome{status: "ok", out: bpCoreOut(ack, n.spec.Type), hadOut: true}
}

// bpExecEnvelope 执行一次引擎信封调用：event/call/broadcast/stats 直调宿主接口，
// 其余（ltp9/test）经 execTestAction 同步等待（TTS hook 可能异步调用 ack，用 channel 等待）。
// 返回 nil 表示运行已停止。
func (r *bpRunner) bpExecEnvelope(env map[string]any) map[string]any {
	switch env["type"] {
	case "ltp9/event":
		topic, _ := env["topic"].(string)
		res := Emit(topic, env["payload"], "bp")
		return map[string]any{"type": "ltp9/result", "result": res}
	case "ltp9/call":
		plugin, _ := env["plugin"].(string)
		fn, _ := env["fn"].(string)
		args, _ := env["args"].([]any)
		if args == nil {
			args = []any{}
		}
		res := map[string]any{}
		if value, err := Call(plugin, fn, args); err != nil {
			res["error"] = err.Error()
		} else {
			res["value"] = value
		}
		return map[string]any{"type": "ltp9/result", "result": res}
	case "ltp9/broadcast":
		Broadcast(env["payload"])
		return map[string]any{"type": "ltp9/broadcast_ack"}
	case "ltp9/stats":
		return map[string]any{"type": "ltp9/stats_ack", "plugins": PluginStates(), "agent_plugins": AgentPackages()}
	}
	// ltp9/test
	action, _ := env["action"].(string)
	ch := make(chan bpAckResult, 1)
	execTestAction(action, env, func(v any, e error) {
		select {
		case ch <- bpAckResult{value: v, err: e}:
		default:
		}
	})
	select {
	case res := <-ch:
		ack := map[string]any{"type": "ltp9/test_ack", "action": action}
		if res.err != nil {
			ack["ok"] = false
			ack["error"] = res.err.Error()
		} else {
			ack["ok"] = true
			ack["value"] = res.value
		}
		return ack
	case <-time.After(bpNodeTimeout):
		return map[string]any{"type": "ltp9/test_ack", "action": action, "ok": false, "error": "节点执行超时"}
	case <-r.haltCh:
		return nil
	}
}

// bpEnvelopeFailed 失败判定（对齐前端 execute.js 的 ack 错误判定）。
func bpEnvelopeFailed(ack map[string]any) bool {
	if ack == nil {
		return true
	}
	if f, ok := ack["ok"].(bool); ok && !f {
		return true
	}
	if rv, ok := ack["result"].(map[string]any); ok {
		if s, ok := rv["error"].(string); ok && s != "" {
			return true
		}
		if v, ok := rv["value"].(map[string]any); ok {
			if f, ok := v["ok"].(bool); ok && !f {
				return true
			}
		}
	}
	return false
}

// bpAckErrorText 从失败信封中提取错误文本（供 bp_node 推送与节点状态展示）。
func bpAckErrorText(ack map[string]any) string {
	if s, ok := ack["error"].(string); ok && s != "" {
		return s
	}
	if rv, ok := ack["result"].(map[string]any); ok {
		if s, ok := rv["error"].(string); ok && s != "" {
			return s
		}
		if v, ok := rv["value"].(map[string]any); ok {
			if s, ok := v["error"].(string); ok && s != "" {
				return s
			}
			if s, ok := v["text"].(string); ok && s != "" {
				return s
			}
		}
	}
	return "节点执行失败"
}

// bpBuildEnvelope 组装节点信封（对齐前端 envelope.js buildEnvelope；toast 校验改为返回错误消息）。
// 返回 nil 表示参数未完整配置（errMsg 为原因）。
func bpBuildEnvelope(nodeType string, p map[string]any) (map[string]any, string) {
	s := func(k string) string {
		v, has := p[k]
		if !has || v == nil {
			return ""
		}
		if str, ok := v.(string); ok {
			return str
		}
		b, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return string(b)
	}
	parseVal := func(k string, asArray bool) any {
		v, has := p[k]
		if !has || v == nil {
			if asArray {
				return []any{}
			}
			return map[string]any{}
		}
		str, ok := v.(string)
		if !ok {
			return v
		}
		t := strings.TrimSpace(str)
		if t == "" {
			if asArray {
				return []any{}
			}
			return map[string]any{}
		}
		var out any
		if json.Unmarshal([]byte(t), &out) == nil {
			return out
		}
		return v
	}

	switch nodeType {
	case "event":
		topic := strings.TrimSpace(s("topic"))
		if topic == "" {
			return nil, "请填写事件主题"
		}
		return map[string]any{"type": "ltp9/event", "topic": topic, "payload": parseVal("payload", false)}, ""
	case "broadcast_all":
		if pv, has := p["payload"]; !has || pv == nil || bpEmptyStr(pv) {
			return nil, "请填写广播载荷"
		}
		return map[string]any{"type": "ltp9/broadcast", "payload": parseVal("payload", false)}, ""
	case "broadcast_target":
		target := strings.TrimSpace(s("target"))
		if target == "" {
			return nil, "请填写目标包ID"
		}
		return map[string]any{"type": "ltp9/test", "action": "broadcast_target", "target": target, "payload": parseVal("payload", false)}, ""
	case "call":
		plugin := strings.TrimSpace(s("plugin"))
		fn := strings.TrimSpace(s("fn"))
		if plugin == "" || fn == "" {
			return nil, "请填写目标包与函数名"
		}
		var args []any
		// 输入数据连线注入优先：作为首个参数
		if linked := p["input"]; linked != nil && linked != "" {
			args = []any{linked}
		} else if raw := strings.TrimSpace(s("args")); raw != "" {
			var j any
			if json.Unmarshal([]byte(raw), &j) == nil {
				if arr, ok := j.([]any); ok {
					args = arr
				} else {
					args = []any{j}
				}
			} else {
				args = []any{raw} // 单个文本 → 首个参数
			}
		}
		if args == nil {
			args = []any{}
		}
		return map[string]any{"type": "ltp9/call", "plugin": plugin, "fn": fn, "args": args}, ""
	case "agent":
		plugin := strings.TrimSpace(s("plugin"))
		instr := p["instruction"]
		if plugin == "" || instr == nil || bpEmptyStr(instr) {
			return nil, "请填写智能体包ID与指令"
		}
		return map[string]any{"type": "ltp9/test", "action": "agent", "plugin": plugin, "instruction": s("instruction")}, ""
	case "db":
		params := parseVal("params", true)
		if _, ok := params.([]any); !ok {
			params = []any{}
		}
		op := s("op")
		if op == "" {
			op = "query"
		}
		return map[string]any{"type": "ltp9/test", "action": "db", "op": op, "sql": s("sql"), "params": params}, ""
	case "memory":
		op := s("op")
		if op == "" {
			op = "store"
		}
		return map[string]any{"type": "ltp9/test", "action": "memory", "op": op, "params": parseVal("params", false)}, ""
	case "file":
		op := s("op")
		if op == "" {
			op = "write"
		}
		return map[string]any{"type": "ltp9/test", "action": "file", "op": op, "path": s("path"), "data": s("data")}, ""
	case "crypto":
		op := s("op")
		if op == "" {
			op = "encode"
		}
		return map[string]any{"type": "ltp9/test", "action": "crypto", "op": op, "key": s("key"), "content": s("content")}, ""
	case "base64":
		op := s("op")
		if op == "" {
			op = "encode"
		}
		return map[string]any{"type": "ltp9/test", "action": "base64", "op": op, "content": s("content")}, ""
	case "jwt":
		claims := parseVal("claims", false)
		if claims == nil {
			claims = map[string]any{}
		}
		alg := s("alg")
		if alg == "" {
			alg = "HS256"
		}
		return map[string]any{"type": "ltp9/test", "action": "jwt", "claims": claims, "secret": s("secret"), "alg": alg, "kid": s("kid")}, ""
	case "llm":
		raw := strings.TrimSpace(s("content"))
		if raw == "" {
			return nil, "请填写消息内容"
		}
		opts, _ := parseVal("opts", false).(map[string]any)
		if opts == nil {
			opts = map[string]any{}
		}
		// 单消息框：可解析且符合 [{role,content}] 消息数组格式 → 视为消息列表；否则视为一条用户消息
		var messages []any
		var j any
		if json.Unmarshal([]byte(raw), &j) == nil {
			if arr, ok := j.([]any); ok && len(arr) > 0 {
				allRoles := true
				for _, mm := range arr {
					mmm, ok := mm.(map[string]any)
					if !ok {
						allRoles = false
						break
					}
					if _, ok := mmm["role"].(string); !ok {
						allRoles = false
						break
					}
				}
				if allRoles {
					messages = arr
				}
			}
		}
		if messages == nil {
			messages = []any{map[string]any{"role": "user", "content": raw}}
		}
		if sys := strings.TrimSpace(s("system")); sys != "" {
			opts["system"] = sys
		}
		return map[string]any{"type": "ltp9/test", "action": "llm", "messages": messages, "opts": opts}, ""
	case "command":
		text := strings.TrimSpace(s("text"))
		if text == "" {
			return nil, "请填写指令文本"
		}
		env := map[string]any{"type": "ltp9/test", "action": "command", "text": text, "context": parseVal("context", false)}
		if plugin := strings.TrimSpace(s("plugin")); plugin != "" {
			env["plugin"] = plugin
		}
		return env, ""
	case "emoji":
		op := s("op")
		if op == "" {
			op = "search"
		}
		params := map[string]any{"query": s("query"), "limit": int(bpNum(p, "limit", 5)), "image": s("image")}
		return map[string]any{"type": "ltp9/test", "action": "emoji", "op": op, "params": params}, ""
	case "embed":
		if p["input"] == nil && strings.TrimSpace(s("input")) == "" {
			return nil, "请填写待嵌入文本"
		}
		return map[string]any{"type": "ltp9/test", "action": "embed", "input": parseVal("input", false)}, ""
	case "kokoro_tts":
		if strings.TrimSpace(s("text")) == "" {
			return nil, "请填写合成文本"
		}
		speed := bpNum(p, "speed", 1)
		if speed == 0 {
			speed = 1
		}
		lang := s("lang")
		if lang == "" {
			lang = "auto"
		}
		return map[string]any{"type": "ltp9/test", "action": "kokoro_tts", "text": s("text"), "voice": strings.TrimSpace(s("voice")), "speed": speed, "lang": lang}, ""
	case "qwen_tts":
		if strings.TrimSpace(s("text")) == "" {
			return nil, "请填写合成文本"
		}
		env := map[string]any{"type": "ltp9/test", "action": "qwen_tts", "text": s("text")}
		if ref := strings.TrimSpace(s("ref_audio")); ref != "" {
			env["ref_audio"] = ref
		}
		return env, ""
	case "omnivoice_tts":
		if strings.TrimSpace(s("text")) == "" {
			return nil, "请填写合成文本"
		}
		env := map[string]any{"type": "ltp9/test", "action": "omnivoice_tts", "text": s("text")}
		if v := strings.TrimSpace(s("lang")); v != "" {
			env["lang"] = v
		}
		if v := strings.TrimSpace(s("instruct")); v != "" {
			env["instruct"] = v
		}
		if v := strings.TrimSpace(s("ref_audio")); v != "" {
			env["ref_audio"] = v
		}
		if v := strings.TrimSpace(s("ref_text")); v != "" {
			env["ref_text"] = v
		}
		if seed := bpNum(p, "seed", 0); seed > 0 {
			env["seed"] = seed
		}
		return env, ""
	case "qwen_asr":
		if strings.TrimSpace(s("audio")) == "" {
			return nil, "请填写音频 base64"
		}
		format := s("format")
		if format == "" {
			format = "wav"
		}
		return map[string]any{"type": "ltp9/test", "action": "qwen_asr", "audio": s("audio"), "format": format}, ""
	case "network":
		op := s("op")
		if op == "" {
			op = "resolveDNS"
		}
		handle := ""
		if p["handle"] != nil {
			handle = fmt.Sprint(p["handle"])
		}
		data := ""
		if p["data"] != nil {
			data = fmt.Sprint(p["data"])
		}
		args := map[string]any{
			"host": s("host"), "port": int(bpNum(p, "port", 0)), "service": s("service"), "proto": s("proto"),
			"handle": handle, "data": data, "timeout": bpNum(p, "timeout", 0),
		}
		return map[string]any{"type": "ltp9/test", "action": "network", "op": op, "args": args}, ""
	case "async":
		op := s("op")
		if op == "" {
			op = "run"
		}
		params := map[string]any{
			"plugin": strings.TrimSpace(s("plugin")), "fn": strings.TrimSpace(s("fn")),
			"args": parseVal("args", true), "ms": bpNum(p, "ms", 0), "timeout": bpNum(p, "timeout", 0),
			"task_id": int(bpNum(p, "taskId", 0)), "progress": parseVal("progress", false),
		}
		return map[string]any{"type": "ltp9/test", "action": "async", "op": op, "params": params}, ""
	case "http":
		method := s("method")
		if method == "" {
			method = "GET"
		}
		headers, _ := parseVal("headers", false).(map[string]any)
		if headers == nil {
			headers = map[string]any{}
		}
		return map[string]any{"type": "ltp9/test", "action": "http", "method": method, "url": s("url"), "body": s("body"), "headers": headers}, ""
	case "stats":
		return map[string]any{"type": "ltp9/stats"}, ""
	case "tool", "platform":
		return nil, "该能力已在 LTP9-Flash 中移除: " + nodeType
	}
	return nil, "未知节点类型: " + nodeType
}

// bpEmptyStr 值是否为空字符串（信封参数校验用，非字符串视为非空）。
func bpEmptyStr(v any) bool {
	s, ok := v.(string)
	return ok && s == ""
}

// bpCoreOut 提取节点核心输出（对齐前端 websocket.js coreOut，下游直接可用）：
//   - event/call/stats 节点输出原始回执不做简化
//   - ltp9/test_ack → value；ltp9/result → result.value；ltp9/stats_ack → plugins
//   - {ok:true,value:X} 包装递归剥离（上限 8 层）
//   - 按节点声明输出字段（out.k）→ text → body 顺序取主值（字符串/数字）
func bpCoreOut(ack map[string]any, nodeType string) any {
	if ack == nil {
		return nil
	}
	if nodeType == "event" || nodeType == "call" || nodeType == "stats" {
		return ack
	}
	if f, ok := ack["ok"].(bool); ok && !f {
		return ack // 失败信封原样保留，便于在日志/详情中看到错误
	}
	v := any(ack)
	switch ack["type"] {
	case "ltp9/test_ack":
		v = ack["value"]
	case "ltp9/result":
		if rv, ok := ack["result"].(map[string]any); ok {
			if val, has := rv["value"]; has {
				v = val
			} else {
				v = rv
			}
		} else if ack["result"] != nil {
			v = ack["result"]
		}
	case "ltp9/stats_ack":
		if plugins, has := ack["plugins"]; has {
			v = plugins
		}
	}
	for i := 0; i < 8; i++ {
		m, ok := v.(map[string]any)
		if !ok {
			break
		}
		if f, ok := m["ok"].(bool); ok && f {
			if val, has := m["value"]; has {
				v = val
				continue
			}
		}
		break
	}
	if m, ok := v.(map[string]any); ok {
		cand := []string{}
		if k := bpPrimaryOutKey(nodeType); k != "" && k != "gate" {
			cand = append(cand, k)
		}
		cand = append(cand, "text", "body")
		for _, k := range cand {
			switch val := m[k].(type) {
			case string:
				if val != "" {
					return val
				}
			case float64:
				return val
			}
		}
	}
	return v
}

// ==== D. 复合节点扁平执行 ====

// runComposite 复合节点扁平执行（对齐前端 runCompositeNode）：子图节点经 ID 重映射
// （原ID~cN 后缀）后加入扁平命名空间，与主图节点同层并行（无视图栈、无嵌套执行），
// 可多实例/循环激活且互不串扰；复合节点自身等待派生激活全部出池，
// 再以出口节点输出作为自身输出并激活后续节点。
func (r *bpRunner) runComposite(n *bpNodeRun) bpOutcome {
	p := n.spec.Params
	sg, _ := p["subgraph"].(map[string]any)
	sub := &bpGraph{}
	if sg != nil {
		if raw, err := json.Marshal(sg); err == nil {
			_ = json.Unmarshal(raw, sub)
		}
	}
	if len(sub.Nodes) == 0 {
		msg := "复合节点为空"
		return bpOutcome{status: "err", errText: msg, out: map[string]any{"error": msg}, hadOut: true}
	}
	inMap, _ := p["inMap"].([]any)

	comp := &bpCompState{ownerID: n.spec.ID, doneCh: make(chan struct{})}
	inst := map[string]*bpNodeRun{} // 子图原始 ID → 克隆节点
	instLinks := []bpLinkSpec{}
	r.stMu.Lock()
	r.comps[n.spec.ID] = comp
	// 克隆子图节点并重映射 ID（避免与主图/其他实例冲突）
	for i := range sub.Nodes {
		sn := sub.Nodes[i]
		if sn.ID == "" || sn.Type == "" {
			continue
		}
		if sn.Params == nil {
			sn.Params = map[string]any{}
		}
		r.compSeq++
		clone := &bpNodeRun{
			spec: bpNodeSpec{ID: sn.ID + "~c" + strconv.Itoa(r.compSeq), Type: sn.Type, Params: sn.Params, Enabled: sn.Enabled},
			comp: comp,
		}
		inst[sn.ID] = clone
		r.nodes[clone.spec.ID] = clone
	}
	// 子图快照连线清洗：端点端口不在节点当前端口集合中的历史连线（如冗余顺序线）直接剔除
	for _, l := range sub.Links {
		f := inst[l.From.Node]
		t := inst[l.To.Node]
		if f == nil || t == nil {
			continue
		}
		if !bpPortExists(f.spec, "out", l.From.Port) || !bpPortExists(t.spec, "in", l.To.Port) {
			continue
		}
		instLinks = append(instLinks, l)
	}
	r.links = append(r.links, instLinks...)
	r.stMu.Unlock()

	// 外部输入：按连线源端口取上游输出（gate 顺序线不作为数据输入），注入到映射的内部节点参数
	extIn := map[string]any{}
	for _, l := range r.linksSnapshot() {
		if l.To.Node != n.spec.ID || l.To.Port == "gate" || l.From.Port == "gate" {
			continue
		}
		if sv := r.nodeOut(r.nodes[l.From.Node], l.From.Port); sv != nil {
			extIn[l.To.Port] = sv
		}
	}
	for _, mAny := range inMap {
		m, ok := mAny.(map[string]any)
		if !ok {
			continue
		}
		mk, _ := m["k"].(string)
		mnode, _ := m["node"].(string)
		mport, _ := m["port"].(string)
		if v, has := extIn[mk]; has {
			if clone := inst[mnode]; clone != nil {
				clone.spec.Params[mport] = v
			}
		}
	}
	// 入口：无内部入线的节点立即加入执行池（与主图节点同层并行，受全局执行数量上限约束）
	internalIncoming := map[string]bool{}
	for _, l := range instLinks {
		internalIncoming[l.To.Node] = true
	}
	for _, clone := range inst {
		if !internalIncoming[clone.spec.ID] {
			r.enterPool(clone.spec.ID, comp)
		}
	}
	// 无可入池节点（如内部成环）时直接排空
	r.stMu.Lock()
	if !comp.closed && comp.active == 0 {
		comp.closed = true
		close(comp.doneCh)
	}
	r.stMu.Unlock()

	// 等待派生激活全部出池
	select {
	case <-comp.doneCh:
	case <-r.haltCh:
		r.removeComposite(n.spec.ID, comp, inst)
		return bpOutcome{status: "skip"}
	}
	r.removeComposite(n.spec.ID, comp, inst)

	// 内部显示类节点的弹出消息汇总到复合节点（统一回放）；出口取值：声明的出口节点输出即复合输出
	pops := []any{}
	hasErr := false
	for _, clone := range inst {
		clone.outMu.Lock()
		if clone.pop != nil {
			pops = append(pops, clone.pop)
		}
		if clone.status == "err" {
			hasErr = true
		}
		clone.outMu.Unlock()
	}
	out := bpOutcome{}
	if len(pops) > 0 {
		out.pop = map[string]any{"kind": "multi", "pops": pops}
	}
	if om, ok := p["outMap"].(map[string]any); ok {
		exitID, _ := om["node"].(string)
		if exit := inst[exitID]; exit != nil {
			exit.outMu.Lock()
			out.status = exit.status
			out.out = exit.primary
			out.hadOut = exit.hadOut
			exit.outMu.Unlock()
		} else {
			out.status = "ok"
		}
	} else if hasErr {
		out.status = "err"
	} else {
		out.status = "ok"
	}
	if out.status == "err" && out.errText == "" {
		out.errText = "复合节点内部存在错误"
	}
	return out
}

// removeComposite 清理复合实例：移除派生节点与派生连线（进行中的激活持有节点指针，不受影响）。
func (r *bpRunner) removeComposite(ownerID string, _ *bpCompState, inst map[string]*bpNodeRun) {
	instIDs := map[string]bool{}
	for _, clone := range inst {
		instIDs[clone.spec.ID] = true
	}
	r.stMu.Lock()
	delete(r.comps, ownerID)
	filtered := make([]bpLinkSpec, 0, len(r.links))
	for _, l := range r.links {
		if !instIDs[l.From.Node] && !instIDs[l.To.Node] {
			filtered = append(filtered, l)
		}
	}
	r.links = filtered
	for id := range instIDs {
		delete(r.nodes, id)
	}
	r.stMu.Unlock()
}

// ==== 取值 / 路径工具（对齐前端 utils.js 的执行侧函数） ====

// bpPathKeys 解析取值路径：支持 a.b[0].c 或 a.b.0.c 混合写法 → ['a','b','0','c']（对齐 pathKeys）。
func bpPathKeys(path string) []string {
	s := bpIdxRe.ReplaceAllString(path, ".$1")
	parts := strings.Split(s, ".")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// bpIdxRe 数组下标写法 [n] → .n。
var bpIdxRe = regexp.MustCompile(`\[(\d+)\]`)

// bpExtractField 按路径从对象取值；path 为空返回对象本身（对齐前端 extractField）。
func bpExtractField(obj any, path string) any {
	if obj == nil {
		return nil
	}
	if strings.TrimSpace(path) == "" {
		return obj
	}
	v := obj
	for _, k := range bpPathKeys(path) {
		if v == nil {
			return nil
		}
		if m, ok := v.(map[string]any); ok {
			v = m[k]
			continue
		}
		if arr, ok := v.([]any); ok {
			idx, err := strconv.Atoi(k)
			if err != nil || idx < 0 || idx >= len(arr) {
				return nil
			}
			v = arr[idx]
			continue
		}
		return nil
	}
	return v
}

// bpParseMaybeJSON 字符串形如 JSON（{ 或 [ 开头）时尝试解析为对象，失败/非字符串原样返回
// （引擎回执经 coreOut 提取后 text 常为 JSON 文本形态，按路径取字段前需先解析）。
func bpParseMaybeJSON(v any) any {
	s, ok := v.(string)
	if !ok {
		return v
	}
	t := strings.TrimSpace(s)
	if t == "" || (t[0] != '{' && t[0] != '[') {
		return v
	}
	var out any
	if json.Unmarshal([]byte(t), &out) == nil {
		return out
	}
	return v
}

// ==== 图像识别（对齐前端 render.js findMedia 的图像分支） ====

// bpFindImage 递归识别图像：data:image/ 前缀或裸 base64（magic bytes：PNG/JPEG/GIF/WEBP），
// 返回 {image: dataURI}；未识别返回 nil。
func bpFindImage(v any) map[string]any {
	switch t := v.(type) {
	case string:
		s := strings.TrimSpace(t)
		if strings.HasPrefix(s, "data:image/") {
			return map[string]any{"image": s}
		}
		if len(s) > 60 && len(s)%4 == 0 && bpIsB64Charset(s) {
			if raw, err := base64.StdEncoding.DecodeString(s); err == nil {
				if mime := bpSniffImageMime(raw); mime != "" {
					return map[string]any{"image": "data:" + mime + ";base64," + s}
				}
			}
		}
		return nil
	case []any:
		for _, x := range t {
			if r := bpFindImage(x); r != nil {
				return r
			}
		}
	case map[string]any:
		for _, x := range t {
			if r := bpFindImage(x); r != nil {
				return r
			}
		}
	}
	return nil
}

// bpIsB64Charset 是否为合法 base64 字符集（容忍粘贴混入的空白）。
func bpIsB64Charset(s string) bool {
	for _, c := range s {
		ok := (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') ||
			c == '+' || c == '/' || c == '=' || c == ' ' || c == '\n' || c == '\r' || c == '\t'
		if !ok {
			return false
		}
	}
	return true
}

// bpSniffImageMime 按解码后的 magic bytes 识别图像 MIME（与前端 sniffMime 的图像分支一致）。
func bpSniffImageMime(b []byte) string {
	switch {
	case len(b) > 8 && b[0] == 0x89 && b[1] == 0x50 && b[2] == 0x4e && b[3] == 0x47:
		return "image/png"
	case len(b) > 3 && b[0] == 0xff && b[1] == 0xd8 && b[2] == 0xff:
		return "image/jpeg"
	case len(b) > 6 && (string(b[:6]) == "GIF87a" || string(b[:6]) == "GIF89a"):
		return "image/gif"
	case len(b) > 12 && string(b[:4]) == "RIFF" && string(b[8:12]) == "WEBP":
		return "image/webp"
	}
	return ""
}
