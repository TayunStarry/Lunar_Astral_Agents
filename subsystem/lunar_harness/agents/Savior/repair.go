package Savior

// repair.go 智能修复自身也是可修复目标：故障队列被坏数据卡住时允许整体清空。

import (
	"fmt"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/protocol/agentrepair"
)

func init() {
	agentrepair.RegisterResettable("savior", "智能修复",
		"智能修复的待办故障与静默期；reset 清空待修故障并解除全部静默",
		resetFailures)
}

// resetFailures 清空故障队列与静默表。
func resetFailures() string {
	mu.Lock()
	count := len(pending)
	pending = map[string]*Failure{}
	order = nil
	cooldown = map[string]time.Time{}
	mu.Unlock()
	logger.Warn(roleLabel, "[拯救] 故障队列已清空（%d 条），全部静默解除", count)
	return fmt.Sprintf("已清空 %d 条待修复故障并解除静默", count)
}
