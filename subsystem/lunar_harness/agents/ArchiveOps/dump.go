package ArchiveOps

// dump.go 归档操作的能力导出：全量生命周期工具清单。

import (
	"time"

	"LunarSubsystem/LunarHarness/engine/agentdump"
)

func init() {
	agentdump.Register(agentdump.Provider{
		ID:          "archive_ops",
		Label:       "归档操作",
		Group:       "Go 智能体",
		Description: "归档操作智能体的压缩包生命周期工具清单",
		Export: func(includeImages bool) (any, error) {
			return map[string]any{
				"kind":          "archive-ops-context",
				"generated_at":  time.Now().Format(time.RFC3339),
				"message_count": 0,
				"note":          "归档操作按指令执行压缩包创建 / 删除 / 新增 / 移除，读取能力复用归档解析智能体",
				"tools": []map[string]string{
					{"name": OpCreate, "description": "创建压缩包"},
					{"name": OpDelete, "description": "删除压缩包"},
					{"name": OpAdd, "description": "压缩包内新增或覆盖文件"},
					{"name": OpRemove, "description": "压缩包内移除文件"},
					{"name": OpList, "description": "查询压缩包结构"},
					{"name": OpRead, "description": "读取压缩包条目内容"},
				},
			}, nil
		},
	})
}
