package DialogueBridging

// repair.go 向维修侧注册通讯桥接的只读诊断入口。
//
// 桥接按次规划并投递，不保留跨轮上下文；可观察的是最近一次投递记录。

import (
	"LunarSubsystem/LunarHarness/protocol/agentrepair"
)

func init() {
	agentrepair.RegisterReadonly("dialogue_bridging", roleLabel,
		"通讯桥接最近一次需求、投递清单与执行报告（只读）",
		func(int) (any, error) { return exportContext(false) })
}
