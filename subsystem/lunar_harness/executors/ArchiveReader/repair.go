package ArchiveReader

// repair.go 向维修侧注册归档解析的只读诊断入口。
//
// 归档解析按次执行（每次独立读取压缩包结构），不保留跨轮上下文；
// 失败通常来自压缩包本身。

import (
	"LunarSubsystem/LunarHarness/protocol/agentrepair"
)

func init() {
	agentrepair.RegisterReadonly("archive_reader", "归档解析",
		"归档解析的最近一次解析记录（按次执行、不保留对话上下文，只读）",
		func(int) (any, error) { return exportContext(false) })
}
