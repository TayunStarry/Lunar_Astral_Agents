package lunar

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/protocol/ltpx"
)

const crystalToolName = "use_the_program"

// syncLTPXRemoteTools 读取进程内工具链并写入应用清单缓存；返回是否同步成功
func syncLTPXRemoteTools() bool {
	broker := ltpx.GetBroker()
	if broker == nil {
		return false
	}
	status := broker.Status()
	if status.Tool == nil || status.Tool.Name == "" {
		return false
	}
	tools := make([]LTPXRemoteToolRef, 0, len(status.Tools))
	names := make([]string, 0, len(status.Tools))
	for _, item := range status.Tools {
		if item.Name == "" {
			continue
		}
		tools = append(tools, LTPXRemoteToolRef{Name: item.Name, Description: item.Description})
		names = append(names, item.Name)
	}
	ltpMu.Lock()
	ltpSubTools = tools
	if len(names) > 0 && !equalStrings(names, ltpLoggedNames) {
		ltpLoggedNames = names
		logger.Info(lunarCoreModule, "已同步 LTPX 工具: \n%s", strings.Join(names, "\n"))
	}
	ltpMu.Unlock()
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
	ltpMu.RLock()
	defer ltpMu.RUnlock()
	names := make([]string, 0, len(ltpSubTools))
	for _, item := range ltpSubTools {
		names = append(names, item.Name)
	}
	return names
}

// getCrystalRemoteTools 读取琉璃聚合工具定义
func getCrystalRemoteTools() []ToolCall {
	ltpMu.RLock()
	empty := len(ltpSubTools) == 0
	ltpMu.RUnlock()
	if empty {
		syncLTPXRemoteTools()
	}
	return []ToolCall{crystalToolDefinition()}
}

// crystalToolDefinition 用当前应用清单构建聚合工具定义
func crystalToolDefinition() ToolCall {
	names := crystalProgramNames()
	properties := map[string]any{
		"details": map[string]any{
			"type":        "boolean",
			"description": "为 true 时只返回 tool 所指应用的详细说明，不执行任何操作；此时仍需传 tool",
		},
		"tool": map[string]any{
			"type": "string",
			"description": func() string {
				if len(names) > 0 {
					return "要使用的应用名，只能取枚举值；只填本项表示打开该应用"
				}
				return "要使用的应用名（当前没有可用应用）"
			}(),
		},
		"instruction": map[string]any{
			"type":        "string",
			"description": "打开应用后交给它执行的具体任务；只打开应用时省略",
		},
	}
	if len(names) > 0 {
		properties["tool"].(map[string]any)["enum"] = names
	}
	required := []string{}
	if len(names) > 0 {
		required = append(required, "tool")
	}
	return ToolCall{
		Type: "function",
		Function: ToolDefinition{
			Name: crystalToolName,
			Description: "调用星月智能的扩展应用。",
			Parameters: map[string]any{
				"type":       "object",
				"properties": properties,
				"required":   required,
			},
		},
	}
}

// queryCrystalProgramDetails 按工具名返回应用详情
func queryCrystalProgramDetails(name string) string {
	names := crystalProgramNames()
	if name == "" {
		return "查询应用详情失败：details=true 时还需传 tool（应用名）。当前可用应用：" + strings.Join(names, "、") + "。"
	}
	ltpMu.RLock()
	var found *LTPXRemoteToolRef
	for i := range ltpSubTools {
		if ltpSubTools[i].Name == name {
			found = &LTPXRemoteToolRef{Name: ltpSubTools[i].Name, Description: ltpSubTools[i].Description}
			break
		}
	}
	ltpMu.RUnlock()
	if found == nil {
		syncLTPXRemoteTools()
		ltpMu.RLock()
		for i := range ltpSubTools {
			if ltpSubTools[i].Name == name {
				found = &LTPXRemoteToolRef{Name: ltpSubTools[i].Name, Description: ltpSubTools[i].Description}
				break
			}
		}
		ltpMu.RUnlock()
	}
	if found == nil {
		return "未找到应用 " + name + "；当前可用应用：" + strings.Join(crystalProgramNames(), "、") + "。"
	}
	description := found.Description
	if description == "" {
		description = "（该应用未提供说明）"
	}
	return "【" + found.Name + "】" + description + "\n\n用法：只打开该应用传 tool=" + found.Name + "；要它执行任务再填 instruction。"
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

// handleUseTheProgram use_the_program 工具处理函数
func handleUseTheProgram(args map[string]any) ToolResult {
	tool := stringArg(args, "tool")
	instruction := stringArg(args, "instruction")
	detailsArg := args["details"]
	detailsName := ""
	if text, ok := detailsArg.(string); ok && text != "true" && text != "false" {
		detailsName = strings.TrimSpace(text)
	}
	if detailsArg == true || detailsArg == "true" || detailsName != "" {
		return ToolResult{Text: queryCrystalProgramDetails(detailsName)}
	}
	if tool == "" {
		available := strings.Join(crystalProgramNames(), "、")
		if available == "" {
			available = "无"
		}
		return ToolResult{Text: "调用失败：tool 不能为空，只能取以下应用名：" + available + "。只打开应用时无需 instruction。"}
	}
	argsJSON := safeJSON(map[string]any{"tool": tool, "instruction": instruction}, "{}")
	text, err := callLTPXRemoteTool(crystalToolName, argsJSON)
	syncLTPXRemoteTools()
	if err != nil {
		return ToolResult{Text: "【工具调用失败】" + err.Error()}
	}
	return ToolResult{Text: text}
}
