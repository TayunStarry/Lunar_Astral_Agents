package ArchiveReader

// repair.go 向拯救者注册归档解析的修复能力。
//
// 归档解析按次执行（每次独立读取压缩包结构），不保留跨轮上下文；
// 失败通常来自压缩包本身，reset 只清理运行期标记。

import (
	"LunarSubsystem/LunarHarness/protocol/agentrepair"
)

func init() {
	agentrepair.RegisterResettable("archive_reader", "归档解析",
		"归档解析按次执行、不保留对话上下文；reset 清理运行期状态",
		resetArchiveReader)
}

// resetArchiveReader 归档解析没有持久上下文，重置是安全且幂等的。
func resetArchiveReader() string {
	return "归档解析按次执行，没有可清理的对话上下文"
}
