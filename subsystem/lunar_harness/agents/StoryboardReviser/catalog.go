package StoryboardReviser

// catalog.go 分镜规划-修订的阶段能力目录（供控制台「工具列表」屏展示）。

import (
	"LunarSubsystem/LunarHarness/protocol/agentinfo"
	"LunarSubsystem/LunarHarness/protocol/toolreg"
)

func init() {
	toolreg.Register(AgentID, toolreg.Schema{
		Name:        "revise_storyboard_shots",
		Description: "整理用户审查后的分镜稿：补全空画面描述与镜头衔接、统一编号，不改用户已写内容。",
	})
}

// ToolCatalog 返回分镜规划-修订的能力。
func ToolCatalog() []agentinfo.Agent {
	return []agentinfo.Agent{{
		ID: AgentID, Name: modelRole,
		Note:  "演绎链审查后一步：把用户确认的分镜整理成可直接绘制的最终稿。",
		Tools: agentinfo.ToolsFromRegistry(AgentID),
	}}
}
