package StarLTP

// ==== 前端可视化测试探针接口（engine.* 能力复用，供 ltp9/test 信封调用） ====
// 每个 Probe* 函数都有对应当前引擎能力的真实 Go 实现（api.go / api_ext.go / api_net.go），
// 此处仅做统一包装与容错，不重写逻辑。文件类操作作用于独立探针数据目录，隔离于真实插件。

import (
	"os"
	"path/filepath"
	"time"

	"LunarSubsystem/GeneralConfig"
)

// probeDataDir 探针文件区根目录：<execDir>/<LocalDir>/database/ltp9_probe
// 与项目数据根目录（local_data/database/）保持一致，隔离于真实插件。
func probeDataDir() string {
	execPath, err := os.Executable()
	if err != nil {
		return filepath.Join("local_data", "database", ProbeDataDirName)
	}
	return filepath.Join(filepath.Dir(execPath), *GeneralConfig.LocalDir, "database", ProbeDataDirName)
}

// probePlugin 构造虚拟探针插件，其 DataDir 指向探针文件区（供 fileWrite/fileRead/fileDelete 的 scopedPath 使用）。
func probePlugin() *plugin {
	return &plugin{ID: ProbePluginID, DataDir: probeDataDir()}
}

// probeResult 把 (any, error) 归一为 map 返回。
func probeResult(v any, err error) map[string]any {
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}
	}
	return map[string]any{"ok": true, "value": v}
}

// ProbeFile 文件读写删（作用于探针数据目录）。
func ProbeFile(op, path, data string) map[string]any {
	p := probePlugin()
	switch op {
	case "read":
		if r, err := fileRead(p, path); err != nil {
			return probeResult(nil, err)
		} else {
			return probeResult(r, nil)
		}
	case "delete":
		return probeResult(fileDelete(p, path))
	default: // write
		return probeResult(fileWrite(p, path, data))
	}
}

// ProbeDB 数据库读写。op: query|exec。
func ProbeDB(op, sql string, params []any) map[string]any {
	switch op {
	case "exec":
		return probeResult(databaseExec(sql, params))
	default: // query
		return probeResult(databaseQuery(sql, params))
	}
}

// ProbeMemory 记忆库读写。op: store|search。
func ProbeMemory(op string, v map[string]any) map[string]any {
	switch op {
	case "search":
		return probeResult(memorySearch(v))
	default: // store
		return probeResult(memoryStore(v))
	}
}

// ProbeEncode 加密（engine.encoder 同源实现）。
func ProbeEncode(key, content string) map[string]any {
	return probeResult(engineEncode(key, content))
}

// ProbeDecode 解密（engine.decoder 同源实现）。
func ProbeDecode(key, cipher string) map[string]any {
	return probeResult(engineDecode(key, cipher))
}

// ProbeJWT JWT 签名（engine.crypto.signJWT 同源实现）。
func ProbeJWT(claims map[string]any, secret, algorithm, kid string) map[string]any {
	return probeResult(engineSignJWT(claims, secret, algorithm, kid))
}

// ProbeLLM LLM 对话（engine.llm.chat 同源实现）。
func ProbeLLM(messages []any, opts map[string]any) map[string]any {
	return probeResult(engineLLMChat(messages, opts))
}

// ProbeHTTP 同步 HTTP（engine.http.get/post 同源实现）。method: GET|POST。
func ProbeHTTP(method, url, body string, headers map[string]any) map[string]any {
	if method == "POST" {
		return map[string]any{"ok": true, "value": httpPostSync(url, body, headers)}
	}
	return map[string]any{"ok": true, "value": httpGetSync(url, headers)}
}

// ProbeSleep 同步等待（engine.sleep 同源语义），阻塞指定毫秒后返回。
func ProbeSleep(ms float64) map[string]any {
	if ms > 0 {
		time.Sleep(time.Duration(ms) * time.Millisecond)
	}
	return map[string]any{"ok": true, "value": map[string]any{"slept_ms": ms}}
}