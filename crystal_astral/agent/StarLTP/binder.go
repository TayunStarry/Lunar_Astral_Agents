package StarLTP

// ==== engine.* 全局对象绑定（按 allow-* 权限注入） ====

import (
	"os"
	"time"

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
		engine.Set("http", bindHTTP(vm))
	}
	engine.Set("sleep", func(ms float64) { time.Sleep(time.Duration(ms) * time.Millisecond) })

	return engine
}

// bindHTTP 同步阻塞的 HTTP 能力（engine.http.get/post），返回非 Promise 的普通对象。
func bindHTTP(vm *goja.Runtime) *goja.Object {
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
	})
	return c
}

// bindCrypto engine.crypto.signJWT（allow-certificate）。
func bindCrypto(vm *goja.Runtime) *goja.Object {
	c := vm.NewObject()
	c.Set("signJWT", func(claims map[string]any, secret, algorithm, kid string) (string, error) {
		return engineSignJWT(claims, secret, algorithm, kid)
	})
	return c
}

// bindImage engine.image.loadValid/download（allow-file；下载另需 allow-network）。
func bindImage(vm *goja.Runtime, p *plugin) *goja.Object {
	img := vm.NewObject()
	img.Set("loadValid", func(path string) (any, error) { return engineImageLoadValid(p, path) })
	img.Set("download", func(url, fileName string) (any, error) { return engineImageDownload(p, url, fileName) })
	return img
}

// bindLLM engine.llm.chat(messages, opts)（allow-agent）。
func bindLLM(vm *goja.Runtime) *goja.Object {
	l := vm.NewObject()
	l.Set("chat", func(messages []any, opts map[string]any) (any, error) {
		if opts == nil {
			opts = map[string]any{}
		}
		return engineLLMChat(messages, opts)
	})
	return l
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
