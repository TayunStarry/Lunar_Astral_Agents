package lunar

// tick_scheduler.go 统一 Tick 调度器。
//
// 规则：
//  1. 任一体系运行中（busy=true）→ 当前 Tick 直接跳过，不计数；
//  2. 无未读、无待处理视频切片、生活冷却未完成 → 空闲计数 +1；
//  3. 未读消息 → 对话体系，计数清零；
//  4. 待处理视频切片 → 观影体系，计数清零；
//  5. 空闲计数达到生活阈值 → 生活体系，计数清零。
//
// 体系通过 TickSystem 接口注册，主调度逻辑不感知具体实现。

import (
	"context"
	"encoding/json"
	"sort"
	"sync"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/persistence"
)

type tickScheduler struct {
	mu      sync.Mutex
	systems []TickSystem
	state   TickState
	busy    bool
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
	}
	for _, system := range s.systems {
		ready, reason := system.Ready(snap)
		if !ready {
			continue
		}
		s.busy = true
		s.state.IdleTicks = 0
		s.state.LastSystem = system.ID()
		s.state.LastReason = reason
		s.state.LastTriggerAt = snap.Now.Unix()
		s.state.UpdatedAt = snap.Now.Unix()
		s.tickSeq++
		seq := s.tickSeq
		s.persistLocked()
		s.mu.Unlock()
		logger.Info(tickModule, "[Tick#%d] 触发 %s（%s），空闲计数清零", seq, system.ID(), reason)
		s.run(seq, system, snap.Now)
		return
	}
	s.state.IdleTicks++
	if s.state.IdleTicks%60 == 0 {
		logger.Info(tickModule, "[Tick] 空闲计数 %d/%d", s.state.IdleTicks, tickIdleThreshold)
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
		}
	}()
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
	if state.IdleTicks < 0 {
		state.IdleTicks = 0
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
