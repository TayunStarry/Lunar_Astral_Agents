package runtime

// ==== 运行时沙箱顶层全局绑定（能力全量注入，无权限开关） ====
// 契约见 prompts/runtimeAPI/runtimeAPI.d.ts 及按能力拆分的 prompts/runtimeAPI/*API.d.ts。

import (
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"github.com/dop251/goja"
)

// bindSandbox 把沙箱顶层全局注册进私有运行时。
// 定时器（setTimeout/setInterval 等）由 eventloop 自动注入，无需在此绑定。
func bindSandbox(vm *goja.Runtime, p *plugin) {
	vm.Set("console", bindConsole(vm))

	// 导出能力（供宿主 Runtime.Call）
	vm.Set("exportFunction", func(name string, fn goja.Value) {
		if fn == nil || !isFunc(fn) {
			return
		}
		p.mu.Lock()
		p.exports[name] = fn
		p.mu.Unlock()
	})

	// 文件 + 图像 + 视频
	vm.Set("file", bindFile(vm, p))
	vm.Set("image", bindImage(vm, p))
	vm.Set("video", bindVideo(vm, p))
	// 记忆库
	vm.Set("memory", bindMemory(vm))
	// 数据库
	vm.Set("database", bindDatabase(vm))
	// 编解码 + 哈希签名
	vm.Set("encoding", bindEncoding(vm))
	vm.Set("hash", bindHash(vm))
	// 智能体：LLM 对话 / 网络搜索
	vm.Set("agent", bindAgent(vm, p))
	// 网络：文件下载 + 全局 fetch/WebSocket（通用请求一律用 fetch）
	vm.Set("http", bindHTTP(vm, p))
	bindNetwork(vm, p)
}

// bindHTTP 同步下载（http.download）：把远端文件存进脚本数据目录。
func bindHTTP(vm *goja.Runtime, p *plugin) *goja.Object {
	h := vm.NewObject()
	h.Set("download", func(url, savePath string) map[string]any { return httpDownloadSync(p, url, savePath) })
	return h
}

// bindFile file.write/read/delete（路径限定运行时数据目录）。
func bindFile(vm *goja.Runtime, p *plugin) *goja.Object {
	f := vm.NewObject()
	f.Set("write", func(path, data string) (any, error) { return fileWrite(p, path, data) })
	f.Set("read", func(path string) (any, error) { return fileRead(p, path) })
	f.Set("delete", func(path string) (any, error) { return fileDelete(p, path) })
	return f
}

// bindMemory memory.store/search/searchImage/storeImage/randomImage。
// 文本与图片记忆都按类型全局检索，不暴露集合名；集合只是内容自动归入的分组。
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

// bindDatabase database.query/exec/transaction/migrate。
// 全部操作作用于单一共享库 knowledge.db，不提供新建库文件能力。
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
	return d
}

// bindHash hash.*：摘要/HMAC/Ed25519/JWT 签名。
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

// bindImage image.loadValid/resize/convert。
func bindImage(vm *goja.Runtime, p *plugin) *goja.Object {
	img := vm.NewObject()
	img.Set("loadValid", func(path string) (any, error) { return engineImageLoadValid(p, path) })
	// resize 缩放（max_dim 等比 / width+height 精确）+ 可选编码格式；convert 仅重编码（编码格式化）
	img.Set("resize", func(path string, opts map[string]any) (any, error) {
		return engineImageResize(p, path, opts)
	})
	img.Set("convert", func(path, format string, opts map[string]any) (any, error) {
		return engineImageConvert(p, path, format, opts)
	})
	return img
}

// bindVideo video.frames：从视频抽帧。
func bindVideo(vm *goja.Runtime, p *plugin) *goja.Object {
	v := vm.NewObject()
	v.Set("frames", func(source string, opts map[string]any) map[string]any {
		return engineVideoFrames(p, source, opts)
	})
	return v
}

// bindAgent agent.chat/search。
func bindAgent(vm *goja.Runtime, p *plugin) *goja.Object {
	a := vm.NewObject()
	a.Set("chat", func(messages []any, opts map[string]any) (any, error) {
		if opts == nil {
			opts = map[string]any{}
		}
		return engineLLMChat(messages, opts)
	})
	// search 调用进程内 Web-LTP 网络搜索
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
		logger.Warn(ServiceName, "[console.warn] %s", msg)
	case "error":
		logger.Error(ServiceName, "[console.error] %s", msg)
	case "debug":
		logger.Info(ServiceName, "[console.debug] %s", msg)
	default:
		logger.Info(ServiceName, "[console.log] %s", msg)
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
