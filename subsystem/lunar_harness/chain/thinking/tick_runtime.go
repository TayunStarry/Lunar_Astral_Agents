package thinking

// tick_runtime.go 月华接入引擎 Tick 运行时的声明层。
//
// 调度规则（忙让出、特许响应、强制事件抢占、优先级排序、空闲冷却与状态持久化）
// 全部由 chain/thinking/tickloop 提供；本文件只声明月华的快照来源与宿主钩子。

import (
	"fmt"
	"time"

	"LunarSubsystem/LunarHarness/agents/Savior"
	"LunarSubsystem/LunarHarness/chain/life"
	"LunarSubsystem/LunarHarness/chain/thinking/tickloop"
	"LunarSubsystem/LunarHarness/engine/tickpermit"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// newTickScheduler 创建月华 Tick 调度器并恢复上次持久化状态。
func newTickScheduler() *tickScheduler {
	return tickloop.New(tickloop.Hooks[TickSnapshot]{
		Module:        tickModule,
		StateFile:     tickStateFile,
		PersistEveryN: tickPersistEveryN,
		Snapshot: func(idleTicks int) TickSnapshot {
			return TickSnapshot{
				Now:            time.Now(),
				IdleTicks:      idleTicks,
				Unread:         UnreadCount(),
				SalvagePending: Savior.Pending(),
			}
		},
		Permit:        tickpermit.Default(),
		Forced:        life.HasPendingEvent,
		NotifyIdle:    func() { notifyIdleHook() },
		IdleThreshold: life.IdleThreshold,
		OnError: func(systemID string, err error) {
			reportTickError(systemID, err)
		},
	})
}

// reportTickError 把 Tick 调度失败上报给智能修复。
//
// 与模型失败路径保持一致，统一走 ReportRun（主动取消与循环截断不触发修复）；
// 错误里只补「调度阶段 + 失败系统」，不掺入模型调用的专用字段。
func reportTickError(systemID string, err error) {
	if err == nil {
		return
	}
	agentbase.ReportRun(repairTargetOf(systemID), fmt.Errorf("Tick 调度失败（%s）: %w", systemID, err))
}

// repairTargetOf 把 Tick 体系映射到拯救者可修复的上下文目标。
func repairTargetOf(systemID string) string {
	if systemID == "dialogue" {
		return "dialogue"
	}
	return systemID
}
