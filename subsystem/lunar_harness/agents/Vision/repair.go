package Vision

// repair.go 向拯救者注册视觉理解的修复能力。
//
// 视觉理解按次执行（每次独立读图 / 读视频），不保留跨轮上下文；
// 失败通常来自输入资源本身，reset 只清理运行期标记。

import (
	"LunarSubsystem/LunarHarness/protocol/agentrepair"
)

func init() {
	agentrepair.RegisterResettable("vision", "视觉理解",
		"视觉理解按次执行、不保留对话上下文；reset 清理运行期状态",
		resetVision,
		func(int) (any, error) { return exportContext(false) })
}

// resetVision 视觉理解没有持久上下文，重置是安全且幂等的。
func resetVision() string {
	return "视觉理解按次执行，没有可清理的对话上下文"
}
