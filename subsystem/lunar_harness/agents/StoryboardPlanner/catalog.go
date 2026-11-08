package StoryboardPlanner

// catalog.go 分镜规划-拆解的阶段能力目录（供控制台「工具列表」屏展示）。

import (
	"LunarSubsystem/LunarHarness/protocol/agentinfo"
	"LunarSubsystem/LunarHarness/protocol/toolreg"
)

func init() {
	toolreg.Register(AgentID, toolreg.Schema{
		Name:        "plan_storyboard_shots",
		Description: "读故事并完成叙事推演与逐镜拆分，流式产出可读推演与结构化分镜。",
	})
}

// ToolCatalog 返回分镜规划-拆解的能力。
func ToolCatalog() []agentinfo.Agent {
	return []agentinfo.Agent{{
		ID: AgentID, Name: modelRole,
		Note:  "演绎链核心一步：推演故事并逐一构建全部分镜。",
		Tools: agentinfo.ToolsFromRegistry(AgentID),
	}}
}
