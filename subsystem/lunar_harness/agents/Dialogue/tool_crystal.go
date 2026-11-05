package Dialogue

// tool_crystal.go use_the_program 扩展应用工具：声明与执行都在智能体侧。
//
// 声明是动态枚举：枚举值来自 LTPX 协调层的运行时应用清单，
// 经 toolreg.RegisterProvider 在查询时重新计算，避免 init 时把状态固化。
// 执行仅经 protocol/ltpx 进程内协调层，不依赖任何链包。

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"LunarSubsystem/LunarHarness/agents/Responder"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/protocol/agentdispatch"
	"LunarSubsystem/LunarHarness/protocol/ltpx"
	"LunarSubsystem/LunarHarness/protocol/toolreg"
)

const (
	// ToolUseTheProgram 扩展应用聚合工具名
	ToolUseTheProgram = "use_the_program"
)

// remoteToolRef 可调用应用（名称 + 能力描述）
type remoteToolRef struct {
	Name        string
	Description string
}

var (
	// remoteMu 保护聚合工具缓存
	remoteMu sync.RWMutex
	// remoteTools 可调用应用清单
	remoteTools []remoteToolRef
	// remoteLoggedNames 上次打日志的应用名清单
	remoteLoggedNames []string
)

// SyncLTPXRemoteTools 读取进程内工具链并写入应用清单缓存；返回是否同步成功
func SyncLTPXRemoteTools() bool {
	broker := ltpx.GetBroker()
	if broker == nil {
		return false
	}
	status := broker.Status()
	if status.Tool == nil || status.Tool.Name == "" {
		return false
	}
	tools := make([]remoteToolRef, 0, len(status.Tools))
	names := make([]string, 0, len(status.Tools))
	for _, item := range status.Tools {
		if item.Name == "" {
			continue
		}
		tools = append(tools, remoteToolRef{Name: item.Name, Description: item.Description})
		names = append(names, item.Name)
	}
	remoteMu.Lock()
	remoteTools = tools
	if len(names) > 0 && !equalStrings(names, remoteLoggedNames) {
		remoteLoggedNames = names
		logger.Info(LoggerModule, "已同步 LTPX 工具: \n%s", strings.Join(names, "\n"))
	}
	remoteMu.Unlock()
	return true
}

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

// crystalProgramNames 取当前应用名清单
func crystalProgramNames() []string {
	remoteMu.RLock()
	defer remoteMu.RUnlock()
	names := make([]string, 0, len(remoteTools))
	for _, item := range remoteTools {
		names = append(names, item.Name)
	}
	return names
}

// crystalRemoteSchemas 读取琉璃聚合工具声明（动态枚举）。
func crystalRemoteSchemas() []toolreg.Schema {
	remoteMu.RLock()
	empty := len(remoteTools) == 0
	remoteMu.RUnlock()
	if empty {
		SyncLTPXRemoteTools()
	}
	names := crystalProgramNames()
	branchDesc := "要使用的扩展应用名，只能取枚举值；只填本项表示打开该应用"
	if len(names) == 0 {
		branchDesc = "要使用的扩展应用名（当前没有可用应用）"
	}
	return []toolreg.Schema{{
		Name:        ToolUseTheProgram,
		Description: "调用星月智能的扩展应用。",
		Parameters: agentdispatch.Schema(
			branchDesc,
			names,
			"打开应用后交给它执行的具体任务；只打开应用时省略",
			"该扩展应用需要的目标句柄（如 @D.012、@I.007）。",
		),
	}}
}

// queryCrystalProgramDetails 按应用名返回职能范围描述。
func queryCrystalProgramDetails(name string) (string, bool) {
	remoteMu.RLock()
	var found *remoteToolRef
	for i := range remoteTools {
		if remoteTools[i].Name == name {
			found = &remoteToolRef{Name: remoteTools[i].Name, Description: remoteTools[i].Description}
			break
		}
	}
	remoteMu.RUnlock()
	if found == nil {
		SyncLTPXRemoteTools()
		remoteMu.RLock()
		for i := range remoteTools {
			if remoteTools[i].Name == name {
				found = &remoteToolRef{Name: remoteTools[i].Name, Description: remoteTools[i].Description}
				break
			}
		}
		remoteMu.RUnlock()
	}
	if found == nil {
		return "未找到应用 " + name + "；当前可用应用：" + strings.Join(crystalProgramNames(), "、") + "。", false
	}
	description := found.Description
	if description == "" {
		description = "（该应用未提供说明）"
	}
	return "【" + found.Name + "】" + description + "\n\n用法：只打开该应用传 type=" + found.Name + "；要它执行任务再填 instruction。", true
}

// callLTPXRemoteTool 进程内调用工具并返回文本结果
func callLTPXRemoteTool(toolName, argumentsJSON string) (string, error) {
	broker := ltpx.GetBroker()
	if broker == nil {
		return "", &ltpxUnavailableError{tool: toolName}
	}
	var args map[string]any
	if strings.TrimSpace(argumentsJSON) != "" && argumentsJSON != "{}" {
		if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
			args = nil
		}
	}
	if args == nil {
		args = map[string]any{}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	result, err := broker.Call(ctx, toolName, args)
	if err != nil {
		return "", err
	}
	if !result.Success {
		detail := strings.TrimSpace(result.Text)
		switch {
		case result.Error != "" && detail != "":
			return "", &ltpxToolError{message: "工具 " + toolName + " 执行失败: " + result.Error + "；已完成部分：" + detail}
		case result.Error != "":
			return "", &ltpxToolError{message: "工具 " + toolName + " 执行失败: " + result.Error}
		case detail != "":
			return "", &ltpxToolError{message: "工具 " + toolName + " 未成功完成；已完成部分：" + detail}
		default:
			return "", &ltpxToolError{message: "工具 " + toolName + " 执行失败"}
		}
	}
	if strings.TrimSpace(result.Text) == "" {
		return "工具 " + toolName + " 已执行完成（无文本结果）", nil
	}
	return result.Text, nil
}

type ltpxUnavailableError struct{ tool string }

func (err *ltpxUnavailableError) Error() string {
	return "LTPX 协调层未就绪，无法调用工具 " + err.tool
}

type ltpxToolError struct{ message string }

func (err *ltpxToolError) Error() string { return err.message }

// ExecuteUseTheProgram use_the_program 工具处理函数。
// 输入严格使用统一原语 {type, instruction, handle, details}，输出统一为 {result, handle, error}。
func ExecuteUseTheProgram(args map[string]any) ToolResult {
	call, err := agentdispatch.Parse(args)
	if err != nil {
		return toolJSON(agentdispatch.Failure(err.Error(), nil))
	}
	available := crystalProgramNames()
	if len(available) == 0 {
		// 工具链可能刚装好但尚未同步：执行前补一次，避免把合法应用误判成不存在
		SyncLTPXRemoteTools()
		available = crystalProgramNames()
	}
	if !containsProgramName(available, call.Type) {
		names := strings.Join(available, "、")
		if names == "" {
			names = "无"
		}
		return toolJSON(agentdispatch.Failure("type 只能取以下应用名："+names, nil))
	}
	if call.Details {
		text, found := queryCrystalProgramDetails(call.Type)
		if !found {
			return toolJSON(agentdispatch.Failure(text, nil))
		}
		return toolJSON(agentdispatch.Success(text, nil))
	}
	argsJSON := safeJSON(map[string]any{
		"type":        call.Type,
		"instruction": call.Instruction,
		"handle":      call.Handle,
	}, "{}")
	// 对话者正阻塞在工具等待上、模型空闲：开启特许运行，让工具内部的多轮推理即时处理；
	// 拿到工具返回（含失败）后才关闭
	Responder.BeginPermit()
	defer Responder.EndPermit()
	text, err := callLTPXRemoteTool(ToolUseTheProgram, argsJSON)
	SyncLTPXRemoteTools()
	if err != nil {
		return toolJSON(agentdispatch.Failure("【工具调用失败】"+err.Error(), call.Handle))
	}
	return toolJSON(agentdispatch.Success(text, call.Handle))
}

// containsProgramName 判断应用名是否在当前清单中（与枚举同源，避免大小写误判）。
func containsProgramName(names []string, target string) bool {
	for _, name := range names {
		if name == target {
			return true
		}
	}
	return false
}
