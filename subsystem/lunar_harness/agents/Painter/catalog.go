package Painter

// catalog.go 绘制者的工具目录（供控制台「工具列表」屏展示）。
//
// 工具声明归智能体：init() 把 tools.go 的函数工具登记进 toolreg，
// 目录由注册表派生，链只读取不硬编码。

import (
	"LunarSubsystem/LunarHarness/protocol/agentinfo"
	"LunarSubsystem/LunarHarness/protocol/toolreg"
)

func init() {
	for _, def := range toolDefinitions() {
		toolreg.Register(agentID, toolreg.Schema{
			Name:        def.Function.Name,
			Description: def.Function.Description,
			Parameters:  def.Function.Parameters,
		})
	}
}

// ToolCatalog 返回绘制者的工具清单。
func ToolCatalog() []agentinfo.Agent {
	return []agentinfo.Agent{{
		ID: agentID, Name: roleLabel,
		Note:  "绘图智能体：文生图、图生图、角色立绘与相册管理。",
		Tools: agentinfo.ToolsFromRegistry(agentID),
	}}
}
