package thinking

// tool_catalog.go 对话者的工具目录（供控制台「工具列表」屏扫描展示）。

import (
	"LunarSubsystem/LunarHarness/agents/Dialogue"
	"LunarSubsystem/LunarHarness/protocol/agentinfo"
)

// ToolCatalog 返回思维链对话者的工具清单。
//
// 其它智能体（绘制者、演奏者、截图者等）各自在自己包的 catalog.go 里提供
// ToolCatalog()，由装配层汇总，链侧不再硬编码它们的工具目录。
//
// 注意：记忆者不是独立智能体——它只是记忆入库与检索的后端流程，相关能力通过
// 对话者的 fetch_resources 工具暴露，因此这里不再把它列为一个智能体。
func ToolCatalog() []agentinfo.Agent {
	return []agentinfo.Agent{
		{
			ID: "dialogue", Name: Dialogue.Role,
			Note:  "对话智能体：理解对话意图、调度工具并生成回复。",
			Tools: toolCallsInfo(dialogueToolCalls()),
		},
	}
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
