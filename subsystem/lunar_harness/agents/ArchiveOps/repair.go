package ArchiveOps

// repair.go 向维修侧注册归档操作的只读诊断入口。
//
// 归档操作按次执行（每次独立创建 / 删除 / 增删条目），不保留跨轮上下文；
// 失败通常来自压缩包或磁盘状态。

import (
	"LunarSubsystem/LunarHarness/protocol/agentrepair"
)

func init() {
	agentrepair.RegisterReadonly("archive_ops", Module,
		"归档操作的最近一次执行记录（按次执行、不保留对话上下文，只读）",
		func(int) (any, error) { return exportContext(false) })
}
