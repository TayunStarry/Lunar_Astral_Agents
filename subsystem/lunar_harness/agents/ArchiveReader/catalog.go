package ArchiveReader

// catalog.go 归档解析智能体的工具目录（供控制台「工具列表」屏展示）。

import "LunarSubsystem/LunarHarness/protocol/agentinfo"

// ToolCatalog 返回归档解析智能体的工具清单。
func ToolCatalog() []agentinfo.Agent {
	tools := []agentinfo.Tool{
		{Name: ToolList, Description: "查询压缩包内部文件结构：条目路径、原始大小、压缩大小与目录标记。"},
		{Name: ToolRead, Description: "读取压缩包内指定条目的文本内容（单次最多 10000 字，二进制条目拒绝读取）。"},
		{Name: ToolGGUF, Description: "解析 GGUF 模型文件的元数据摘要（模型名、架构、量化、上下文长度等）。"},
	}
	return []agentinfo.Agent{{
		ID: "archive_reader", Name: "归档解析", Source: "agents/ArchiveReader",
		Note:  "归档解析智能体：基于 Lunar Harness 原生 zip 机制查询压缩包结构、读取条目内容并解析 GGUF 元数据。",
		Tools: tools,
	}}
}
