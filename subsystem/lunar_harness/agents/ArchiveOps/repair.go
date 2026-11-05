package ArchiveOps

// repair.go 向拯救者注册归档操作的修复能力。
//
// 归档操作按次执行（每次独立创建 / 删除 / 增删条目），不保留跨轮上下文；
// 失败通常来自压缩包或磁盘状态，reset 只清理运行期标记。

import (
	"LunarSubsystem/LunarHarness/protocol/agentrepair"
)

func init() {
	agentrepair.RegisterResettable("archive_ops", "归档操作",
		"归档操作按次执行、不保留对话上下文；reset 清理运行期状态",
		resetArchiveOps,
		func(int) (any, error) { return exportContext(false) })
}

// resetArchiveOps 归档操作没有持久上下文，重置是安全且幂等的。
func resetArchiveOps() string {
	return "归档操作按次执行，没有可清理的对话上下文"
}
