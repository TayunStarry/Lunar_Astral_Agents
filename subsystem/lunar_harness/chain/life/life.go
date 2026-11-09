package life

// life.go 一次生活 Tick：推进回合的一个阶段，或续跑未完成事件。
//
// 事件执行期间不持有 life 锁：模型调用可能持续数分钟，
// 持有锁会让对话运行时消息读取生活快照被拖住。

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// Step 由 Tick 调度器调用：每次只执行一个原子阶段。
func Step(ctx context.Context) error {
	now := time.Now()
	mu.Lock()
	ensureDateLocked()
	active := state.Active != nil
	mu.Unlock()
	if active {
		return continueActiveEvent(ctx, now)
	}
	return advanceRound(ctx, now)
}

// continueActiveEvent 续跑未完成事件。
func continueActiveEvent(ctx context.Context, now time.Time) error {
	mu.Lock()
	// 进程可能在「选中事件」与「清空回合」之间退出，这里补写回合摘要。
	if state.Round != nil {
		closeRoundLocked(state.Active.Event)
	}
	event := eventByIDLocked(state.Active.Event)
	if event == nil {
		logger.Warn(Module, "[生活] 未完成事件 %s 已不存在，清空断点", state.Active.Event)
		state.Active = nil
		saveStateLocked()
		mu.Unlock()
		setActivity("刚才想做的事已经不存在了，先歇一会儿")
		return nil
	}
	stepNo := state.Active.Step
	data := append(json.RawMessage(nil), state.Active.Data...)
	env := envFromStateLocked(now)
	mu.Unlock()

	run := event.NewRun()
	if stepNo == 0 && len(data) == 0 {
		// 进程在 Start 阶段崩溃：断点只有事件名，没有准备数据，重新准备一次。
		if err := run.Start(ctx, env); err != nil {
			failActive(event.ID(), fmt.Errorf("恢复事件准备失败: %w", err))
			return err
		}
	} else if err := run.Restore(data); err != nil {
		failActive(event.ID(), fmt.Errorf("恢复事件断点失败: %w", err))
		return err
	}
	logger.Info(Module, "[生活] 续跑事件 %s，第 %d 步", event.ID(), stepNo+1)
	return runOneEventStep(ctx, env, event, run, stepNo)
}

// startEventFromPick 回合第 5 步：判定愿望 → 权重选择（或强制事件）→ 启动事件并走第一步。
func startEventFromPick(ctx context.Context, now time.Time) error {
	summary := judgeWishes(now)

	mu.Lock()
	if summary != state.WishSummary {
		state.WishSummary = summary
		saveStateLocked()
	}
	env := envFromStateLocked(now)
	// 工具触发的强制事件：跳过权重选取，直接进入指定流程。
	event := takeForcedEventLocked(&env)
	if event == nil {
		event = pickLocked()
	}
	if event == nil {
		closeRoundLocked("")
		mu.Unlock()
		setActivity("这一轮没挑到合适的事做，先歇一会儿")
		logger.Warn(Module, "[生活] 没有可用事件，本次跳过")
		return nil
	}
	state.Used[event.ID()]++
	triggered := state.Used[event.ID()]
	state.Active = &ActiveRun{Event: event.ID(), Step: 0, StartedAt: now.Unix()}
	saveStateLocked()
	closeRoundLocked(event.ID())
	mu.Unlock()

	run := event.NewRun()
	if err := run.Start(ctx, env); err != nil {
		failActive(event.ID(), err)
		return err
	}
	mu.Lock()
	if state.Active != nil && state.Active.Event == event.ID() {
		state.Active.Data = run.Snapshot()
		saveStateLocked()
	}
	mu.Unlock()
	logger.Info(Module, "[生活] 开始事件 %s，今日已触发 %d 次", event.ID(), triggered)

	return runOneEventStep(ctx, env, event, run, 0)
}

// runOneEventStep 执行事件的下一步并推进断点。
// 这里是状态字段的兜底写入点：分支文案（ActivityRun）优先，其次步骤记录。
func runOneEventStep(ctx context.Context, env Env, event Event, run EventRun, stepNo int) error {
	step, err := run.Next(ctx, env)
	if err != nil {
		failActive(event.ID(), err)
		return err
	}
	text := activityTextOf(run, step)
	if !step.Done {
		mu.Lock()
		if state.Active != nil {
			state.Active.Step++
			state.Active.Data = run.Snapshot()
			saveStateLocked()
		}
		mu.Unlock()
		setActivity(text)
		logger.Info(Module, "[生活] 事件 %s 第 %d 步完成：%s", event.ID(), stepNo+1, step.Detail)
		return nil
	}

	mu.Lock()
	state.LastRecord = step.Record
	state.Active = nil
	saveStateLocked()
	mu.Unlock()
	setActivity(text)
	logger.Info(Module, "[生活] 事件 %s 完成，记录=%s", event.ID(), step.Record)
	return nil
}

// closeRoundLocked 结束当前回合并写入可观测摘要；调用方需持有锁。
func closeRoundLocked(branch string) {
	if state.Round == nil {
		return
	}
	state.LastRound = &RoundSummary{
		StartedAt:  state.Round.StartedAt,
		FinishedAt: time.Now().Unix(),
		Branch:     branch,
		Steps:      state.Round.Steps,
	}
	state.Round = nil
	saveStateLocked()
	logger.Info(Module, "[生活] 回合结束，分支=%s，阶段=%d 个", emptyAs(branch, "无"), len(state.LastRound.Steps))
}

// takeForcedEventLocked 取出并清空强制事件；无强制事件时返回 nil（调用方回退权重选择）。
func takeForcedEventLocked(env *Env) Event {
	if strings.TrimSpace(state.PendingEvent) == "" {
		return nil
	}
	id := state.PendingEvent
	env.Instruction = state.PendingInstruction
	state.PendingEvent = ""
	state.PendingInstruction = ""
	saveStateLocked()
	event := eventByIDLocked(id)
	if event == nil {
		logger.Warn(Module, "[生活] 强制事件 %s 未注册，回退到权重选择", id)
		return nil
	}
	logger.Info(Module, "[生活] 跳过权重选择，优先执行事件 %s", id)
	return event
}

func failActive(eventID string, err error) {
	record := failureRecord(eventID)
	text := failureActivity(eventID, err)
	mu.Lock()
	state.LastRecord = record
	state.Active = nil
	saveStateLocked()
	mu.Unlock()
	setActivity(text)
	logger.Warn(Module, "[生活] 事件 %s 失败，已按「%s」记录: %v", eventID, record, err)
}

func failureRecord(eventID string) string {
	switch eventID {
	case "forum":
		return "逛了论坛（未完成）"
	case "watch":
		return "看视频没看成（未完成）"
	default:
		return "发呆中"
	}
}
