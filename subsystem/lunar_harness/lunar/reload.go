package lunar

// reload.go 应答失败计数。
//
// 只用于日志提示「连续第几次失败」，不再触发任何对话链状态重建：
// 自动重建与手动重建实测都无法真正挽救月华，已整体移除。

import (
	"sync/atomic"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// consecutiveReplyFailures 连续应答失败次数（仅用于日志）
var consecutiveReplyFailures atomic.Int32

// notifyAgentSuccess 本轮应答成功，清零失败计数
func notifyAgentSuccess() {
	consecutiveReplyFailures.Store(0)
}

// reportAgentFailure 上报本轮应答失败
func reportAgentFailure(reason string) {
	count := consecutiveReplyFailures.Add(1)
	logger.Warn(lunarCoreModule, "应答失败（连续 %d 次）: %s", count, reason)
}
