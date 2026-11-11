package tickloop

// scheduler.go 通用 Tick 调度器（单循环 + 特许响应车道）。
//
// 规则：
//  1. 任一主链运行中（busy=true）→ 当前 Tick 不再触发其他主链，也不推进空闲计数；
//     开启 AllowResponseWhileBusy 时响应链仍可插入，避免长步骤挡住外部请求；
//  2. 特许运行有效（思考链阻塞在 LTPX 工具等待）→ 只允许响应链并行，其他主链一律不选；
//  3. 存在强制事件 → 只允许 life 体系；
//  4. 无就绪体系 → 空闲 Tick：生活冷却 +1，并通知空闲钩子；
//  5. 按 Priority 降序取第一个就绪体系，空闲进度按该体系的 IdleReset 回写；
//  6. 实现在独立 goroutine 中执行，返回即释放 Tick。
//
// 同一时刻最多一个主链步骤；唯一例外是特许运行期间的响应步骤，由 EndPermit 的
// 返回屏障保证对话恢复前响应步骤已经结束。

import (
	"context"
	"encoding/json"
	"sort"
	"sync"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/persistence"
	"LunarSubsystem/LunarHarness/engine/tickpermit"
)

// Scheduler 通用 Tick 调度器（S 为宿主定义的快照类型）。
type Scheduler[S any] struct {
	hooks   Hooks[S]
	mu      sync.Mutex
	systems []System[S]
	state   State
	busy    bool
	// responseBusy 特许响应步骤是否在途
	responseBusy bool
	// paused 维护态：为 true 时 Tick 不再触发任何体系
	paused  bool
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	tickSeq int64
}

// New 创建调度器并恢复上次持久化状态。
func New[S any](hooks Hooks[S]) *Scheduler[S] {
	if hooks.Module == "" {
		hooks.Module = defaultModule
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &Scheduler[S]{hooks: hooks, ctx: ctx, cancel: cancel, state: State{Version: 1}}
	s.loadState()
	return s
}

// Register 注册体系；按 Priority 降序排序。重复 ID 忽略。
func (s *Scheduler[S]) Register(system System[S]) {
	if system == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range s.systems {
		if item.ID() == system.ID() {
			logger.Warn(s.hooks.Module, "[Tick] 体系 %s 重复注册，已忽略", system.ID())
			return
		}
	}
	s.systems = append(s.systems, system)
	sort.SliceStable(s.systems, func(i, j int) bool {
		return s.systems[i].Priority() > s.systems[j].Priority()
	})
}

// Tick 每秒调用一次；非阻塞，长任务在独立 goroutine 中执行。
func (s *Scheduler[S]) Tick() {
	s.mu.Lock()
	if s.paused {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()

	permit := s.hooks.Permit
	if permit != nil {
		if expired, owner := permit.ExpireIfNeeded(time.Now()); expired {
			logger.Error(s.hooks.Module, "[Tick] 特许运行超时，已强制回收（owner=%s）", owner)
		}
	}

	forced := s.hooks.Forced != nil && s.hooks.Forced()

	s.mu.Lock()
	if s.busy {
		// 特许期间：对话步骤阻塞在工具等待上，只允许响应链并行。
		// 非特许的主链长步骤（创作 / 分镜规划）同样不应挡住外部请求：
		// 开启 AllowResponseWhileBusy 后响应链照常插入，主链之间仍串行。
		if !s.responseBusy {
			if permit != nil && permit.Active() {
				s.tryResponseLocked(permit)
			} else if s.hooks.AllowResponseWhileBusy {
				s.startResponseWhileBusyLocked()
			}
		}
		s.mu.Unlock()
		return
	}
	if s.responseBusy {
		s.mu.Unlock()
		return
	}
	if permit != nil && permit.Active() {
		// 没有主步骤却仍有特许：视为泄漏，回收后按正常流程继续
		permit.ForceClear()
		logger.Warn(s.hooks.Module, "[Tick] 检测到无主特许运行，已回收")
	}

	var snap S
	if s.hooks.Snapshot != nil {
		snap = s.hooks.Snapshot(s.state.IdleTicks)
	}
	for _, system := range s.systems {
		kind := KindOf(system)
		if forced && kind != KindLife {
			continue
		}
		ready, reason := system.Ready(snap)
		if !ready {
			continue
		}
		if kind == KindResponse {
			// 正常空档期的响应步骤：同步标记在途，使后续特许 End 的屏障能等待它结束。
			if s.hooks.Permit != nil {
				s.hooks.Permit.MarkResponse(true)
			}
			s.startResponseLocked(system, reason)
			s.mu.Unlock()
			return
		}
		s.busy = true
		if keeper, ok := system.(idleKeeper); ok && keeper.KeepIdle() {
			// 体系接管空闲 Tick：本次运行同样推进一格生活冷却
			s.state.IdleTicks++
		} else {
			s.state.IdleTicks = system.IdleReset()
		}
		s.state.LastSystem = system.ID()
		s.state.LastReason = reason
		s.state.LastTriggerAt = nowUnix()
		s.state.UpdatedAt = nowUnix()
		s.tickSeq++
		seq := s.tickSeq
		s.persistLocked()
		idle := s.state.IdleTicks
		s.mu.Unlock()
		logger.Info(s.hooks.Module, "[Tick#%d] 触发 %s（%s），空闲进度为 %d", seq, system.ID(), reason, idle)
		s.run(seq, system)
		return
	}
	s.state.IdleTicks++
	if s.state.IdleTicks > 0 && s.state.IdleTicks%60 == 0 && s.hooks.IdleThreshold != nil {
		logger.Info(s.hooks.Module, "[Tick] 等待[ 生活系统 ]触发: %d/%d", s.state.IdleTicks, s.hooks.IdleThreshold())
	}
	s.persistEveryNLocked()
	s.mu.Unlock()

	if s.hooks.NotifyIdle != nil {
		s.hooks.NotifyIdle()
	}
}

// tryResponseLocked 特许期间尝试启动一个响应步骤；调用方必须持有锁。
func (s *Scheduler[S]) tryResponseLocked(_ *tickpermit.Controller) {
	var snap S
	if s.hooks.Snapshot != nil {
		snap = s.hooks.Snapshot(s.state.IdleTicks)
	}
	for _, system := range s.systems {
		if KindOf(system) != KindResponse {
			continue
		}
		ready, reason := system.Ready(snap)
		if !ready {
			return
		}
		// 与 End 的撤销在同一把锁内完成：特许已撤销则不再启动新的响应步骤。
		if s.hooks.Permit != nil && !s.hooks.Permit.TryBeginResponse() {
			return
		}
		s.startResponseLocked(system, reason)
		return
	}
}

// startResponseWhileBusyLocked 在主链步骤在途时启动一个响应步骤；调用方必须持有锁。
//
// 响应链只处理外部请求，不读写主链状态；模型服务若为单槽会自然排队，
// 但外部请求不会再被整条长步骤挡住。
func (s *Scheduler[S]) startResponseWhileBusyLocked() {
	var snap S
	if s.hooks.Snapshot != nil {
		snap = s.hooks.Snapshot(s.state.IdleTicks)
	}
	for _, system := range s.systems {
		if KindOf(system) != KindResponse {
			continue
		}
		ready, reason := system.Ready(snap)
		if !ready {
			return
		}
		s.startResponseLocked(system, reason)
		return
	}
}

// startResponseLocked 置响应忙、记录状态并启动 goroutine；调用方必须持有锁。
// 响应在途标记由调用方在启动前完成（特许路径走 TryBeginResponse）。
func (s *Scheduler[S]) startResponseLocked(system System[S], reason string) {
	s.responseBusy = true
	s.state.LastSystem = system.ID()
	s.state.LastReason = reason
	s.state.LastTriggerAt = nowUnix()
	s.state.UpdatedAt = nowUnix()
	s.tickSeq++
	seq := s.tickSeq
	s.persistLocked()
	logger.Info(s.hooks.Module, "[Tick#%d] 触发响应步骤 %s（%s）", seq, system.ID(), reason)
	s.runResponse(seq, system)
}

// run 在独立 goroutine 中执行主链体系，结束后释放 Tick。
func (s *Scheduler[S]) run(seq int64, system System[S]) {
	started := time.Now()
	s.wg.Go(func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.Error(s.hooks.Module, "[Tick#%d] %s 执行异常: %v", seq, system.ID(), recovered)
			}
			s.mu.Lock()
			s.busy = false
			s.state.UpdatedAt = nowUnix()
			s.persistLocked()
			s.mu.Unlock()
			logger.Info(s.hooks.Module, "[Tick#%d] 释放 %s，耗时 %v", seq, system.ID(), time.Since(started).Round(time.Millisecond))
		}()
		if err := system.Run(s.ctx); err != nil {
			logger.Warn(s.hooks.Module, "[Tick#%d] %s 执行失败: %v", seq, system.ID(), err)
			if s.hooks.OnError != nil {
				s.hooks.OnError(system.ID(), err)
			}
		}
	})
}

// runResponse 在独立 goroutine 中执行响应链，结束后清除响应忙与许可屏障。
func (s *Scheduler[S]) runResponse(seq int64, system System[S]) {
	started := time.Now()
	s.wg.Go(func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.Error(s.hooks.Module, "[Tick#%d] %s 执行异常: %v", seq, system.ID(), recovered)
			}
			// 先清调度器忙标记，再清许可屏障：后者唤醒对话继续时，响应步骤已完全返回。
			s.mu.Lock()
			s.responseBusy = false
			s.state.UpdatedAt = nowUnix()
			s.persistLocked()
			s.mu.Unlock()
			if s.hooks.Permit != nil {
				s.hooks.Permit.MarkResponse(false)
			}
			logger.Info(s.hooks.Module, "[Tick#%d] 释放响应步骤 %s，耗时 %v", seq, system.ID(), time.Since(started).Round(time.Millisecond))
		}()
		if err := system.Run(s.ctx); err != nil {
			logger.Warn(s.hooks.Module, "[Tick#%d] %s 执行失败: %v", seq, system.ID(), err)
			if s.hooks.OnError != nil {
				s.hooks.OnError(system.ID(), err)
			}
		}
	})
}

// Status 返回调度状态快照与当前是否忙碌（只读观测用）。
func (s *Scheduler[S]) Status() (State, bool) {
	s.mu.Lock()
	state := s.state
	busy := s.busy || s.responseBusy
	s.mu.Unlock()
	return state, busy || s.hasActiveStep()
}

// Available 报告调度器是否处于空档期。
func (s *Scheduler[S]) Available() bool {
	s.mu.Lock()
	idle := !s.busy && !s.responseBusy && !s.paused
	s.mu.Unlock()
	return idle && !s.hasActiveStep()
}

// hasActiveStep 报告是否有体系的后台步骤在途。
//
// 后台步骤不占调度器的 busy 标记，但空闲判定与维护暂停必须把它们算进忙碌，
// 否则 WaitIdle 会在模型调用尚未结束时误报空闲。
func (s *Scheduler[S]) hasActiveStep() bool {
	s.mu.Lock()
	systems := append([]System[S](nil), s.systems...)
	s.mu.Unlock()
	for _, system := range systems {
		if waiter, ok := system.(StepWaiter); ok && waiter.StepActive() {
			return true
		}
	}
	return false
}

// Paused 报告调度器是否处于维护暂停态。
func (s *Scheduler[S]) Paused() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.paused
}

// Pause 进入维护态：后续 Tick 不再触发任何体系。
func (s *Scheduler[S]) Pause() {
	s.mu.Lock()
	s.paused = true
	s.mu.Unlock()
}

// Resume 退出维护态，恢复 Tick 调度。
func (s *Scheduler[S]) Resume() {
	s.mu.Lock()
	s.paused = false
	s.mu.Unlock()
}

// WaitIdle 等待当前运行中的一轮（含特许响应）结束；超时返回 false。
func (s *Scheduler[S]) WaitIdle(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		s.mu.Lock()
		busy := s.busy || s.responseBusy
		s.mu.Unlock()
		if !busy && !s.hasActiveStep() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(idlePollInterval)
	}
}

// Stop 取消在跑任务并等待退出。
func (s *Scheduler[S]) Stop() {
	s.cancel()
	// 后台步骤（演绎 / 生活）可能仍在模型调用中：先等它们收尾，再等前台 goroutine。
	for _, system := range s.systems {
		waiter, ok := system.(StepWaiter)
		if !ok {
			continue
		}
		if !waiter.WaitStep(stopStepTimeout) {
			logger.Warn(s.hooks.Module, "[Tick] %s 的后台步骤未在 %s 内结束", system.ID(), stopStepTimeout)
		}
	}
	s.wg.Wait()
	s.mu.Lock()
	s.persistLocked()
	s.mu.Unlock()
}

func (s *Scheduler[S]) loadState() {
	if s.hooks.StateFile == "" {
		return
	}
	raw, ok := persistence.ReadFileBytes(s.hooks.StateFile)
	if !ok {
		return
	}
	var state State
	if err := json.Unmarshal(raw, &state); err != nil {
		logger.Warn(s.hooks.Module, "[Tick] 状态文件损坏，空闲计数从 0 开始: %v", err)
		return
	}
	if state.Version == 0 {
		state.Version = 1
	}
	s.state = state
	logger.Info(s.hooks.Module, "[Tick] 已恢复空闲计数 %d，上次触发 %s", state.IdleTicks, state.LastSystem)
}

func (s *Scheduler[S]) persistEveryNLocked() {
	if s.hooks.PersistEveryN <= 0 || s.state.IdleTicks%s.hooks.PersistEveryN != 0 {
		return
	}
	s.state.UpdatedAt = nowUnix()
	s.persistLocked()
}

func (s *Scheduler[S]) persistLocked() {
	if s.hooks.StateFile == "" {
		return
	}
	raw, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		logger.Warn(s.hooks.Module, "[Tick] 状态序列化失败: %v", err)
		return
	}
	if err := persistence.WriteFileAtomic(s.hooks.StateFile, raw); err != nil {
		logger.Warn(s.hooks.Module, "[Tick] 状态写入失败: %v", err)
	}
}
