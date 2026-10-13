package StarLTP

// ==== LTP9-Flash 顶层全局绑定（按 allow-* 权限注入插件沙箱） ====
// 契约见 docs/LTP 9.1 Flash.d.ts：能力以顶层全局（signal/file/memory/http/...）注入，
// 未获权限的全局不注入，脚本调用即触发 ReferenceError；调用被拒时返回带 error 的结果对象。

import (
	"fmt"
	"os"
	"strings"
	"time"

	"LunarSubsystem/LoggerGeneral"
	"github.com/dop251/goja"
)

// bindSandbox 把 LTP9-Flash 顶层全局注册进插件沙箱。
// 定时器（setTimeout/setInterval 等）由 eventloop 自动注入，无需在此绑定。
func bindSandbox(vm *goja.Runtime, p *plugin) {
	vm.Set("console", bindConsole(vm))

	// 常驻基础能力（不参与 allow-* 开关）
	vm.Set("event", bindEvent(vm, p)) // 事件订阅器
	vm.Set("time", bindTime(vm))      // 时间戳
	vm.Set("sleep", func(ms float64) { time.Sleep(time.Duration(ms) * time.Millisecond) })
	vm.Set("exportFunction", func(name string, fn goja.Value) { // 导出插件能力（供 callFunction）
		if fn == nil || !isFunc(fn) {
			return
		}
		p.mu.Lock()
		p.exports[name] = fn
		p.mu.Unlock()
	})
	vm.Set("config", bindConfig(vm, p))   // 插件配置
	vm.Set("command", bindCommand(vm, p)) // 指令系统
	vm.Set("async", bindAsync(vm, p))     // 异步子任务

	// 广播收发（allow-signal）
	if p.hasPerm("allow-signal") {
		vm.Set("signal", bindSignal(vm, p))
	}
	// 文件 + 图像 + 视频（allow-file；image.download / video URL 源运行时另需 allow-network）
	if p.hasPerm("allow-file") {
		vm.Set("file", bindFile(vm, p))
		vm.Set("image", bindImage(vm, p))
		vm.Set("video", bindVideo(vm, p))
	}
	// 记忆库（allow-memory）
	if p.hasPerm("allow-memory") {
		vm.Set("memory", bindMemory(vm))
	}
	// 数据库（allow-database）
	if p.hasPerm("allow-database") {
		vm.Set("database", bindDatabase(vm))
	}
	// 编解码 + 哈希签名（allow-certificate）
	if p.hasPerm("allow-certificate") {
		vm.Set("encoding", bindEncoding(vm))
		vm.Set("hash", bindHash(vm))
	}
	// 跨包调用（allow-call）：Result 统一风格，目标不存在/未导出不抛异常
	if p.hasPerm("allow-call") {
		vm.Set("callFunction", func(pkgID, fnName string, args []any) map[string]any {
			v, err := engineCall(pkgID, fnName, args)
			if err != nil {
				return map[string]any{"success": false, "error": err.Error()}
			}
			return map[string]any{"success": true, "result": v}
		})
	}
	// 智能体（allow-agent）：LLM 对话 / 文本嵌入 / 前端智能体 / Web-LTP 搜索（search 运行时另需 allow-network）
	if p.hasPerm("allow-agent") {
		vm.Set("agent", bindAgent(vm, p))
	}
	// 网络（allow-network）：同步 http + 裸套接字 + 全局 fetch/WebSocket
	if p.hasPerm("allow-network") {
		vm.Set("http", bindHTTP(vm, p))
		vm.Set("network", bindNet(vm))
		bindNetwork(vm, p)
	}
}

// bindHTTP 同步阻塞的 HTTP 能力（http.get/post/download），返回非 Promise 的普通对象。
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

// bindEvent event.subscribe(topic, handler, priority?)/unsubscribe/publish（常驻注入）。
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

// bindSignal signal.all/target/subscribe/unsubscribe（allow-signal）。
// all/target 单向广播；subscribe/unsubscribe 注册/退订广播接收回调。
func bindSignal(vm *goja.Runtime, p *plugin) *goja.Object {
	sig := vm.NewObject()
	sig.Set("all", func(payload any) { engineBroadcast("__all__", payload) })
	sig.Set("target", func(id string, payload any) { engineBroadcast(id, payload) })
	sig.Set("subscribe", func(cb goja.Value) int {
		if cb == nil || !isFunc(cb) {
			return 0
		}
		p.mu.Lock()
		p.subSeq++
		id := p.subSeq
		p.signalSubs = append(p.signalSubs, &eventSub{orderID: id, handler: cb})
		p.mu.Unlock()
		return id
	})
	sig.Set("unsubscribe", func(id int) {
		p.mu.Lock()
		kept := p.signalSubs[:0]
		for _, s := range p.signalSubs {
			if s.orderID != id {
				kept = append(kept, s)
			}
		}
		p.signalSubs = kept
		p.mu.Unlock()
	})
	return sig
}

// bindTime time.now()/nowMs()。
func bindTime(vm *goja.Runtime) *goja.Object {
	t := vm.NewObject()
	t.Set("now", func() int64 { return time.Now().Unix() })
	t.Set("nowMs", func() int64 { return time.Now().UnixMilli() })
	return t
}

// bindFile file.write/read/delete（allow-file；路径限定插件数据目录）。
func bindFile(vm *goja.Runtime, p *plugin) *goja.Object {
	f := vm.NewObject()
	f.Set("write", func(path, data string) (any, error) { return fileWrite(p, path, data) })
	f.Set("read", func(path string) (any, error) { return fileRead(p, path) })
	f.Set("delete", func(path string) (any, error) { return fileDelete(p, path) })
	return f
}

// bindMemory memory.store/search/searchImage/storeImage/randomImage（allow-memory）。
// 文本记忆为集合级管理，无按 id 单条读删；图片方法复用项目记忆库 stickers 集合。
func bindMemory(vm *goja.Runtime) *goja.Object {
	m := vm.NewObject()
	m.Set("store", func(v map[string]any) (any, error) { return memoryStore(v) })
	m.Set("search", func(v map[string]any) (any, error) { return memorySearch(v) })
	m.Set("searchImage", func(query string, opts map[string]any) (any, error) {
		limit := 5
		if opts != nil {
			if n, ok := opts["limit"].(int64); ok && n > 0 {
				limit = int(n)
			}
		}
		return memorySearchImage(query, limit)
	})
	m.Set("storeImage", func(image string) (any, error) { return memoryStoreImage(image) })
	m.Set("randomImage", func(query string) (any, error) { return memoryRandomImage(query) })
	return m
}

// bindDatabase database.query/exec/transaction/migrate/namespace（allow-database）。
// transaction/migrate 操作共享库 knowledge.db；namespace(name) 返回独立 <name>.db 的作用域对象。
func bindDatabase(vm *goja.Runtime) *goja.Object {
	d := vm.NewObject()
	d.Set("query", func(sql string, params []any) (any, error) { return databaseQuery(sql, params) })
	d.Set("exec", func(sql string, params []any) (any, error) { return databaseExec(sql, params) })
	// 同步事务：fn 接收 {query, exec}（绑定到事务）；正常返回提交，fn 抛错/返回 Promise 回滚
	d.Set("transaction", func(fn goja.Value) (any, error) { return databaseTransaction(vm, fn) })
	// 按名称迁移：未应用则在事务内执行 upSql（支持多语句）并记录到 _migrations
	d.Set("migrate", func(name, upSQL string) (any, error) {
		db, err := ensureDB()
		if err != nil {
			return rwResult{Success: false, Error: err.Error()}, nil
		}
		return dbMigrateOn(db, name, upSQL)
	})
	// 命名空间作用域：独立 <name>.db 文件，返回 {query, exec, transaction, migrate}
	d.Set("namespace", func(name string) (any, error) { return databaseNamespace(vm, name) })
	return d
}

// bindConfig config.read/write（常驻）。read 返回 { success, config? }（无配置时省略 config）；
// write 同步写回 config.yaml。
func bindConfig(vm *goja.Runtime, p *plugin) *goja.Object {
	c := vm.NewObject()
	c.Set("read", func() map[string]any {
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.config == nil {
			return map[string]any{"success": true}
		}
		return map[string]any{"success": true, "config": p.config}
	})
	c.Set("write", func(v map[string]any) {
		p.mu.Lock()
		p.config = v
		p.mu.Unlock()
		// 回写 config.yaml（引擎启动时注入的配置与磁盘保持同步）
		_ = os.WriteFile(p.ConfigPath, []byte(marshalYAML(v)), 0644)
	})
	return c
}

// bindHash hash.*（allow-certificate）：摘要/HMAC/Ed25519/JWT 签名。
func bindHash(vm *goja.Runtime) *goja.Object {
	h := vm.NewObject()
	// 契约（Result 统一风格）：签名失败返回 { success:false, error }，不再抛异常
	h.Set("signJWT", func(claims map[string]any, secret, algorithm, kid string) map[string]any {
		s, err := engineSignJWT(claims, secret, algorithm, kid)
		if err != nil {
			return map[string]any{"success": false, "error": err.Error()}
		}
		return map[string]any{"success": true, "text": s}
	})
	h.Set("md5", func(data string) string { return cryptoMD5(data) })
	h.Set("sha1", func(data string) string { return cryptoSHA1(data) })
	h.Set("sha256", func(data string) string { return cryptoSHA256(data) })
	h.Set("hmacSha1", func(key, data string) string { return cryptoHMAC1(key, data) })
	h.Set("hmacSha256", func(key, data string) string { return cryptoHMAC256(key, data) })
	h.Set("ed25519Sign", func(priv, data string) map[string]any {
		s, err := edgeSign(priv, data)
		if err != nil {
			return map[string]any{"success": false, "error": err.Error()}
		}
		return map[string]any{"success": true, "text": s}
	})
	h.Set("generateJWT", func(claims map[string]any, priv, kid string) map[string]any {
		s, err := edgeJWT(priv, kid, claims)
		if err != nil {
			return map[string]any{"success": false, "error": err.Error()}
		}
		return map[string]any{"success": true, "text": s}
	})
	return h
}

// bindImage image.loadValid/download/resize/convert（allow-file；download 运行时另需 allow-network）。
func bindImage(vm *goja.Runtime, p *plugin) *goja.Object {
	img := vm.NewObject()
	img.Set("loadValid", func(path string) (any, error) { return engineImageLoadValid(p, path) })
	img.Set("download", func(url, fileName string) (any, error) { return engineImageDownload(p, url, fileName) })
	// resize 缩放（max_dim 等比 / width+height 精确）+ 可选编码格式；convert 仅重编码（编码格式化）
	img.Set("resize", func(path string, opts map[string]any) (any, error) {
		return engineImageResize(p, path, opts)
	})
	img.Set("convert", func(path, format string, opts map[string]any) (any, error) {
		return engineImageConvert(p, path, format, opts)
	})
	return img
}

// bindVideo video.frames：从视频抽帧（allow-file；URL/dataURI 源运行时另需 allow-network）。
func bindVideo(vm *goja.Runtime, p *plugin) *goja.Object {
	v := vm.NewObject()
	v.Set("frames", func(source string, opts map[string]any) map[string]any {
		return engineVideoFrames(p, source, opts)
	})
	return v
}

// bindAgent agent.chat/embed/synergy/search（allow-agent；search 运行时另需 allow-network）。
func bindAgent(vm *goja.Runtime, p *plugin) *goja.Object {
	a := vm.NewObject()
	a.Set("chat", func(messages []any, opts map[string]any) (any, error) {
		if opts == nil {
			opts = map[string]any{}
		}
		return engineLLMChat(messages, opts)
	})
	a.Set("embed", func(input any, opts map[string]any) (any, error) {
		if opts == nil {
			opts = map[string]any{}
		}
		return engineLLMEmbed(input, opts)
	})
	// synergy 调用前端智能体（Mini-LTP / Node-LTP），同步阻塞返回其执行文本；
	// Result 统一风格：目标包不存在/拒绝响应返回 { success:false, error }，不抛异常
	a.Set("synergy", func(pkgID, text string) map[string]any {
		v, err := engineAgent(pkgID, text)
		if err != nil {
			return map[string]any{"success": false, "error": err.Error()}
		}
		return map[string]any{"success": true, "text": fmt.Sprint(v)}
	})
	// search 调用进程内 Web-LTP 网络搜索（双权限：allow-agent 绑定 + allow-network 执行）
	a.Set("search", func(instruction string) map[string]any {
		return engineAgentSearch(p, instruction)
	})
	return a
}

// sprintArgs 把任意个控制台实参拼接为单行文本（空格分隔）。
func sprintArgs(args ...any) string {
	parts := make([]string, 0, len(args))
	for _, a := range args {
		parts = append(parts, fmt.Sprint(a))
	}
	return strings.Join(parts, " ")
}

// consolePrint 控制台输出的汇总与路由。
func consolePrint(level string, args ...any) {
	msg := sprintArgs(args...)
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
