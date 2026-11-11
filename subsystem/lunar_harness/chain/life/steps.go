package life

// steps.go 生活链每轮固定动作的顺序编排。
//
// 顺序硬编码为：结算愿望 → 更新心情 → 更新对他人态度 → 合并记忆待处理条目。
// 四个动作彼此独立：任一步失败只记录失败并继续后续步骤，
// 避免一次嵌入/数据库抖动同时丢掉本轮的愿望联动、心情、态度与记忆整理推进。

import (
	"context"
	"fmt"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/wishchain"
)

// runFixedSteps 按固定顺序执行四个固定动作，返回失败列表（调用方无需中断 Tick）。
func runFixedSteps(ctx context.Context) []error {
	var failures []error
	exec := func(name string, hook func(context.Context) error) {
		if hook == nil {
			return
		}
		if err := hook(ctx); err != nil {
			logger.Warn(Module, "[生活] 固定步骤「%s」失败，继续后续步骤: %v", name, err)
			failures = append(failures, fmt.Errorf("%s: %w", name, err))
		}
	}
	exec("结算愿望", wishStepHook)
	exec("更新心情", moodStepHook)
	exec("更新对他人态度", affectionStepHook)
	exec("合并记忆待处理条目", memoryPendingStepHook)
	return failures
}

// fixedStepActivity 固定四步结束后写入的状态文案：全部完成与部分失败分开表述。
func fixedStepActivity(failures []error) string {
	if len(failures) == 0 {
		return "刚结算完愿望，整理完心情、对他人的态度和记忆"
	}
	return "想整理愿望、心情和记忆，但没整理完（" + truncateAtSentence(failures[0].Error(), activityReasonLimit) + "）"
}

// settleWishesStep 固定步骤一：把延期 / 临近 / 完成 / 逾期映射为心情偏移，
// 并清除「完成 + 心情结算」都已完成的历史愿望。
func settleWishesStep(_ context.Context) error {
	result := wishchain.Settle(time.Now())
	if len(result.Applied) == 0 && len(result.Removed) == 0 {
		return nil
	}
	logger.Info(Module, "[生活] 愿望结算：联动 %d 项，清除已完成 %d 条", len(result.Applied), len(result.Removed))
	return nil
}

// updateMoodStep 固定步骤二：用最近 5 轮对话更新心情。
// 窗口未前移时跳过，避免生活续跑步骤重复调用嵌入服务。
func updateMoodStep(ctx context.Context) error {
	mu.Lock()
	roundsHook := dialogueRoundsHook
	mu.Unlock()
	if roundsHook == nil {
		return nil
	}
	return wishchain.UpdateMoodFromRounds(ctx, roundsHook())
}

// updateAffectionStep 固定步骤二：按「每个角色最近若干轮」逐人更新情感认知。
// 窗口未前移的角色由引擎内部跳过，不会重复嵌入。
func updateAffectionStep(ctx context.Context) error {
	mu.Lock()
	roundsHook := dialogueRoundsHook
	mu.Unlock()
	if roundsHook == nil {
		return nil
	}
	_, err := wishchain.UpdateAttitudeWindows(ctx, roundsHook())
	return err
}

// mergePendingMemories 固定步骤三：存在待处理条目时直接执行整理，直到本次计划的
// 所有批次完成；无待处理条目时立即返回，不读记忆正文、不产生任何模型调用。
//
// 记忆整理不再注册为权重事件，也不再经过强制事件队列：这里是它唯一的入口，
// 每回合固定步骤只会调用一次，不会出现同一回合排队 + 执行两次的情况。
func mergePendingMemories(ctx context.Context) error {
	if pendingCountHook() <= 0 {
		return nil
	}
	logger.Info(Module, "[生活] 固定步骤三：发现待处理记忆，开始整理")
	return memoryMergeRunHook(ctx)
}

// runMemoryBatches 复用 memory_event.go 的 memoryRun 实现跑完本次计划：
// 先写新条目、校验通过后再删旧，失败由引擎事务回滚。
func runMemoryBatches(ctx context.Context) error {
	run := &memoryRun{}
	if err := run.Start(ctx, Env{}); err != nil {
		return err
	}
	if run.NoWork() {
		return nil
	}
	for {
		step, err := run.Next(ctx, Env{})
		if err != nil {
			return err
		}
		if step.Done {
			if step.Record != "" {
				mu.Lock()
				state.LastRecord = step.Record
				saveStateLocked()
				mu.Unlock()
			}
			return nil
		}
	}
}
