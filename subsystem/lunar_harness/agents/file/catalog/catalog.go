// Package catalog 文件族的工具目录汇总入口。
//
// 族根不 import 成员，调用方也不应散落 import 五个成员包：控制台需要一次性
// 取到全部成员的阶段能力时，统一从这里取。
package catalog

import (
	"LunarSubsystem/LunarHarness/agents/file/CodeExecution"
	"LunarSubsystem/LunarHarness/agents/file/ContentRollback"
	"LunarSubsystem/LunarHarness/agents/file/FileOperations"
	"LunarSubsystem/LunarHarness/agents/file/ManipulateArchive"
	"LunarSubsystem/LunarHarness/agents/file/ManipulateFiles"
	"LunarSubsystem/LunarHarness/protocol/agentinfo"
)

// AgentIDs 返回在 protocol/toolreg 登记过能力的成员 ID（顺序：文件操纵 / 归档操纵 / 代码执行 / 内容回滚 / 文件操作）。
func AgentIDs() []string {
	return []string{
		ManipulateFiles.AgentID,
		ManipulateArchive.AgentID,
		CodeExecution.AgentID,
		ContentRollback.AgentID,
		FileOperations.AgentID,
	}
}

// ToolCatalogs 返回族内各成员的工具目录（顺序与 AgentIDs 一致）。
func ToolCatalogs() []agentinfo.Agent {
	out := make([]agentinfo.Agent, 0, 5)
	out = append(out, ManipulateFiles.ToolCatalog()...)
	out = append(out, ManipulateArchive.ToolCatalog()...)
	out = append(out, CodeExecution.ToolCatalog()...)
	out = append(out, ContentRollback.ToolCatalog()...)
	out = append(out, FileOperations.ToolCatalog()...)
	return out
}
