package BilibiliViewer

// repair.go 向拯救者注册哔哩观影的修复能力。
//
// 观影是跨 Tick 推进的多步轮次：某一步反复报错时，reset 直接丢弃本轮进度，
// 让下一次观影从选片重新开始（已看记录与偏好配置不受影响）。

import (
	"LunarSubsystem/LunarHarness/engine/agentrepair"
	"LunarSubsystem/LunarHarness/engine/console/logger"
)

func init() {
	agentrepair.RegisterResettable("bilibili", "哔哩观影",
		"哔哩观影进行中的轮次（候选、步骤进度）；reset 丢弃本轮进度，已看记录与偏好不变",
		resetViewingState)
}

// resetViewingState 丢弃等待 Tick 的观影轮次。
func resetViewingState() string {
	manager.mu.Lock()
	had := manager.job != nil
	manager.job = nil
	manager.mu.Unlock()
	if had {
		logger.Warn("BilibiliViewer", "[拯救] 已丢弃哔哩观影进行中的轮次")
		return "已丢弃哔哩观影进行中的轮次"
	}
	return "哔哩观影当前没有进行中的轮次"
}
