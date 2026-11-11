//go:build windows

package perceive

// catalog.go 「桌面感知」的阶段能力目录（供控制台「工具列表」屏展示）。
//
// 本成员不向模型暴露任何工具：UIA 树由 agent.go 直接读取后作为上下文发送，
// 没有工具执行器，因此不登记 toolreg——工具目录只列真正可执行的动作。

import (
	"LunarSubsystem/LunarHarness/protocol/agentinfo"
)

// ToolCatalog 返回桌面感知的阶段能力（当前没有可执行工具）。
func ToolCatalog() []agentinfo.Agent {
	return []agentinfo.Agent{{
		ID: AgentID, Name: modelRole,
		Note:  "桌面操作族第一步：读取前台窗口 UI 树并由模型整理相关元素；读取在成员内直接完成，不向模型暴露工具。",
		Tools: agentinfo.ToolsFromRegistry(AgentID),
	}}
}
