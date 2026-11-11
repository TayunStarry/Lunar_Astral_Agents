//go:build windows

package execute

// catalog.go 「桌面执行」的阶段能力目录（供控制台「工具列表」屏展示）。
// 工具声明与执行都归本成员，链不硬编码工具目录。

import (
	"LunarSubsystem/LunarHarness/protocol/agentinfo"
	"LunarSubsystem/LunarHarness/protocol/toolreg"
)

func init() {
	for name, def := range allToolDefs {
		toolreg.Register(AgentID, toolreg.Schema{Name: name, Description: def.Function.Description})
	}
}

// ToolCatalog 返回桌面执行的全部可执行动作（按意图白名单约束实际暴露给模型的子集）。
func ToolCatalog() []agentinfo.Agent {
	return []agentinfo.Agent{{
		ID: AgentID, Name: modelRole,
		Note:  "桌面操作族第三步：执行所有会改变桌面状态的动作（启动窗口 / 键鼠 / UIA 写入），按意图暴露工具子集。",
		Tools: agentinfo.ToolsFromRegistry(AgentID),
	}}
}
