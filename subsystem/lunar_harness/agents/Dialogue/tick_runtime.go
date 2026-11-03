package Dialogue

// tick_runtime.go 月华接入引擎 Tick 运行时的声明层。
//
// 调度规则（忙让出、强制事件抢占、优先级排序、空闲冷却与状态持久化）全部由
// engine/tickloop 提供；本文件只声明月华的快照来源与宿主钩子，不重复实现调度。

import (
	"time"

	"LunarSubsystem/LunarHarness/agents/Responder"
	"LunarSubsystem/LunarHarness/agents/Savior"
	"LunarSubsystem/LunarHarness/engine/life"
	"LunarSubsystem/LunarHarness/engine/tickloop"
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
				ViewingPending: viewingPendingHook(),
				SalvagePending: Savior.Pending(),
			}
		},
		Busy:          Responder.Busy,
		Forced:        life.HasPendingEvent,
		NotifyIdle:    func() { notifyIdleHook() },
		IdleThreshold: life.IdleThreshold,
		OnError: func(systemID string, err error) {
			// 交给拯救者：由后勤体系在后续 Tick 里尝试修复上下文，避免同一错误反复崩溃
			agentbase.Report(repairTargetOf(systemID), err)
		},
	})
}

// repairTargetOf 把 Tick 体系映射到拯救者可修复的上下文目标。
func repairTargetOf(systemID string) string {
	switch systemID {
	case "dialogue":
		return "dialogue"
	case "viewing":
		return "bilibili"
	default:
		return systemID
	}
}
