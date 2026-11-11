//go:build windows

package plan

// catalog.go 「桌面规划」的阶段能力目录（供控制台「工具列表」屏展示）。
//
// submit_decision 只是模型与链之间的提交约定（必填工具调用），由本成员直接放进
// 请求的 Tools 里，链侧解析参数后自行推进，没有任何执行器会「执行」它，
// 因此不登记到 toolreg——工具目录只列真正可执行的动作。
// 本成员现无可执行工具。

import (
	"LunarSubsystem/LunarHarness/protocol/agentinfo"
)

// ToolCatalog 返回桌面规划的阶段能力（当前没有可执行工具）。
func ToolCatalog() []agentinfo.Agent {
	return []agentinfo.Agent{{
		ID: AgentID, Name: modelRole,
		Note:  "桌面操作族第二步：把口语请求收敛为结构化任务，并决策下一步动作；submit_decision 是提交约定而非可执行工具，未登记到工具目录。",
		Tools: agentinfo.ToolsFromRegistry(AgentID),
	}}
}
