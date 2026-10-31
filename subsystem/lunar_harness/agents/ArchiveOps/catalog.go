package ArchiveOps

// catalog.go 归档操作智能体的工具目录（供控制台「工具列表」屏展示）。

import "LunarSubsystem/LunarHarness/protocol/agentinfo"

// ToolCatalog 返回归档操作智能体的工具清单。
func ToolCatalog() []agentinfo.Agent {
	tools := []agentinfo.Tool{
		{Name: OpCreate, Description: "创建压缩包：把 /document 下的文件或目录打包为 .zip / .ltpx。"},
		{Name: OpDelete, Description: "删除压缩包，并清理对应句柄映射。"},
		{Name: OpAdd, Description: "在压缩包内新增或覆盖文件：可来自本地文件路径，也可直接写入文本内容。"},
		{Name: OpRemove, Description: "从压缩包内移除指定条目。"},
		{Name: OpList, Description: "查询压缩包内部文件结构（转调归档解析智能体）。"},
		{Name: OpRead, Description: "读取压缩包内指定条目的文本内容（转调归档解析智能体）。"},
	}
	return []agentinfo.Agent{{
		ID: "archive_ops", Name: "归档操作", Source: "agents/ArchiveOps",
		Note:  "归档操作智能体：持有压缩包创建 / 删除 / 条目新增 / 条目移除的全量生命周期能力，并复用归档解析管线读取内容。",
		Tools: tools,
	}}
}
