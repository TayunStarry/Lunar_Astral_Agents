package lunarloop

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/protocol/ltpx"

	"github.com/dop251/goja"
)

// LTPX 工具链协调（进程内实现）
//
// 合并为单进程后，工具提供方（琉璃）与工具消费方（月华）同处一个进程，因此：
//   1. 不再有联络 URL、心跳探测与「离线判死」：每次思考链起点直接读取进程内工具链
//   2. 不再有 HTTP 通道与结果轮询：等待经进程内结果通道完成（保留 request_id 语义，
//      因为前端扩展包运行在独立 WebView 进程中，其回执仍经 /ws → /ltpx/result 回到本进程）
//   3. LTP9 事件全部改由 Go 边界触发（见 ltp_events.go），goja 不再发起或处理事件

// ltpCallMaxWait 单次工具调用的兜底等待上限（覆盖长时间运行的联网搜索等任务）
const ltpCallMaxWait = 30 * time.Minute

// syncLTPXRemoteTools 读取进程内工具链并写入缓存，返回聚合工具。
// 工具链未就绪（宿主未初始化、聚合工具缺失）时保留上次缓存定义——单进程下工具链
// 只在真正不可用时为空，短暂缺失不应让月华丢掉工具定义。
func syncLTPXRemoteTools() *LTPXRemoteStatusResult {
	broker := ltpx.GetBroker()
	if broker == nil {
		return &LTPXRemoteStatusResult{}
	}

	status := broker.Status()
	if status.Tool == nil || status.Tool.Name == "" {
		// 保留缓存定义，仅返回空结果（JS 侧据 tool 缺省维持上次定义）
		return &LTPXRemoteStatusResult{}
	}

	tool := &LTPXRemoteToolDef{
		Name:        status.Tool.Name,
		Description: status.Tool.Description,
		// 参数 schema 透传至 JS 侧（缺失时 JS 按 {details, tool, instruction} 兜底），
		// 保证发给模型的函数定义带有完整参数说明
		Parameters: status.Tool.Parameters,
	}
	ltpRemoteMutex.Lock()
	ltpRemoteTool = tool
	ltpRemoteMutex.Unlock()

	// 应用清单：工具名写进枚举，描述供 details 回查
	tools := make([]LTPXRemoteToolRef, 0, len(status.Tools))
	names := make([]string, 0, len(status.Tools))
	for _, t := range status.Tools {
		if t.Name == "" {
			continue
		}
		tools = append(tools, LTPXRemoteToolRef{Name: t.Name, Description: t.Description})
		names = append(names, t.Name)
	}

	// 应用清单变化时打日志（避免每轮重复刷屏）
	if len(names) > 0 && !equalStrings(names, ltpLoggedSubTools) {
		ltpLoggedSubTools = names
		logger.Info("LunarCore", "已同步 LTPX 工具: \n%s", strings.Join(names, "\n"))
	}
	return &LTPXRemoteStatusResult{Tool: tool, Tools: tools}
}

// equalStrings 比较两个字符串切片是否完全相同（工具清单日志去重）
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
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
			logger.Warn("LunarCore", "LTPX 工具 %s 参数解析失败: %v", toolName, err)
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
		// 失败但带部分进展/结果文本时一并回传：内置智能体达到轮次上限等情况正是
		// "失败 + 已完成内容"，丢弃文本会让模型完全看不到已完成的进展
		detail := strings.TrimSpace(res.Text)
		switch {
		case res.Error != "" && detail != "":
			return "", fmt.Errorf("工具 %s 执行失败: %s；已完成部分：%s", toolName, res.Error, detail)
		case res.Error != "":
			return "", fmt.Errorf("工具 %s 执行失败: %s", toolName, res.Error)
		case detail != "":
			return "", fmt.Errorf("工具 %s 未成功完成；已完成部分：%s", toolName, detail)
		default:
			return "", fmt.Errorf("工具 %s 执行失败", toolName)
		}
	}
	if strings.TrimSpace(res.Text) == "" {
		return fmt.Sprintf("工具 %s 已执行完成（无文本结果）", toolName), nil
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
				logger.Warn("LunarCore", "LTPX 工具调用失败: %v", err)
				resolve("【工具调用失败】" + err.Error())
				return
			}
			resolve(text)
		})
	}()
	// *goja.Promise 需经 ToValue 转换后才能作为 goja.Value 返回给 JS
	return class.runtime.ToValue(promise)
}
