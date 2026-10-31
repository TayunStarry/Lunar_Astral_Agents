package ArchiveReader

// dump.go 归档解析的导出：只读能力，不保留上下文。

import (
	"time"

	"LunarSubsystem/LunarHarness/engine/agentdump"
)

func init() {
	agentdump.Register(agentdump.Provider{
		ID:          "archive_reader",
		Label:       "归档解析",
		Group:       "Go 智能体",
		Description: "归档解析智能体的压缩包结构 / 条目读取 / GGUF 元数据工具清单",
		Export: func(includeImages bool) (any, error) {
			return map[string]any{
				"kind":          "archive-reader-context",
				"generated_at":  time.Now().Format(time.RFC3339),
				"message_count": 0,
				"note":          "归档解析按需读取压缩包结构与条目内容，不写入、不保留跨需求上下文",
				"tools": []map[string]string{
					{"name": ToolList, "description": "查询压缩包内部文件结构"},
					{"name": ToolRead, "description": "读取压缩包内指定条目的文本内容"},
					{"name": ToolGGUF, "description": "解析 GGUF 模型元数据"},
				},
			}, nil
		},
	})
}
