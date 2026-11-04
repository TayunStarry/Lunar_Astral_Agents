package creation

// catalog.go 自由创作体系的工具目录（供控制台「工具列表」屏展示）。

import "LunarSubsystem/LunarHarness/protocol/agentinfo"

// ToolCatalog 返回自由创作体系的工具清单（调度者可调度的全部能力）。
func ToolCatalog() []agentinfo.Agent {
	tools := make([]agentinfo.Tool, 0, len(plannerTools))
	for _, def := range definitionsFor(plannerTools) {
		tools = append(tools, agentinfo.Tool{Name: def.Function.Name, Description: def.Function.Description})
	}
	return []agentinfo.Agent{{
		ID: "creative", Name: "自由创作", Source: "chain/creation",
		Note:  "自由创作智能体：生活创作与对话者 file_operations 编排共用；调度者按计划提交步骤，分派文字创作 / 工程创作 / 资料查阅 / 脚本运行子智能体执行。",
		Tools: tools,
	}}
}
