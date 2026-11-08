package StoryboardResearch

// catalog.go 分镜规划-资料的阶段能力目录（供控制台「工具列表」屏展示）。

import (
	"LunarSubsystem/LunarHarness/protocol/agentinfo"
	"LunarSubsystem/LunarHarness/protocol/toolreg"
)

func init() {
	toolreg.Register(AgentID, toolreg.Schema{
		Name:        "plan_storyboard_research",
		Description: "读故事并判断需要考据的真实设定，一次调用产出可读判断与最多 4 条检索意图。",
	})
}

// ToolCatalog 返回分镜规划-资料的能力。
//
// 该智能体不使用 function calling；这里展示它在演绎链中实际承担的一步判断。
func ToolCatalog() []agentinfo.Agent {
	return []agentinfo.Agent{{
		ID: AgentID, Name: modelRole,
		Note:  "演绎链第一步：判断剧情需要补全哪些现实资料，产出检索意图。",
		Tools: agentinfo.ToolsFromRegistry(AgentID),
	}}
}
