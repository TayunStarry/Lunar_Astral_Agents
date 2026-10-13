package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"CorePersonality/protocol/ltpx"
	"LunarSubsystem/LoggerGeneral"

	"github.com/dop251/goja"
)

// LTPX 工具链协调（进程内实现）
//
// 合并为单进程后，工具提供方（琉璃）与工具消费方（月华）同处一个进程，因此：
//   1. 不再有联络 URL、心跳探测与「离线判死」：每次思考链起点直接读取进程内工具链
//   2. 不再有 HTTP 通道与结果轮询：等待经进程内结果通道完成（保留 request_id 语义，
//      因为前端扩展包运行在独立 WebView 进程中，其回执仍经 /ws → /ltpx/result 回到本进程）
//   3. 「xx事件发生前」推送改为进程内同步调用；无插件订阅/未改写/未回传时回退原始数据

// ltpCallMaxWait 单次工具调用的兜底等待上限（覆盖长时间运行的联网搜索等任务）
const ltpCallMaxWait = 30 * time.Minute

// syncLTPXRemoteTools 读取进程内工具链并写入缓存，返回聚合工具
func syncLTPXRemoteTools() *LTPXRemoteStatusResult {
	broker := ltpx.GetBroker()
	if broker == nil {
		return &LTPXRemoteStatusResult{}
	}

	status := broker.Status()
	result := &LTPXRemoteStatusResult{}
	if status.Tool == nil {
		ltpRemoteMutex.Lock()
		ltpRemoteTool = nil
		ltpRemoteMutex.Unlock()
		return result
	}

	tool := &LTPXRemoteToolDef{
		Name:        status.Tool.Name,
		Description: status.Tool.Description,
	}
	ltpRemoteMutex.Lock()
	ltpRemoteTool = tool
	ltpRemoteMutex.Unlock()

	result.Tool = tool
	// 聚合工具的 description 内嵌子工具清单，同步日志中一并披露，便于直观了解工具链规模
	names := ltpxSubToolNames(*tool)
	if len(names) > 0 {
		LoggerGeneral.Info("LunarCore", "已同步 LTPX 工具: \n%s", strings.Join(names, "\n"))
	}
	return result
}

// ltpxSubToolNames 从聚合工具的 description 中提取子工具名清单。
// buildUseTheProgram 以「- 工具名：功能简述」逐行罗列，此处按行取首个「：」前的名称。
func ltpxSubToolNames(tool LTPXRemoteToolDef) []string {
	var names []string
	for _, line := range strings.Split(tool.Description, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "- ") {
			continue
		}
		entry := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
		if idx := strings.Index(entry, "："); idx > 0 {
			entry = strings.TrimSpace(entry[:idx])
		}
		if entry != "" {
			names = append(names, entry)
		}
	}
	return names
}

// callLTPXRemoteTool 进程内调用工具并返回文本结果。
// 提交后经进程内结果通道等待执行完成（内置智能体直跑；前端包工具经集线器广播后等待回执）。
func callLTPXRemoteTool(toolName, argumentsJSON string) (string, error) {
	broker := ltpx.GetBroker()
	if broker == nil {
		return "", fmt.Errorf("LTPX 协调层未就绪，无法调用工具 %s", toolName)
	}

	// 解析 arguments（允许发来缺失/空对象）
	var args map[string]any
	if strings.TrimSpace(argumentsJSON) != "" && argumentsJSON != "{}" {
		if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
			LoggerGeneral.Warn("LunarCore", "LTPX 工具 %s 参数解析失败: %v", toolName, err)
			args = nil
		}
	}
	if args == nil {
		args = map[string]any{}
	}

	ctx, cancel := context.WithTimeout(context.Background(), ltpCallMaxWait)
	defer cancel()

	res, err := broker.Call(ctx, toolName, args)
	if err != nil {
		return "", err
	}
	if !res.Success {
		if res.Error != "" {
			return "", fmt.Errorf("工具 %s 执行失败: %s", toolName, res.Error)
		}
		return "", fmt.Errorf("工具 %s 执行失败", toolName)
	}
	return res.Text, nil
}

// getLTPXRemoteStatusForJS 供 JS 端在思考链起点调用的 Go 函数
// 读取进程内工具链，返回 JSON 字符串（含 tool；工具链为空时为 {}）
func (class *Runtime) getLTPXRemoteStatusForJS() goja.Value {
	result := syncLTPXRemoteTools()
	data, _ := json.Marshal(result)
	return class.runtime.ToValue(string(data))
}

// callLTPXRemoteToolForJS 供 JS 端工具函数异步转发调用的 Go 函数
// 参数：toolName, argumentsJSON；返回 Promise，异步 resolve 文本结果。
// 异步化设计：调用在独立 goroutine 中执行，完成后经 RunOnAgentLoop 回到 agent 事件循环
// resolve（goja Promise 非 goroutine-safe），因此长时间任务不会阻塞 agent 事件循环。
func (class *Runtime) callLTPXRemoteToolForJS(toolName string, argumentsJSON string) goja.Value {
	promise, resolve, _ := class.runtime.NewPromise()
	go func() {
		text, err := callLTPXRemoteTool(toolName, argumentsJSON)
		// 回到 eventloop 线程执行 resolve（goja Promise 必须在其创建线程上 resolve）
		RunOnAgentLoop(func(vm *goja.Runtime) {
			if err != nil {
				LoggerGeneral.Warn("LunarCore", "LTPX 工具调用失败: %v", err)
				resolve("【工具调用失败】" + err.Error())
				return
			}
			resolve(text)
		})
	}()
	// *goja.Promise 需经 ToValue 转换后才能作为 goja.Value 返回给 JS
	return class.runtime.ToValue(promise)
}

// clearLTPXRemoteToolsForJS 供 JS 端在工具链不可用时清空缓存的聚合工具
func (class *Runtime) clearLTPXRemoteToolsForJS() goja.Value {
	ltpRemoteMutex.Lock()
	ltpRemoteTool = nil
	ltpRemoteMutex.Unlock()
	return class.runtime.ToValue(true)
}

// syncLTPXRemoteEvent 进程内同步推送事件负载并接收插件处理结果（同步阻塞）。
// 事件点触发时调用：月华 JS 把「xx事件发生前」的原始 payload 推送到 LTP9 引擎派发面，
// 订阅该 topic 的插件可经 return 回传业务结果；无插件订阅/未回传时 returned=false，
// 调用方回退本地默认处理。
func syncLTPXRemoteEvent(topic, payloadJSON string) LTPXRemoteEventResult {
	broker := ltpx.GetBroker()
	if broker == nil {
		return LTPXRemoteEventResult{}
	}

	// 构造负载：优先解析为 JSON 对象，其次保留原始字符串
	var payload any = map[string]any{}
	if pj := strings.TrimSpace(payloadJSON); pj != "" && pj != "{}" {
		var obj map[string]any
		if err := json.Unmarshal([]byte(pj), &obj); err == nil {
			payload = obj
		} else {
			payload = pj
		}
	}

	res := broker.Event(topic, payload)
	return LTPXRemoteEventResult{
		Modified: res.Modified,
		Data:     res.Data,
		Returned: res.Returned,
		Return:   res.Return,
	}
}

// interactLTPXEventForJS 供 JS 端在每个事件触发点同步推送事件并接收插件处理结果。
// 参数：topic, payload；返回 JSON 字符串 {modified, data, returned, return}。
// 同步阻塞：LTP9 事件引擎为同步模型，等待插件处理完成。
func (class *Runtime) interactLTPXEventForJS(topic string, payload string) goja.Value {
	result := syncLTPXRemoteEvent(topic, payload)
	data, _ := json.Marshal(result)
	return class.runtime.ToValue(string(data))
}
