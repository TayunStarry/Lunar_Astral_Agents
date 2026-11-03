package tickloop

// scheduler.go 通用 Tick 调度器。
//
// 规则（与月华原调度器一致）：
//  1. 任一体系运行中（busy=true）→ 当前 Tick 直接跳过，不计数；
//  2. 存在强制事件 → 跳过其他体系，只允许 life 体系；
//  3. 无就绪体系 → 空闲 Tick：生活冷却 +1，并通知空闲钩子；
//  4. 按 Priority 降序取第一个就绪体系，空闲进度按该体系的 IdleReset 回写；
//  5. 实现在独立 goroutine 中执行，返回即释放 Tick。
//
// 具体体系由宿主注册，调度逻辑不感知实现。

import (
	"context"
	"encoding/json"
	"sort"
	"sync"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/persistence"
)

// Scheduler 通用 Tick 调度器（S 为宿主定义的快照类型）。
type Scheduler[S any] struct {
	hooks   Hooks[S]
	mu      sync.Mutex
	systems []System[S]
	state   State
	busy    bool
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

	// 宿主有独占模型的任务在跑：本轮让出模型
	if s.hooks.Busy != nil && s.hooks.Busy() {
		return
	}

	forced := s.hooks.Forced != nil && s.hooks.Forced()
	s.mu.Lock()
	if s.busy {
		s.mu.Unlock()
		return
	}
	var snap S
	if s.hooks.Snapshot != nil {
		snap = s.hooks.Snapshot(s.state.IdleTicks)
	}
	for _, system := range s.systems {
		if forced && system.ID() != "life" {
			continue
		}
		ready, reason := system.Ready(snap)
		if !ready {
			continue
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
		s.mu.Unlock()
		logger.Info(s.hooks.Module, "[Tick#%d] 触发 %s（%s），空闲进度为 %d", seq, system.ID(), reason, s.state.IdleTicks)
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

// run 在独立 goroutine 中执行体系，结束后释放 Tick。
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

// Status 返回调度状态快照与当前是否忙碌（只读观测用）。
func (s *Scheduler[S]) Status() (State, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state, s.busy
}

// Available 报告调度器是否处于空档期（可承接外部推理）。
func (s *Scheduler[S]) Available() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.busy && !s.paused
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

// WaitIdle 等待当前运行中的一轮结束；超时返回 false。
func (s *Scheduler[S]) WaitIdle(timeout time.Duration) bool {
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
		time.Sleep(idlePollInterval)
	}
}

// Stop 取消在跑任务并等待退出。
func (s *Scheduler[S]) Stop() {
	s.cancel()
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
