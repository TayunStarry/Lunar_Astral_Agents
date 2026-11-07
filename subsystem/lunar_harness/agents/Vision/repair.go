package Vision

// repair.go 向维修侧注册视觉理解的只读诊断入口。
//
// 视觉理解按次执行（每次独立读图 / 读视频），不保留跨轮上下文；
// 失败通常来自输入资源本身。

import (
	"LunarSubsystem/LunarHarness/protocol/agentrepair"
)

func init() {
	agentrepair.RegisterReadonly("vision", roleLabel,
		"视觉理解的最近一次识别记录（按次执行、不保留对话上下文，只读）",
		func(int) (any, error) { return exportContext(false) })
}
