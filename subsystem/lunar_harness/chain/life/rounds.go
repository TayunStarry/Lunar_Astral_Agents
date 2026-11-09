package life

// rounds.go 生活回合的固定顺序编排。
//
// 一个回合固定五步，顺序不可打乱：
//  1. 固定四步（结算愿望 → 更新心情 → 更新对他人态度 → 合并记忆待处理条目）
//  2. viewer.EnsureWatchlist
//  3. viewer.AutoRespondOnBilibili
//  4. MutualForum.AutoRespondUnread
//  5. 权重选择，进入 看视频 / 逛论坛 / 创作 等分支
//
// 一次 Tick 只推进一个阶段：阶段之间天然是让出点，用户消息到达时对话链优先回应；
// 第 2~4 步是机会性维护，任一步失败或条件不满足都只记录，不中断后续阶段。

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/agents/MutualForum"
	"LunarSubsystem/LunarHarness/chain/viewer"
	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// advanceRound 推进回合的当前阶段（一次只走一个阶段）。
func advanceRound(ctx context.Context, now time.Time) error {
	mu.Lock()
	if state.Round == nil {
		state.Round = &RoundState{Stage: roundStageFixed, StartedAt: now.Unix()}
		saveStateLocked()
	}
	stage := state.Round.Stage
	mu.Unlock()

	// 机会性维护之间的让出点：用户消息优先，阶段不前进，下次 Tick 从同一步继续。
	if stage != roundStageFixed && stage != roundStagePick && pauseHook != nil && pauseHook() {
		logger.Info(Module, "[生活] 用户消息到达，回合在步骤「%s」前让出", stage)
		return nil
	}

	switch stage {
	case roundStageFixed:
		failures := runFixedSteps(ctx)
		recordRoundStage(RoundStep{
			Name:   roundStageFixed,
			Status: statusFromFailures(failures),
			Detail: detailFromFailures(failures),
		})
		setActivity(fixedStepActivity(failures))
		setRoundStage(roundStageWatchlist)
	case roundStageWatchlist:
		recordRoundStage(runWatchlistStage(ctx))
		setRoundStage(roundStageBiliReply)
	case roundStageBiliReply:
		recordRoundStage(runBiliReplyStage(ctx))
		setRoundStage(roundStageForumReply)
	case roundStageForumReply:
		recordRoundStage(runForumReplyStage(ctx))
		setRoundStage(roundStagePick)
	case roundStagePick:
		recordRoundStage(RoundStep{Name: roundStagePick, Status: roundStatusOK, Detail: "进入权重选择"})
		return startEventFromPick(ctx, now)
	default:
		mu.Lock()
		closeRoundLocked("")
		mu.Unlock()
		logger.Warn(Module, "[生活] 未知回合阶段 %q，已重置", stage)
	}
	return nil
}

// runWatchlistStage 第 2 步：待看列表为空时补齐候选、审核、入库与切片下载。
// 成功、失败、跳过都落到状态字段：成功写这次补了什么，失败写「没做成」，跳过写「做不了」与原因。
func runWatchlistStage(ctx context.Context) RoundStep {
	res, err := ensureWatchlistHook(ctx)
	step := RoundStep{Name: roundStageWatchlist}
	okText := ""
	switch {
	case err != nil:
		step.Status, step.Detail = roundStatusFailed, err.Error()
		logger.Warn(Module, "[生活] 待看列表维护失败，继续后续步骤: %v", err)
	case !res.Triggered:
		step.Status, step.Detail = roundStatusSkipped, emptyAs(res.Reason, "触发条件不满足")
	default:
		step.Status = roundStatusOK
		step.Detail = fmt.Sprintf("候选 %d，过审 %d，入队 %d，下载 %d", res.Candidates, res.Approved, res.Enqueued, res.Downloaded)
		okText = watchlistActivityText(res)
	}
	setActivity(stageActivity(step, okText))
	return step
}

// runBiliReplyStage 第 3 步：回应 B 站 @ 与自己的视频/动态评论区。
func runBiliReplyStage(ctx context.Context) RoundStep {
	res, err := biliAutoRespondHook(ctx)
	step := RoundStep{Name: roundStageBiliReply}
	okText := ""
	switch {
	case err != nil:
		step.Status, step.Detail = roundStatusFailed, err.Error()
		logger.Warn(Module, "[生活] B 站自动回应失败，继续后续步骤: %v", err)
	case !res.Triggered:
		step.Status, step.Detail = roundStatusSkipped, emptyAs(res.Reason, "触发条件不满足")
	default:
		step.Status = roundStatusOK
		step.Detail = fmt.Sprintf("@ %d 条，视频评论 %d 条，动态评论 %d 条",
			res.At.Replied, res.Video.Replied, res.Dynamic.Replied)
		okText = viewer.AutoRespondActivityText(res)
	}
	setActivity(stageActivity(step, okText))
	return step
}

// runForumReplyStage 第 4 步：回应互助论坛的未读消息。
func runForumReplyStage(ctx context.Context) RoundStep {
	res, err := forumAutoRespondHook(ctx)
	step := RoundStep{Name: roundStageForumReply}
	switch {
	case err != nil && errors.Is(err, MutualForum.ErrForumPageUnavailable):
		step.Status, step.Detail = roundStatusSkipped, "论坛页面打不开："+err.Error()
	case err != nil:
		step.Status, step.Detail = roundStatusFailed, err.Error()
	case !res.PageOpen:
		step.Status, step.Detail = roundStatusSkipped, "论坛页面未开启"
	case res.Pending == 0:
		step.Status, step.Detail = roundStatusSkipped, "没有未读回应"
	default:
		step.Detail = fmt.Sprintf("待回应 %d，成功 %d，失败 %d", res.Pending, res.Responded, res.Failed)
		if res.Responded > 0 {
			step.Status = roundStatusOK
		} else {
			step.Status = roundStatusFailed
		}
	}
	if err != nil {
		logger.Warn(Module, "[生活] 论坛自动回应失败，继续后续步骤: %v", err)
	}
	setActivity(stageActivity(step, MutualForum.ReplyActivityText(res)))
	return step
}

// stageActivity 第 2~4 步的状态文案：成功用分支文案，失败写「没做成」，跳过写「做不了」与原因。
// 复用 failureActivity，与生活事件失败 / 早退的措辞保持同一套。
func stageActivity(step RoundStep, okText string) string {
	switch step.Status {
	case roundStatusOK:
		return okText
	case roundStatusSkipped:
		return failureActivity(step.Name, skip(step.Detail))
	default:
		return failureActivity(step.Name, errors.New(step.Detail))
	}
}

// watchlistActivityText 第 2 步成功时的状态文案：补到了什么就写什么，一个都没留下也如实写。
func watchlistActivityText(res viewer.WatchlistResult) string {
	if res.Enqueued == 0 && res.Downloaded == 0 {
		return fmt.Sprintf("刚去补待看列表，筛了 %d 个候选都没留下", res.Candidates)
	}
	return fmt.Sprintf("刚补齐待看列表，新入队 %d 个、下载好 %d 个", res.Enqueued, res.Downloaded)
}

// recordRoundStage 记录当前阶段结果。
func recordRoundStage(step RoundStep) {
	mu.Lock()
	defer mu.Unlock()
	if state.Round == nil {
		return
	}
	state.Round.Steps = append(state.Round.Steps, step)
	saveStateLocked()
}

// setRoundStage 推进到下一个阶段。
func setRoundStage(stage string) {
	mu.Lock()
	defer mu.Unlock()
	if state.Round == nil {
		return
	}
	state.Round.Stage = stage
	saveStateLocked()
}

// statusFromFailures 固定四步任一失败记为 failed，但回合继续。
func statusFromFailures(failures []error) string {
	if len(failures) == 0 {
		return roundStatusOK
	}
	return roundStatusFailed
}

// detailFromFailures 汇总固定四步的失败原因。
func detailFromFailures(failures []error) string {
	if len(failures) == 0 {
		return "愿望 / 心情 / 态度 / 记忆待合并 均已完成"
	}
	parts := make([]string, 0, len(failures))
	for _, err := range failures {
		parts = append(parts, err.Error())
	}
	return strings.Join(parts, "；")
}
