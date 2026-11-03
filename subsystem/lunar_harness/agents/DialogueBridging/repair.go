package DialogueBridging

// repair.go 向拯救者注册通讯桥接的修复能力。
//
// 桥接按次规划并投递，不保留跨轮上下文；可修的是最近一次投递记录，
// 避免损坏的记录一直被当作「当前桥接上下文」导出。

import (
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/protocol/agentrepair"
)

func init() {
	agentrepair.RegisterResettable("dialogue_bridging", "通讯桥接",
		"通讯桥接最近一次需求、投递清单与执行报告；reset 清空该记录",
		resetDelivery)
}

// resetDelivery 清空最近一次投递记录。
func resetDelivery() string {
	lastMu.Lock()
	had := lastDelivery != nil
	lastDelivery = nil
	lastMu.Unlock()
	if had {
		logger.Warn("DialogueBridging", "[拯救] 通讯桥接最近一次投递记录已清空")
		return "已清空通讯桥接最近一次投递记录"
	}
	return "通讯桥接当前没有投递记录"
}
