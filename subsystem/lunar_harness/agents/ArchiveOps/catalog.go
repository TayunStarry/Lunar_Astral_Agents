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
	}
	return []agentinfo.Agent{{
		ID: "archive_ops", Name: "归档操作", Source: "agents/ArchiveOps",
		Note:  "归档操作智能体：按自然语言指令创建 / 删除压缩包、增删条目；结构查询与内容读取归归档解析智能体。",
		Tools: tools,
	}}
}
