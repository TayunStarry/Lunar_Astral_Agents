package viewer

// repair.go 哔哩观影的只读诊断登记。
//
// 新流程不保留跨轮运行期状态：待看列表、片段进度与待回应队列都在 viewer.db，
// 失败由生活回合按持久化状态续跑；维修侧只读取同一份导出。
// 与 dump.go 共用同一个 ID，保证「能导出就能诊断」的约束成立。

import (
	"LunarSubsystem/LunarHarness/protocol/agentrepair"
)

func init() {
	agentrepair.RegisterReadonly(
		chainScopeID,
		chainScopeLabel,
		"无跨轮运行期状态：待看列表与待回应队列由生活回合按 viewer.db 续跑（只读）",
	)
}
