package viewer

// repair.go 哔哩观影的智能修复登记。
//
// 新流程不保留跨轮运行期状态：待看列表、片段进度与待回应队列都在 viewer.db，
// 失败由生活回合按持久化状态续跑，因此 reset 只需说明没有可清理的内存状态。
// 与 dump.go 共用同一个 ID，保证「能导出就能修复」的约束成立。

import (
	"LunarSubsystem/LunarHarness/protocol/agentrepair"
)

func init() {
	agentrepair.RegisterResettable(
		"bilibili",
		"哔哩观影",
		"无跨轮运行期状态：待看列表与待回应队列由生活回合按 viewer.db 续跑",
		func() string { return "观影由生活回合驱动，无内存运行期状态需要清理" },
	)
}
