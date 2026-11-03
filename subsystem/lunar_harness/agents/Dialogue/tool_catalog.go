package Dialogue

// tool_catalog.go 对话者与各扁平智能体的工具目录（供控制台「工具列表」屏扫描展示）。

import (
	"LunarSubsystem/LunarHarness/agents/Painter"
	"LunarSubsystem/LunarHarness/agents/Screenshoter"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
	"LunarSubsystem/LunarHarness/protocol/agentinfo"
)

// ToolCatalog 返回 lunar 包内全部 Go 角色的工具清单。
//
// 注意：记忆者不是独立智能体——它只是记忆入库与检索的后端流程，相关能力通过
// 对话者的 fetch_resources 工具暴露，因此这里不再把它列为一个智能体。
func ToolCatalog() []agentinfo.Agent {
	dialogueCalls := concatToolCalls(
		agentControlTools, fetchResourceTools, lifePlanningTools, networkExploreToolCalls(),
	)
	// LTPX 远程工具是运行时从扩展应用同步来的，和静态工具一起拼进对话者清单。
	// 应用清单不再逐条展开成「use_the_program · 应用名」条目：它们不是独立的工具，
	// 展开只会重复同一句说明，工具列表页已改为「对话者 → 工具」的两级展示。
	dialogueCalls = append(dialogueCalls, getCrystalRemoteTools()...)
	dialogueTools := toolCallsInfo(dialogueCalls)

	return []agentinfo.Agent{
		{
			ID: "dialogue", Name: "对话者", Source: "lunar",
			Note:  "对话智能体：理解对话意图、调度工具并生成回复。",
			Tools: dialogueTools,
		},
		{ID: "painter", Name: "绘制者", Source: "lunar",
			Note:  "绘图智能体：文生图、图生图、角色立绘与相册管理。",
			Tools: toolDefinitionsInfo(Painter.ToolDefinitions())},
		{ID: "musician", Name: "演奏者", Source: "lunar",
			Note:  "音乐智能体：主模型构思作品描述，music_generation 模型生成旋律并渲染演奏音频。",
			Tools: []agentinfo.Tool{{Name: "music_generation", Description: "加载音乐生成模型产出旋律，完成后卸载并恢复主模型"}}},
		{ID: "screenshoter", Name: "截图者", Source: "lunar",
			Note:  "截图智能体：截取屏幕、识别区域并回传图片。",
			Tools: toolDefinitionsInfo(Screenshoter.ToolDefinitions())},
	}
}

// concatToolCalls 合并多组工具定义（不修改原切片）。
func concatToolCalls(groups ...[]ToolCall) []ToolCall {
	out := make([]ToolCall, 0)
	for _, group := range groups {
		out = append(out, group...)
	}
	return out
}

// toolDefinitionsInfo 把扁平智能体的工具声明转成控制台可展示的目录条目。
func toolDefinitionsInfo(defs []agentbase.ToolDefinition) []agentinfo.Tool {
	out := make([]agentinfo.Tool, 0, len(defs))
	for _, def := range defs {
		out = append(out, agentinfo.Tool{
			Name:        def.Function.Name,
			Description: def.Function.Description,
		})
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
