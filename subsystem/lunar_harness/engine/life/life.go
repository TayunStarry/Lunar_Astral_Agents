package life

// life.go 一次生活 Tick：推进未完成事件，或按权重开启新事件。
//
// 事件执行期间不持有 life 锁：模型调用可能持续数分钟，
// 持有锁会让对话运行时消息读取生活快照被拖住。

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// Step 由 Tick 调度器调用：每次只执行一个原子步骤。
func Step(ctx context.Context) error {
	now := time.Now()
	summary := judgePlans(now)

	mu.Lock()
	ensureDateLocked()
	if summary != state.PlanSummary {
		state.PlanSummary = summary
		saveStateLocked()
	}
	env := envFromStateLocked(now)
	var event Event
	var run EventRun
	var stepNo int
	if state.Active == nil {
		event = pickLocked()
		if event == nil {
			mu.Unlock()
			logger.Warn(Module, "[生活] 没有可用事件，本次跳过")
			return nil
		}
		state.Used[event.ID()]++
		triggered := state.Used[event.ID()]
		state.Active = &ActiveRun{Event: event.ID(), Step: 0, StartedAt: now.Unix()}
		saveStateLocked()
		mu.Unlock()

		run = event.NewRun()
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
	} else {
		event = eventByIDLocked(state.Active.Event)
		if event == nil {
			logger.Warn(Module, "[生活] 未完成事件 %s 已不存在，清空断点", state.Active.Event)
			state.Active = nil
			saveStateLocked()
			mu.Unlock()
			return nil
		}
		stepNo = state.Active.Step
		data := append(json.RawMessage(nil), state.Active.Data...)
		mu.Unlock()

		run = event.NewRun()
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
	}

	step, err := run.Next(ctx, env)
	if err != nil {
		failActive(event.ID(), err)
		return err
	}
	if !step.Done {
		mu.Lock()
		if state.Active != nil {
			state.Active.Step++
			state.Active.Data = run.Snapshot()
			saveStateLocked()
		}
		mu.Unlock()
		logger.Info(Module, "[生活] 事件 %s 第 %d 步完成：%s", event.ID(), stepNo+1, step.Detail)
		return nil
	}

	mu.Lock()
	state.LastRecord = step.Record
	state.Active = nil
	saveStateLocked()
	mu.Unlock()
	logger.Info(Module, "[生活] 事件 %s 完成，记录=%s", event.ID(), step.Record)
	return nil
}

func failActive(eventID string, err error) {
	mu.Lock()
	state.LastRecord = failureRecord(eventID)
	state.Active = nil
	saveStateLocked()
	mu.Unlock()
	logger.Warn(Module, "[生活] 事件 %s 失败，已按「%s」记录: %v", eventID, failureRecord(eventID), err)
}

func failureRecord(eventID string) string {
	switch eventID {
	case "watch":
		return "刷了会视频（未完成）"
	case "forum":
		return "逛了论坛（未完成）"
	case "memory":
		return "小憩了片刻（未完成）"
	case "mood":
		return "出门转了转（未完成）"
	default:
		return "发呆中"
	}
}


