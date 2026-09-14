package StarLTP

// ==== 前端可视化测试探针接口（engine.* 能力复用，供 ltp9/test 信封调用） ====
// 每个 Probe* 函数都有对应当前引擎能力的真实 Go 实现（api.go / api_ext.go / api_net.go），
// 此处仅做统一包装与容错，不重写逻辑。文件类操作作用于独立探针数据目录，隔离于真实插件。

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
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

// ProbeLLMEmbed 文本向量化（engine.llm.embed 同源实现）。
func ProbeLLMEmbed(input any, opts map[string]any) map[string]any {
	return probeResult(engineLLMEmbed(input, opts))
}

// ProbeEmoji 表情包（engine.emoji 同源实现；作用于项目记忆库 stickers 集合）。op: search|store|random。
func ProbeEmoji(op string, params map[string]any) map[string]any {
	if params == nil {
		params = map[string]any{}
	}
	q, _ := params["query"].(string)
	switch op {
	case "store":
		img, _ := params["image"].(string)
		return probeResult(engineEmojiStore(img))
	case "random":
		return probeResult(engineEmojiRandom(q))
	default: // search
		limit := 5
		if n, ok := params["limit"].(int64); ok && n > 0 {
			limit = int(n)
		}
		return probeResult(engineEmojiSearch(q, limit))
	}
}

// ProbePlatform 平台上下文（engine.platform 同源实现）。method: getName|getGroupId|lookupUser。
func ProbePlatform(method string, params map[string]any) map[string]any {
	if params == nil {
		params = map[string]any{}
	}
	return probeResult(enginePlatform(method, params))
}

// ProbeTool 调用目标插件注册的工具（engine.tool 的 Go 侧入口，等价 star.CallTool 同源实现）。
func ProbeTool(pluginID, name string, params map[string]any) map[string]any {
	if params == nil {
		params = map[string]any{}
	}
	return probeResult(engineToolCall(pluginID, name, params))
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

// ==== 网络探针（engine.network 同源实现；套接字以句柄登记，供前端分步操作） ====
// resolveDNS/resolveSRV 即时返回；tcpConnect/udpConnect/udpListen 建立套接字并登记句柄；
// sockSend/sockReceive/sockSendTo/sockClose 按句柄操作既有套接字。

var (
	netProbeMu  sync.Mutex
	netProbeSeq int
	// netProbes 探针套接字句柄表：显式 sockClose 或超员淘汰时才释放，
	// 上限兜底防止前端反复 connect/listen 造成 fd 无限泄漏。
	netProbes = map[string]*netProbeEntry{}
)

// netProbeMax 句柄表上限，超过时关闭并淘汰最旧句柄。
const netProbeMax = 64

// netProbeEntry 句柄 → 套接字 + 登记时刻（淘汰最旧用）。
type netProbeEntry struct {
	sock    *netSock
	created time.Time
}

func netProbeArg(v map[string]any, k string) string { s, _ := v[k].(string); return s }
func netProbeNum(v map[string]any, k string) float64 { f, _ := v[k].(float64); return f }
func netProbeInt(v map[string]any, k string) int { return int(netProbeNum(v, k)) }
func netProbeSock(v map[string]any) (*netSock, string) {
	h := netProbeArg(v, "handle")
	netProbeMu.Lock()
	defer netProbeMu.Unlock()
	if e := netProbes[h]; e != nil {
		return e.sock, h
	}
	return nil, h
}
func netProbeRegister(s *netSock) map[string]any {
	netProbeMu.Lock()
	defer netProbeMu.Unlock()
	// 超员：关闭并淘汰最旧句柄
	for len(netProbes) >= netProbeMax {
		oldestKey := ""
		var oldest time.Time
		for h, e := range netProbes {
			if oldestKey == "" || e.created.Before(oldest) {
				oldestKey, oldest = h, e.created
			}
		}
		if oldestKey == "" {
			break
		}
		netProbes[oldestKey].sock.netSockClose()
		delete(netProbes, oldestKey)
	}
	netProbeSeq++
	h := "s" + fmt.Sprint(netProbeSeq)
	netProbes[h] = &netProbeEntry{sock: s, created: time.Now()}
	return map[string]any{"success": true, "handle": h}
}

// ProbeNetwork 网络探针分发。
func ProbeNetwork(op string, args map[string]any) map[string]any {
	if args == nil {
		args = map[string]any{}
	}
	switch op {
	case "resolveDNS":
		return probeResult(netResolveDNS(netProbeArg(args, "host"), netProbeNum(args, "timeout")), nil)
	case "resolveSRV":
		return probeResult(netResolveSRV(netProbeArg(args, "service"), netProbeArg(args, "proto"), netProbeArg(args, "host"), netProbeNum(args, "timeout")), nil)
	case "tcpConnect":
		s, err := netDialTCP(netProbeArg(args, "host"), netProbeInt(args, "port"), netProbeNum(args, "timeout"))
		if err != nil {
			return probeResult(nil, err)
		}
		return probeResult(netProbeRegister(s), nil)
	case "udpConnect":
		s, err := netDialUDP(netProbeArg(args, "host"), netProbeInt(args, "port"), netProbeNum(args, "timeout"))
		if err != nil {
			return probeResult(nil, err)
		}
		return probeResult(netProbeRegister(s), nil)
	case "udpListen":
		s, err := netListenUDP(netProbeArg(args, "host"), netProbeInt(args, "port"))
		if err != nil {
			return probeResult(nil, err)
		}
		return probeResult(netProbeRegister(s), nil)
	case "sockSend":
		s, h := netProbeSock(args)
		if s == nil {
			return probeResult(nil, fmt.Errorf("套接字 %q 不存在", h))
		}
		return probeResult(s.netSockSend(netProbeArg(args, "data")), nil)
	case "sockReceive":
		s, h := netProbeSock(args)
		if s == nil {
			return probeResult(nil, fmt.Errorf("套接字 %q 不存在", h))
		}
		return probeResult(s.netSockReceive(netProbeNum(args, "timeout")), nil)
	case "sockSendTo":
		s, h := netProbeSock(args)
		if s == nil {
			return probeResult(nil, fmt.Errorf("套接字 %q 不存在", h))
		}
		return probeResult(s.netSockSendTo(netProbeArg(args, "host"), netProbeInt(args, "port"), netProbeArg(args, "data")), nil)
	case "sockClose":
		s, h := netProbeSock(args)
		if s != nil {
			s.netSockClose()
		}
		netProbeMu.Lock()
		delete(netProbes, h)
		netProbeMu.Unlock()
		return probeResult(map[string]any{"success": true}, nil)
	}
	return probeResult(nil, fmt.Errorf("未知 network op: %s", op))
}

// ==== 异步子任务探针（engine.async 同源语义） ====
// 任务体二选一：
//   - 提供 plugin+fn：以目标插件导出的函数作为任务体，后台真实执行（引擎 async.run 语义）
//   - 否则：纯耗时模拟（ms 毫秒后置 done），便于前端验证 run/report/getStatus/list 生命周期
// params: { plugin?, fn?, args?, ms?, timeout?, task_id?, progress? }

var probeAsyncP = &plugin{ID: ProbePluginID, asyncTasks: map[int]*asyncTask{}}

// ProbeAsync 异步子任务探针分发。
func ProbeAsync(op string, params map[string]any) map[string]any {
	if params == nil {
		params = map[string]any{}
	}
	num := func(k string) float64 { f, _ := params[k].(float64); return f }
	taskID := int(num("task_id"))
	switch op {
	case "run":
		probeAsyncP.asyncMu.Lock()
		asyncPrune(probeAsyncP)
		probeAsyncP.asyncSeq++
		id := probeAsyncP.asyncSeq
		probeAsyncP.asyncTasks[id] = &asyncTask{id: id, status: "running", createdAt: time.Now()}
		probeAsyncP.asyncMu.Unlock()

		pluginID, _ := params["plugin"].(string)
		fnName, _ := params["fn"].(string)
		args, _ := params["args"].([]any)
		if args == nil {
			if s, ok := params["args"].(string); ok && s != "" {
				args = []any{s}
			}
		}

		go func() {
			var err error
			if pluginID != "" && fnName != "" {
				// 真实任务体：后台调用插件导出的函数（与引擎 async.run 一致，任务运行期间占用该插件事件循环）
				_, err = engineCall(pluginID, fnName, args)
			} else {
				// 无任务体：纯耗时模拟
				if ms := num("ms"); ms > 0 {
					time.Sleep(time.Duration(ms) * time.Millisecond)
				}
			}
			probeAsyncP.asyncMu.Lock()
			defer probeAsyncP.asyncMu.Unlock()
			t, ok := probeAsyncP.asyncTasks[id]
			if !ok {
				return
			}
			if err != nil {
				t.status = "error"
				t.progress = err.Error()
			} else if t.status == "running" {
				t.status = "done"
			}
		}()
		// 超时看门狗（仅标记状态）
		if t := num("timeout"); t > 0 {
			go func() {
				time.Sleep(time.Duration(t) * time.Millisecond)
				probeAsyncP.asyncMu.Lock()
				defer probeAsyncP.asyncMu.Unlock()
				if tt, ok := probeAsyncP.asyncTasks[id]; ok && tt.status == "running" {
					tt.status = "timeout"
				}
			}()
		}
		return probeResult(map[string]any{"success": true, "taskId": id}, nil)
	case "report":
		probeAsyncP.asyncMu.Lock()
		if t, ok := probeAsyncP.asyncTasks[taskID]; ok {
			t.progress = params["progress"]
		}
		probeAsyncP.asyncMu.Unlock()
		return probeResult(map[string]any{"success": true}, nil)
	case "status":
		probeAsyncP.asyncMu.Lock()
		defer probeAsyncP.asyncMu.Unlock()
		t, ok := probeAsyncP.asyncTasks[taskID]
		if !ok {
			return probeResult(nil, fmt.Errorf("任务 %d 不存在", taskID))
		}
		return probeResult(map[string]any{"success": true, "taskId": t.id, "status": t.status, "progress": t.progress}, nil)
	case "list":
		probeAsyncP.asyncMu.Lock()
		defer probeAsyncP.asyncMu.Unlock()
		asyncPrune(probeAsyncP)
		out := []any{}
		for _, t := range probeAsyncP.asyncTasks {
			out = append(out, map[string]any{"taskId": t.id, "status": t.status, "progress": t.progress})
		}
		return probeResult(out, nil)
	}
	return probeResult(nil, fmt.Errorf("未知 async op: %s", op))
}