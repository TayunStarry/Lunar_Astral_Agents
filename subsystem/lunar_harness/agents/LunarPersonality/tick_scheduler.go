package LunarPersonality

// tick_scheduler.go 统一 Tick 调度器。
//
// 规则：
//  1. 任一体系运行中（busy=true）→ 当前 Tick 直接跳过，不计数；
//  2. 存在工具触发的强制生活事件 → 跳过其他体系，下一 Tick 直接进入生活链；
//  3. 无就绪体系 → 空闲 Tick：生活冷却 +1（归属后勤体系），并通知 IDLE 钩子；
//  4. 未读消息 → 对话体系，计数清零；
//  5. 待处理视频切片 → 观影体系，计数清零；
//  6. 空闲计数达到生活阈值 → 生活体系，计数清零；
//  7. 待修复故障 → 后勤体系（不重置生活冷却）。
//
// 外部推理请求不在这里调度：响应者拥有独立的一秒 Tick（见 responder_loop.go），
// 只在主调度器空档期运行，对话系统调用 LTPX 工具时会被临时放行一次。
// 体系通过 TickSystem 接口注册，主调度逻辑不感知具体实现。

import (
	"context"
	"encoding/json"
	"sort"
	"sync"
	"time"

	"LunarSubsystem/LunarHarness/agents/Responder"
	"LunarSubsystem/LunarHarness/agents/Savior"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/life"
	"LunarSubsystem/LunarHarness/engine/persistence"
)

type tickScheduler struct {
	mu      sync.Mutex
	systems []TickSystem
	state   TickState
	busy    bool
	// paused 维护态：为 true 时 Tick 不再触发任何体系（打包 / 压缩期间使用）
	paused  bool
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	tickSeq int64
}

func newTickScheduler() *tickScheduler {
	ctx, cancel := context.WithCancel(context.Background())
	s := &tickScheduler{ctx: ctx, cancel: cancel, state: TickState{Version: 1}}
	s.loadState()
	return s
}

// Register 注册体系；按 Priority 降序排序。重复 ID 忽略。
func (s *tickScheduler) Register(system TickSystem) {
	if system == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range s.systems {
		if item.ID() == system.ID() {
			logger.Warn(tickModule, "[Tick] 体系 %s 重复注册，已忽略", system.ID())
			return
		}
	}
	s.systems = append(s.systems, system)
	sort.SliceStable(s.systems, func(i, j int) bool {
		return s.systems[i].Priority() > s.systems[j].Priority()
	})
}

// Tick 每秒调用一次；非阻塞，长任务在独立 goroutine 中执行。
func (s *tickScheduler) Tick() {
	// 维护态（打包 / 压缩）期间不触发任何体系
	s.mu.Lock()
	if s.paused {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()

	// 响应者正在处理外部推理：本轮让出模型，等它处理完再触发体系
	if Responder.Busy() {
		return
	}

	// 工具触发的生活事件在下一 Tick 抢占执行：即使有未读消息也先进入生活链，
	// 进入后先推进心情/情感，再按强制事件跳过权重选择。
	forcedLife := life.HasPendingEvent()
	s.mu.Lock()
	if s.busy {
		s.mu.Unlock()
		return
	}
	snap := TickSnapshot{
		Now:            time.Now(),
		IdleTicks:      s.state.IdleTicks,
		Unread:         UnreadCount(),
		ViewingPending: viewingPendingHook(),
		SalvagePending: Savior.Pending(),
	}
	for _, system := range s.systems {
		if forcedLife && system.ID() != "life" {
			continue
		}
		ready, reason := system.Ready(snap)
		if !ready {
			continue
		}
		s.busy = true
		if keeper, ok := system.(idleKeeper); ok && keeper.KeepIdle() {
			// 后勤体系接管空闲 Tick：本次运行同样推进一格生活冷却
			s.state.IdleTicks++
		} else {
			s.state.IdleTicks = system.IdleReset()
		}
		s.state.LastSystem = system.ID()
		s.state.LastReason = reason
		s.state.LastTriggerAt = snap.Now.Unix()
		s.state.UpdatedAt = snap.Now.Unix()
		s.tickSeq++
		seq := s.tickSeq
		s.persistLocked()
		s.mu.Unlock()
		logger.Info(tickModule, "[Tick#%d] 触发 %s（%s），空闲进度为 %d", seq, system.ID(), reason, s.state.IdleTicks)
		s.run(seq, system, snap.Now)
		return
	}
	s.state.IdleTicks++
	if s.state.IdleTicks > 0 && s.state.IdleTicks%60 == 0 {
		logger.Info(tickModule, "[Tick] 等待[ 生活系统 ]触发: %d/%d", s.state.IdleTicks, life.IdleThreshold())
	}
	s.persistEveryNLocked()
	s.mu.Unlock()

	// 空闲 Tick 仍要推进 QQ 桥接队列：上一轮回应已转发后，桥接器需要这个信号
	// 才会把等待中的下一条请求送进未读队列。旧实现由每秒 pullContext() 顺带调用，
	// 统一调度器在无未读时不再跑对话体系，因此必须在这里显式通知。
	notifyIdleHook()
}

func (s *tickScheduler) run(seq int64, system TickSystem, started time.Time) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.Error(tickModule, "[Tick#%d] %s 执行异常: %v", seq, system.ID(), recovered)
			}
			s.mu.Lock()
			s.busy = false
			s.state.UpdatedAt = time.Now().Unix()
			s.persistLocked()
			s.mu.Unlock()
			logger.Info(tickModule, "[Tick#%d] 释放 %s，耗时 %v", seq, system.ID(), time.Since(started).Round(time.Millisecond))
		}()
		if err := system.Run(s.ctx); err != nil {
			logger.Warn(tickModule, "[Tick#%d] %s 执行失败: %v", seq, system.ID(), err)
			// 交给拯救者：由后勤体系在后续 Tick 里尝试修复上下文，避免同一错误反复崩溃
			Savior.Report(repairTargetOf(system.ID()), err)
		}
	}()
}

// repairTargetOf 把 Tick 体系映射到拯救者可修复的上下文目标。
func repairTargetOf(systemID string) string {
	switch systemID {
	case "dialogue":
		return "core"
	case "viewing":
		return "bilibili"
	default:
		return systemID
	}
}

// Available 报告主调度器是否处于空档期（可承接外部推理）。
// 响应者独立 Tick 据此判断：任一体系运行中或维护态时直接跳过。
func (s *tickScheduler) Available() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.busy && !s.paused
}

// Paused 报告主调度器是否处于维护暂停态（响应者 Tick 据此无条件跳过）。
func (s *tickScheduler) Paused() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.paused
}

// Pause 进入维护态：后续 Tick 不再触发任何体系（正在运行的一轮不受影响）。
func (s *tickScheduler) Pause() {
	s.mu.Lock()
	s.paused = true
	s.mu.Unlock()
}

// Resume 退出维护态，恢复 Tick 调度。
func (s *tickScheduler) Resume() {
	s.mu.Lock()
	s.paused = false
	s.mu.Unlock()
}

// WaitIdle 等待当前运行中的一轮结束；超时返回 false。
func (s *tickScheduler) WaitIdle(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		s.mu.Lock()
		busy := s.busy
		s.mu.Unlock()
		if !busy {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(maintenanceIdlePoll)
	}
}

// Stop 取消在跑任务并等待退出。
func (s *tickScheduler) Stop() {
	s.cancel()
	s.wg.Wait()
	s.mu.Lock()
	s.persistLocked()
	s.mu.Unlock()
}

func (s *tickScheduler) loadState() {
	raw, ok := persistence.ReadFileBytes(tickStateFile)
	if !ok {
		return
	}
	var state TickState
	if err := json.Unmarshal(raw, &state); err != nil {
		logger.Warn(tickModule, "[Tick] 状态文件损坏，空闲计数从 0 开始: %v", err)
		return
	}
	if state.Version == 0 {
		state.Version = 1
	}
	s.state = state
	logger.Info(tickModule, "[Tick] 已恢复空闲计数 %d，上次触发 %s", state.IdleTicks, state.LastSystem)
}

func (s *tickScheduler) persistEveryNLocked() {
	if s.state.IdleTicks%tickPersistEveryN == 0 {
		s.state.UpdatedAt = time.Now().Unix()
		s.persistLocked()
	}
}

func (s *tickScheduler) persistLocked() {
	raw, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		logger.Warn(tickModule, "[Tick] 状态序列化失败: %v", err)
		return
	}
	if err := persistence.WriteFileAtomic(tickStateFile, raw); err != nil {
		logger.Warn(tickModule, "[Tick] 状态写入失败: %v", err)
	}
}
