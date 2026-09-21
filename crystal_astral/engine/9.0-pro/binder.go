package ltp9

// ==== engine.* 全局对象绑定（按 allow-* 权限注入） ====

import (
	"fmt"
	"os"
	"strings"
	"time"

	"LunarSubsystem/LoggerGeneral"
	"github.com/dop251/goja"
)

// bindEngine 组装并返回 engine 对象，注册进插件沙箱。
func bindEngine(vm *goja.Runtime, p *plugin) *goja.Object {
	engine := vm.NewObject()

	// 事件订阅器
	engine.Set("event", bindEvent(vm, p))
	// 前端事件（广播接收；allow-signal）
	if p.hasPerm("allow-signal") {
		engine.Set("frontEvent", bindFrontEvent(vm, p))
	}

	// 广播（allow-signal）
	if p.hasPerm("allow-signal") {
		engine.Set("signal", bindSignal(vm))
	}

	// 时间戳
	engine.Set("time", bindTime(vm))

	// 导出插件能力（供 engine.call）
	engine.Set("export", func(name string, fn goja.Value) {
		if fn == nil || !isFunc(fn) {
			return
		}
		p.mu.Lock()
		p.exports[name] = fn
		p.mu.Unlock()
	})

	// 跨包函数调用（调用方需 allow-call）
	if p.hasPerm("allow-call") {
		engine.Set("call", func(id string) *goja.Object {
			o := vm.NewObject()
			o.Set("run", func(fnName string, args []any) (any, error) {
				return engineCall(id, fnName, args)
			})
			return o
		})
	}

	// 调用前端智能体（allow-agent）
	if p.hasPerm("allow-agent") {
		engine.Set("agent", func(id string) *goja.Object {
			o := vm.NewObject()
			o.Set("run", func(text string) (any, error) {
				return engineAgent(id, text)
			})
			return o
		})
	}

	// 加解密（allow-certificate）
	if p.hasPerm("allow-certificate") {
		engine.Set("encoder", func(key, content string) (string, error) { return engineEncode(key, content) })
		engine.Set("decoder", func(key, cipher string) (string, error) { return engineDecode(key, cipher) })
	}

	// 文件（allow-file）
	if p.hasPerm("allow-file") {
		engine.Set("file", bindFile(vm, p))
	}

	// 记忆库（allow-memory）
	if p.hasPerm("allow-memory") {
		engine.Set("memory", bindMemory(vm))
	}

	// 数据库（allow-database）
	if p.hasPerm("allow-database") {
		engine.Set("database", bindDatabase(vm))
	}

	// 配置
	engine.Set("config", bindConfig(vm, p))

	// JWT 签名（allow-certificate）
	if p.hasPerm("allow-certificate") {
		engine.Set("crypto", bindCrypto(vm))
	}

	// 图像处理（allow-file；下载另需 allow-network）
	if p.hasPerm("allow-file") {
		engine.Set("image", bindImage(vm, p))
	}

	// LLM 对话（allow-agent，模型垄断下强制 agent 模型）
	if p.hasPerm("allow-agent") {
		engine.Set("llm", bindLLM(vm))
	}

	// Agent 工具注册（allow-agent）
	if p.hasPerm("allow-agent") {
		engine.Set("tool", bindTool(vm, p))
	}

	// 平台上下文（allow-send）
	if p.hasPerm("allow-send") {
		engine.Set("platform", bindPlatform(vm))
	}

	// 表情包（allow-memory：复用记忆库 stickers 集合）
	if p.hasPerm("allow-memory") {
		engine.Set("emoji", bindEmoji(vm))
	}

	// 发送到会话（allow-send）
	if p.hasPerm("allow-send") {
		engine.Set("send", bindSend(vm, p))
	}

	// WebSocket 服务端（allow-socket）
	if p.hasPerm("allow-socket") {
		engine.Set("ws", bindWs(vm, p))
	}

	// 同步 HTTP（allow-network）与同步休眠（常驻，供不用异步的插件使用）
	if p.hasPerm("allow-network") {
		engine.Set("http", bindHTTP(vm, p))
	}
	engine.Set("sleep", func(ms float64) { time.Sleep(time.Duration(ms) * time.Millisecond) })

	// 编解码工具（常驻基础能力，不参与 allow-* 开关）
	engine.Set("encoding", bindEncoding(vm))
	// 裸 TCP/UDP/DNS（allow-network）
	if p.hasPerm("allow-network") {
		engine.Set("network", bindNet(vm))
	}
	// 指令系统（常驻基础能力，经 Go 侧 Command/CommandAll 触发）
	engine.Set("command", bindCommand(vm, p))
	// 异步子任务（常驻基础能力）
	engine.Set("async", bindAsync(vm, p))

	return engine
}

// bindHTTP 同步阻塞的 HTTP 能力（engine.http.get/post/download）。
func bindHTTP(vm *goja.Runtime, p *plugin) *goja.Object {
	h := vm.NewObject()
	h.Set("get", func(url string, headers map[string]any) map[string]any {
		if headers == nil {
			headers = map[string]any{}
		}
		return httpGetSync(url, headers)
	})
	h.Set("post", func(url, body string, headers map[string]any) map[string]any {
		if headers == nil {
			headers = map[string]any{}
		}
		return httpPostSync(url, body, headers)
	})
	h.Set("download", func(url, savePath string) map[string]any { return httpDownloadSync(p, url, savePath) })
	return h
}

// bindEvent engine.event.subscribe/subscribe优先级/unsubscribe（基础能力，常驻注入）。
func bindEvent(vm *goja.Runtime, p *plugin) *goja.Object {
	event := vm.NewObject()
	event.Set("subscribe", func(topic string, cb goja.Value, priority goja.Value) int {
		if cb == nil || !isFunc(cb) {
			return 0
		}
		pri := priorityUnset
		if priority != nil && !goja.IsUndefined(priority) && !goja.IsNull(priority) {
			if n := priority.ToInteger(); n >= 0 {
				pri = int(n)
			}
		}
		p.mu.Lock()
		id := p.registerEvent(topic, cb, pri)
		p.mu.Unlock()
		return id
	})
	event.Set("unsubscribe", func(topic string, id int) {
		p.mu.Lock()
		p.removeEvent(topic, id)
		p.mu.Unlock()
	})
	event.Set("publish", func(topic string, payload any) {
		enginePublish(p, topic, payload)
	})
	return event
}

// bindFrontEvent engine.frontEvent.signal.subscribe/unsubscribe（allow-signal 绑定）。
func bindFrontEvent(vm *goja.Runtime, p *plugin) *goja.Object {
	front := vm.NewObject()
	fe := vm.NewObject()
	fe.Set("subscribe", func(cb goja.Value) int {
		if cb == nil || !isFunc(cb) {
			return 0
		}
		p.mu.Lock()
		p.subSeq++
		id := p.subSeq
		p.frontSignal = append(p.frontSignal, &eventSub{orderID: id, handler: cb})
		p.mu.Unlock()
		return id
	})
	fe.Set("unsubscribe", func(id int) {
		p.mu.Lock()
		kept := p.frontSignal[:0]
		for _, s := range p.frontSignal {
			if s.orderID != id {
				kept = append(kept, s)
			}
		}
		p.frontSignal = kept
		p.mu.Unlock()
	})
	front.Set("signal", fe)
	return front
}

// bindSignal engine.signal.all/target（广播到 frontEvent 或外部 outbound）。
func bindSignal(vm *goja.Runtime) *goja.Object {
	sig := vm.NewObject()
	sig.Set("all", func(payload any) { engineBroadcast("__all__", payload) })
	sig.Set("target", func(id string, payload any) { engineBroadcast(id, payload) })
	return sig
}

// bindTime engine.time.now()/nowMs()。
func bindTime(vm *goja.Runtime) *goja.Object {
	t := vm.NewObject()
	t.Set("now", func() int64 { return time.Now().Unix() })
	t.Set("nowMs", func() int64 { return time.Now().UnixMilli() })
	return t
}

// bindFile engine.file.write/read/delete（allow-file；路径限定插件数据目录）。
func bindFile(vm *goja.Runtime, p *plugin) *goja.Object {
	f := vm.NewObject()
	f.Set("write", func(path, data string) (any, error) { return fileWrite(p, path, data) })
	f.Set("read", func(path string) (any, error) { return fileRead(p, path) })
	f.Set("delete", func(path string) (any, error) { return fileDelete(p, path) })
	return f
}

// bindMemory engine.memory.store/search（allow-memory）。
func bindMemory(vm *goja.Runtime) *goja.Object {
	m := vm.NewObject()
	m.Set("store", func(v map[string]any) (any, error) { return memoryStore(v) })
	m.Set("search", func(v map[string]any) (any, error) { return memorySearch(v) })
	return m
}

// bindDatabase engine.database.*（allow-database）。
func bindDatabase(vm *goja.Runtime) *goja.Object {
	d := vm.NewObject()
	d.Set("query", func(sql string, params []any) (any, error) { return databaseQuery(sql, params) })
	d.Set("exec", func(sql string, params []any) (any, error) { return databaseExec(sql, params) })
	return d
}

// bindConfig engine.config.getFile/setFile/registerSchema。
func bindConfig(vm *goja.Runtime, p *plugin) *goja.Object {
	c := vm.NewObject()
	c.Set("getFile", func() map[string]any { return p.config })
	c.Set("setFile", func(v map[string]any) {
		p.mu.Lock()
		p.config = v
		p.mu.Unlock()
		_ = os.WriteFile(p.ConfigPath, []byte(marshalYAML(v)), 0644)
		p.fireConfigUpdate("plugin", v, "")
	})
	c.Set("registerSchema", func(v any) {
		p.registerConfigSchema(exportConfigSections(v))
	})
	return c
}

// exportConfigSections 把插件 JS 传入的 schema（[]ConfigSectionDef）从 goja 值导出为 Go 结构。
func exportConfigSections(v any) []ConfigSectionDef {
	if v == nil {
		return nil
	}
	if list, ok := v.([]any); ok {
		out := make([]ConfigSectionDef, 0, len(list))
		for _, item := range list {
			if m, ok := item.(map[string]any); ok {
				out = append(out, configSectionFromMap(m))
			}
		}
		return out
	}
	return nil
}

// configSectionFromMap 把单个节的 JS 对象转换为 ConfigSectionDef。
func configSectionFromMap(m map[string]any) ConfigSectionDef {
	s := ConfigSectionDef{Name: asString(m["name"]), Label: asString(m["label"]), Description: asString(m["description"])}
	if o, ok := m["order"].(int64); ok {
		s.Order = int(o)
	}
	if fields, ok := m["fields"].([]any); ok {
		for _, f := range fields {
			if fm, ok := f.(map[string]any); ok {
				s.Fields = append(s.Fields, ConfigFieldDef{
					Name:        asString(fm["name"]),
					Label:       asString(fm["label"]),
					Type:        asString(fm["type"]),
					Description: asString(fm["description"]),
					Default:     fm["default"],
				})
			}
		}
	}
	return s
}

// asString 把 goja 导出值安全转字符串。
func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// bindCrypto engine.crypto.*（allow-certificate）：摘要/HMAC/Ed25519/JWT 签名。
func bindCrypto(vm *goja.Runtime) *goja.Object {
	c := vm.NewObject()
	c.Set("signJWT", func(claims map[string]any, secret, algorithm, kid string) (string, error) {
		return engineSignJWT(claims, secret, algorithm, kid)
	})
	c.Set("md5", func(data string) string { return cryptoMD5(data) })
	c.Set("sha1", func(data string) string { return cryptoSHA1(data) })
	c.Set("sha256", func(data string) string { return cryptoSHA256(data) })
	c.Set("hmacSha1", func(key, data string) string { return cryptoHMAC1(key, data) })
	c.Set("hmacSha256", func(key, data string) string { return cryptoHMAC256(key, data) })
	c.Set("ed25519Sign", func(priv, data string) (string, error) { return edgeSign(priv, data) })
	c.Set("generateJWT", func(claims map[string]any, priv, kid string) (string, error) {
		return edgeJWT(priv, kid, claims)
	})
	return c
}

// consolePrint 控制台输出的汇总与路由（接入琉璃日志）。
func consolePrint(level string, args ...any) {
	parts := make([]string, 0, len(args))
	for _, a := range args {
		parts = append(parts, fmt.Sprint(a))
	}
	msg := strings.Join(parts, " ")
	switch level {
	case "warn":
		LoggerGeneral.Warn(ServiceName, "[console.warn] %s", msg)
	case "error":
		LoggerGeneral.Error(ServiceName, "[console.error] %s", msg)
	case "debug":
		LoggerGeneral.Info(ServiceName, "[console.debug] %s", msg)
	default:
		LoggerGeneral.Info(ServiceName, "[console.log] %s", msg)
	}
}

// bindConsole 注入沙箱全局 console（log/info/warn/error/debug），输出走琉璃日志。
func bindConsole(vm *goja.Runtime) *goja.Object {
	o := vm.NewObject()
	o.Set("log", func(args ...any) { consolePrint("log", args...) })
	o.Set("info", func(args ...any) { consolePrint("info", args...) })
	o.Set("warn", func(args ...any) { consolePrint("warn", args...) })
	o.Set("error", func(args ...any) { consolePrint("error", args...) })
	o.Set("debug", func(args ...any) { consolePrint("debug", args...) })
	return o
}

// bindImage engine.image.loadValid/download（allow-file；下载另需 allow-network）。
func bindImage(vm *goja.Runtime, p *plugin) *goja.Object {
	img := vm.NewObject()
	img.Set("loadValid", func(path string) (any, error) { return engineImageLoadValid(p, path) })
	img.Set("download", func(url, fileName string) (any, error) { return engineImageDownload(p, url, fileName) })
	return img
}

// bindLLM engine.llm.*（allow-agent，模型垄断）。chat/embed 与模型池查询统一落在 agent 字段模型。
func bindLLM(vm *goja.Runtime) *goja.Object {
	l := vm.NewObject()
	l.Set("chat", func(messages []any, opts map[string]any) (any, error) {
		if opts == nil {
			opts = map[string]any{}
		}
		return engineLLMChat(messages, opts)
	})
	l.Set("embed", func(input any, opts map[string]any) (any, error) {
		if opts == nil {
			opts = map[string]any{}
		}
		return engineLLMEmbed(input, opts)
	})

	// chatWithTools 走模型的函数调用（tools 透传）。
	l.Set("chatWithTools", func(opts map[string]any) (any, error) {
		if opts == nil {
			opts = map[string]any{}
		}
		messages, _ := opts["messages"].([]any)
		if len(messages) == 0 {
			return rwResult{Success: false, Error: "缺少 messages"}, nil
		}
		return engineLLMChat(messages, opts)
	})

	// chatWithConfig 等价 chat（模型垄断：模型参数一律忽略，强制 agent 模型）。
	l.Set("chatWithConfig", func(opts map[string]any) (any, error) {
		if opts == nil {
			opts = map[string]any{}
		}
		messages, _ := opts["messages"].([]any)
		if len(messages) == 0 {
			return rwResult{Success: false, Error: "缺少 messages"}, nil
		}
		return engineLLMChat(messages, opts)
	})

	l.Set("getConfig", func(taskType string) any { return poolGetConfig(taskType) })
	l.Set("getAllConfigs", func() any { return poolGetAllConfigs() })
	l.Set("listTasks", func() any { return poolListTasks() })
	l.Set("getAvailableModels", func() any { return poolGetAvailableModels() })
	l.Set("getCallRecords", func(n int64) any { return recentRecords(int(n)) })
	l.Set("getCircuitBreakers", func() any { return poolCircuitStatus() })
	return l
}

// bindTool engine.tool.register（allow-agent）。
func bindTool(vm *goja.Runtime, p *plugin) *goja.Object {
	t := vm.NewObject()
	t.Set("register", func(name string, definition goja.Value, handler goja.Value) (any, error) {
		if name == "" {
			return rwResult{Success: false, Error: "工具名不能为空"}, nil
		}
		if handler == nil || !isFunc(handler) {
			return rwResult{Success: false, Error: "handler 需为函数"}, nil
		}
		def := map[string]any{}
		if definition != nil && !goja.IsUndefined(definition) && !goja.IsNull(definition) {
			if exp := definition.Export(); exp != nil {
				if m, ok := exp.(map[string]any); ok {
					def = m
				}
			}
		}
		desc, _ := def["description"].(string)
		params, _ := def["parameters"].([]any)
		p.registerTool(&toolEntry{
			name:        name,
			description: desc,
			parameters:  params,
			handler:     handler,
		})
		return rwResult{Success: true}, nil
	})
	t.Set("getDefinitions", func() []any {
		p.mu.Lock()
		defer p.mu.Unlock()
		out := make([]any, 0, len(p.tools))
		for _, e := range p.tools {
			def := map[string]any{
				"name":        e.name,
				"description": e.description,
			}
			if e.parameters != nil {
				def["parameters"] = e.parameters
			}
			out = append(out, def)
		}
		return out
	})
	return t
}

// bindPlatform engine.platform（allow-send）。
func bindPlatform(vm *goja.Runtime) *goja.Object {
	pl := vm.NewObject()
	pl.Set("getName", func() (any, error) { return enginePlatform("getName", map[string]any{}) })
	pl.Set("getGroupId", func() (any, error) { return enginePlatform("getGroupId", map[string]any{}) })
	pl.Set("lookupUser", func(groupID, name string) (any, error) {
		return enginePlatform("lookupUser", map[string]any{"groupId": groupID, "name": name})
	})
	pl.Set("sendCommand", func(command string, args map[string]any) (any, error) {
		m := map[string]any{"command": command}
		if args != nil {
			m["args"] = args
		}
		return enginePlatform("sendCommand", m)
	})
	return pl
}

// bindEmoji engine.emoji（allow-memory）。内部复用记忆库 stickers 集合（image 型）。
func bindEmoji(vm *goja.Runtime) *goja.Object {
	e := vm.NewObject()
	e.Set("search", func(query string, opts map[string]any) (any, error) {
		limit := 5
		if opts != nil {
			if n, ok := opts["limit"].(int64); ok && n > 0 {
				limit = int(n)
			}
		}
		return engineEmojiSearch(query, limit)
	})
	e.Set("store", func(image string) (any, error) {
		return engineEmojiStore(image)
	})
	e.Set("random", func(query string) (any, error) {
		return engineEmojiRandom(query)
	})
	return e
}

// bindSend engine.send.text/image/hybrid（allow-send）。
func bindSend(vm *goja.Runtime, p *plugin) *goja.Object {
	s := vm.NewObject()
	pid := p.ID
	s.Set("text", func(target, text string) (any, error) { return engineSend(pid, "text", target, text) })
	s.Set("image", func(target, b64 string) (any, error) { return engineSend(pid, "image", target, b64) })
	s.Set("hybrid", func(target string, segs []any) (any, error) { return engineSend(pid, "hybrid", target, segs) })
	return s
}

// bindWs engine.ws.expose/publish（allow-socket；传输由宿主注入）。
func bindWs(vm *goja.Runtime, p *plugin) *goja.Object {
	w := vm.NewObject()
	w.Set("expose", func(path string, handler goja.Value) (any, error) {
		if handler == nil || !isFunc(handler) {
			return rwResult{Success: false, Error: "handler 需为函数"}, nil
		}
		return engineWsServe(p, path, handler)
	})
	w.Set("publish", func(data string) (any, error) { return engineWsPublish(p, data) })
	return w
}