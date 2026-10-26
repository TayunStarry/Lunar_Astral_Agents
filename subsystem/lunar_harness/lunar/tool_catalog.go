package lunar

// tool_catalog.go 对话者与月华核心人格各角色的工具目录（供控制台「工具列表」屏扫描展示）。

import (
	"strings"

	"LunarSubsystem/LunarHarness/engine/agentinfo"
)

// ToolCatalog 返回 lunar 包内全部 Go 角色的工具清单。
//
// 注意：记忆者不是独立智能体——它只是记忆入库与检索的后端流程，相关能力通过
// 对话者的 search_memory 工具暴露，因此这里不再把它列为一个智能体。
func ToolCatalog() []agentinfo.Agent {
	dialogueCalls := concatToolCalls(
		agentControlTools, memoryTools, resourceTools, scheduleTools, networkExploreToolCalls(),
	)
	// LTPX 远程工具是运行时从扩展应用同步来的，和静态工具一起拼进对话者清单。
	dialogueCalls = append(dialogueCalls, getCrystalRemoteTools()...)
	dialogueTools := toolCallsInfo(dialogueCalls)
	dialogueTools = append(dialogueTools, crystalProgramTools()...)

	return []agentinfo.Agent{
		{
			ID: "dialogue", Name: "对话者", Source: "lunar",
			Note:  crystalCatalogNote(),
			Tools: dialogueTools,
		},
		{ID: "painter", Name: "绘制者", Source: "lunar", Tools: toolCallsInfo(painterToolDefinitions())},
		{ID: "musician", Name: "演奏者", Source: "lunar", Tools: toolCallsInfo(musicToolDefinitions())},
		{ID: "screenshoter", Name: "截图者", Source: "lunar", Tools: toolCallsInfo(screenshoterToolDefinitions())},
	}
}

// crystalProgramTools 把当前同步到的 LTPX 应用逐个列成可读条目。
// 它们不是独立的 function calling 工具，统一由 use_the_program 的 tool 枚举调用。
func crystalProgramTools() []agentinfo.Tool {
	ltpMu.RLock()
	defer ltpMu.RUnlock()
	out := make([]agentinfo.Tool, 0, len(ltpSubTools))
	for _, item := range ltpSubTools {
		description := strings.TrimSpace(item.Description)
		if description == "" {
			description = "LTPX 扩展应用（未提供说明）"
		}
		out = append(out, agentinfo.Tool{
			Name:        "use_the_program · " + item.Name,
			Description: description,
		})
	}
	return out
}

// crystalCatalogNote 说明 LTPX 工具的调用方式与当前同步状态。
func crystalCatalogNote() string {
	ltpMu.RLock()
	defer ltpMu.RUnlock()
	names := make([]string, 0, len(ltpSubTools))
	for _, item := range ltpSubTools {
		names = append(names, item.Name)
	}
	if len(names) == 0 {
		return "LTPX 远程工具尚未同步到可用应用（调用 use_the_program 时会再次尝试同步）"
	}
	return "LTPX 远程应用通过 use_the_program 聚合调用；当前可用：" + strings.Join(names, "、")
}

// concatToolCalls 合并多组工具定义（不修改原切片）。
func concatToolCalls(groups ...[]ToolCall) []ToolCall {
	out := make([]ToolCall, 0)
	for _, group := range groups {
		out = append(out, group...)
	}
	return out
}

// toolCallsInfo 把 function calling 定义转成控制台可展示的目录条目。
func toolCallsInfo(calls []ToolCall) []agentinfo.Tool {
	out := make([]agentinfo.Tool, 0, len(calls))
	for _, call := range calls {
		out = append(out, agentinfo.Tool{
			Name:        call.Function.Name,
			Description: call.Function.Description,
		})
	}
	return out
}
