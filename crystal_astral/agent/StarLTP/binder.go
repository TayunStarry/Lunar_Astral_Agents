package StarLTP

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
// 未纳入 granted 的能力对应方法呈 undefined，脚本顶层调用即触发 TypeError。
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

	// LLM 对话（allow-agent）
	if p.hasPerm("allow-agent") {
		engine.Set("llm", bindLLM(vm))
	}

	// Agent 工具注册（allow-agent：工具供 LLM tool_calls / AtoA 发现调用）
	if p.hasPerm("allow-agent") {
		engine.Set("tool", bindTool(vm, p))
	}

	// 平台上下文（allow-send：平台名/群 ID/用户解析，由宿主注入解析器）
	if p.hasPerm("allow-send") {
		engine.Set("platform", bindPlatform(vm))
	}

	// 表情包（allow-memory：复用项目记忆库 stickers 集合）
	if p.hasPerm("allow-memory") {
		engine.Set("emoji", bindEmoji(vm))
	}

	// 发送到会话（allow-send）
	if p.hasPerm("allow-send") {
		engine.Set("send", bindSend(vm))
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

// bindHTTP 同步阻塞的 HTTP 能力（engine.http.get/post/download），返回非 Promise 的普通对象。
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
	// 插件主动在事件总线上发布事件：派发给其它插件的订阅器 + 转发给宿主 outbound（供外部客户端消费）。
	// 异步分发（fire-and-forget），避免在被调用插件回调内等待触发插件自身回调造成自锁。
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

// bindTime engine.time.now()/nowMs()（可调用）。
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

// bindMemory engine.memory.store/search（allow-memory；记忆库为集合级管理，无按 id 单条读删）。
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

// bindConfig engine.config.getFile/setFile。
func bindConfig(vm *goja.Runtime, p *plugin) *goja.Object {
	c := vm.NewObject()
	c.Set("getFile", func() map[string]any { return p.config })
	c.Set("setFile", func(v map[string]any) {
		p.mu.Lock()
		p.config = v
		p.mu.Unlock()
		// 回写 config.yaml（引擎启动时注入的配置与磁盘保持同步）
		_ = os.WriteFile(p.ConfigPath, []byte(marshalYAML(v)), 0644)
		// 配置更新后触发 onConfigUpdate(scope, config, version) 生命周期回调
		p.fireConfigUpdate("plugin", v, "")
	})
	return c
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

// consolePrint 控制台输出的汇总与路由。
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

// bindConsole 注入沙箱全局 console（log/info/warn/error/debug），输出走引擎日志。
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

// bindLLM engine.llm.chat + embed（allow-agent）。
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
	return l
}

// bindTool engine.tool.register（allow-agent）。工具定义供 LLM/AtoA 发现调用，handler 返回业务结果。
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

// bindPlatform engine.platform（allow-send）。能力由宿主注入 platformResolver 解析。
func bindPlatform(vm *goja.Runtime) *goja.Object {
	pl := vm.NewObject()
	pl.Set("getName", func() (any, error) { return enginePlatform("getName", map[string]any{}) })
	pl.Set("getGroupId", func() (any, error) { return enginePlatform("getGroupId", map[string]any{}) })
	pl.Set("lookupUser", func(groupID, name string) (any, error) {
		return enginePlatform("lookupUser", map[string]any{"groupId": groupID, "name": name})
	})
	return pl
}

// bindEmoji engine.emoji（allow-memory）。内部复用项目记忆库 stickers 集合（image 型）。
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
func bindSend(vm *goja.Runtime) *goja.Object {
	s := vm.NewObject()
	s.Set("text", func(target, text string) (any, error) { return engineSend("text", target, text) })
	s.Set("image", func(target, b64 string) (any, error) { return engineSend("image", target, b64) })
	s.Set("hybrid", func(target string, segs []any) (any, error) { return engineSend("hybrid", target, segs) })
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
