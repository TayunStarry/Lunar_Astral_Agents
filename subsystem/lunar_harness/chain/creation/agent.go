package creation

// agent.go 流程对外入口：规划日志 → 调度编排 → 路由执行 → 审查/回滚 → 归档日志。
//
// 两个入口：
//   - Run：生活体系触发的自由创作模式，完整流程记录全程保存；
//   - RunAssigned：对话者指派的单次任务，只保留历史操作记录供下一次任务使用。

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/engine/activity"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/fileflow"
)

// TaskResult 一次指派任务的执行结果。
type TaskResult struct {
	// Summary 本轮结果简述（步骤级明细，供日志与跨轮记录使用）
	Summary string
	// Activity 一句人话，描述「刚刚在做什么」，供生活活动状态直接展示
	Activity string
	// Topic 本轮自主创作选中的事项标题，用于回写愿望完成 / 逾期
	Topic string
	// Failed 本轮是否整体失败（规划失败或全部步骤被跳过）
	Failed bool
	// Handles 本次涉及的全部资源句柄
	Handles []string
	// ReviewFailures 审查失败并回滚的上下文
	ReviewFailures []string
	// Rollbacks 回滚次数
	Rollbacks int
}

// Run 执行一轮完整创作：读取历史规划日志，由调度者编排，再按顺序调用专属智能体。
// userTexts 为调用方给出的参考文本；生活创作已改为只传入本轮创作参考字符串。
func Run(ctx context.Context, instruction string, userTexts []string) (string, error) {
	return RunWithReference(ctx, instruction, strings.Join(userTexts, "；"))
}

// RunWithReference 执行一轮自由创作，reference 为本轮创作参考（可空）。
func RunWithReference(ctx context.Context, instruction, reference string) (string, error) {
	result, err := RunWithReferenceResult(ctx, instruction, reference)
	if err != nil {
		return "", err
	}
	return result.Summary, nil
}

// RunWithReferenceResult 执行一轮自由创作，并返回包含活动文案的完整结果。
func RunWithReferenceResult(ctx context.Context, instruction, reference string) (TaskResult, error) {
	return runFlow(ctx, fileflow.ModeCreator, instruction, reference, nil, nil)
}

// RunAssigned 执行一次对话者指派的任务：只带入历史操作记录，其余上下文不进入本轮。
// handles 为对话者消息中真实出现的资源句柄，会附在指令中供流程节点定位目标。
func RunAssigned(ctx context.Context, instruction string, handles []string) (TaskResult, error) {
	return runFlow(ctx, fileflow.ModeAssigned, instruction, "", handles, nil)
}

// RunAssignedWithProgress 执行一次指派任务，并把规划 / 执行进度回调给调用方。
func RunAssignedWithProgress(ctx context.Context, instruction string, handles []string, observe func(ProgressEvent)) (TaskResult, error) {
	return runFlow(ctx, fileflow.ModeAssigned, instruction, "", handles, observe)
}

// RunFreeWithProgress 执行一轮自由创作（随机目标池自主选题），并回调节奏进度。
func RunFreeWithProgress(ctx context.Context, observe func(ProgressEvent)) (TaskResult, error) {
	return runFlow(ctx, fileflow.ModeCreator,
		"本轮没有对话者指派的具体任务，请按系统选定的创作事项完成一次完整创作。", "", nil, observe)
}

// runFlow 两种模式共用的流程执行主体。
func runFlow(ctx context.Context, mode fileflow.Mode, instruction, reference string, handles []string, observe func(ProgressEvent)) (TaskResult, error) {
	instruction = strings.TrimSpace(instruction)
	if instruction == "" {
		return TaskResult{}, errors.New("请用自然语言说明本次要做什么")
	}
	if len(handles) > 0 {
		instruction += "\n\n【目标句柄】" + strings.Join(handles, "、")
	}

	runMu.Lock()
	defer runMu.Unlock()

	runCtx, cancel := context.WithTimeout(ctx, runTimeout)
	defer cancel()

	// 自由创作的事项来源改为自主决定：心情极性决定续做未完成事项还是换新，
	// 具体事项由预设列表随机组合；选题失败时退回调用方原始指令。
	var planned *PlanItem
	topic := ""
	if mode == fileflow.ModeCreator {
		if item, planErr := PlanNext(runCtx); planErr != nil {
			logger.Warn(Module, "[自由创作] 自主选题失败，按原指令继续: %v", planErr)
		} else {
			planned = &item
			topic = item.Title
			instruction = withPlanInstruction(instruction, item)
		}
	}

	flowLabel := "文件操作"
	if mode == fileflow.ModeCreator {
		flowLabel = "自由创作"
	}

	// 只保留最近若干流程快照目录，避免长期占用磁盘；增量库按流程数 / 时间清理
	fileflow.CleanupFlows(0)
	if err := fileflow.CleanupDeltas(0, 0); err != nil {
		logger.Warn(Module, "[%s] 清理增量记录失败: %v", flowLabel, err)
	}

	callID, callErr := fileflow.BeginCall(mode)
	if callErr != nil {
		logger.Warn(Module, "[%s] 创建调用轮次失败（本轮跨轮信息将不落库）: %v", flowLabel, callErr)
	}
	history := loadRoundJournal(mode)
	history.Mode = string(mode)
	if mode == fileflow.ModeCreator {
		// 创作参考是一次性输入：本轮没有新参考时必须清空，避免上一轮已完成的意图被重复执行。
		if strings.TrimSpace(reference) != "" {
			history.Directives = mergeDirectives(nil, []string{reference})
		} else {
			history.Directives = nil
		}
	}

	flowID := fileflow.NewFlowID()
	runSession := &session{
		callID:    callID,
		mode:      mode,
		flowID:    flowID,
		flow:      fileflow.NewContext(mode, flowID),
		stepIndex: -1,
		observe:   observe,
	}
	runSession.report(ProgressEvent{Status: statusPlanning, Stage: "调度者正在生成计划", Progress: 5})
	plan, err := runSession.planRound(runCtx, instruction, history)
	if err != nil {
		runSession.addError(err)
		saveRoundFailure(callID, mode, runSession.errs)
		logger.Warn(Module, "[%s] 规划失败: %v", flowLabel, err)
		// 失败上报由创作规划者智能体内部完成（归属 ID：creator）；
		// 带回选题标题，供生活创作回写愿望逾期。
		return TaskResult{Topic: topic, Failed: true}, err
	}
	runSession.report(ProgressEvent{
		Status:   statusExecuting,
		Stage:    fmt.Sprintf("计划已生成：%d 步", len(plan.Steps)),
		Progress: 20,
	})

	results := runSession.executePlan(runCtx, plan)
	summary := summarizeSteps(results, plan)
	if count := rollbackErrorCount(runSession.flowRecords()); count > 0 {
		summary += fmt.Sprintf("（%d 次文件回滚失败，详见流程日志与快照目录）", count)
	}
	next := journal{
		Version:     1,
		Mode:        string(mode),
		Directives:  history.Directives,
		LastPlans:   plan.Steps,
		Memos:       mergeMemos(plan, runSession.memos),
		References:  append([]string(nil), runSession.refs...),
		Errors:      append([]string(nil), runSession.errs...),
		LastSummary: summary,
	}
	saveRoundJournal(callID, mode, next)
	// 本轮创作完成：标记事项结束，并立即刷新「我刚刚在干的事情」。
	if planned != nil {
		finishPlan(*planned)
	}
	// 活动文案只描述「做了什么」，不复述步骤明细；失败时按真实结果收口。
	// 仅自由创作写活动状态；对话者指派的任务由调用方自行决定展示方式。
	activityText := ""
	failed := false
	if mode == fileflow.ModeCreator {
		failed = countDone(results) == 0
		activityText = creativeActivity(planned, results)
		if activityText != "" {
			activity.Set(activityText)
		}
	}
	logger.Info(Module, "[%s] 本轮完成：%d 步规划，%d 条操作记录，%d 个错误",
		flowLabel, len(plan.Steps), len(runSession.flowRecords()), len(runSession.errs))
	result := runSession.taskResult(summary)
	result.Activity = activityText
	result.Topic = topic
	result.Failed = failed
	return result, nil
}

// flowRecords 返回本次流程的操作记录。
func (s *session) flowRecords() []fileflow.Record {
	if s.flow == nil {
		return nil
	}
	return s.flow.Records
}

// taskResult 汇总指派任务的返回值。
func (s *session) taskResult(summary string) TaskResult {
	result := TaskResult{Summary: clipAtSentence(summary, maxAssignedSummaryRunes)}
	seen := map[string]bool{}
	for _, handle := range s.paintHandles {
		if handle != "" && !seen[handle] {
			seen[handle] = true
			result.Handles = append(result.Handles, handle)
		}
	}
	for _, record := range s.flowRecords() {
		for _, handle := range record.Handles {
			if handle != "" && !seen[handle] {
				seen[handle] = true
				result.Handles = append(result.Handles, handle)
			}
		}
		if record.Review != nil && !record.Review.OK {
			result.ReviewFailures = append(result.ReviewFailures, record.Review.Context)
			if record.Review.RolledBack || record.Review.RollbackError != "" {
				result.Rollbacks++
			}
		}
	}
	return result
}

// rollbackErrorCount 统计回滚失败次数（用于在最终摘要中显式提示）。
func rollbackErrorCount(records []fileflow.Record) int {
	count := 0
	for _, record := range records {
		if record.Review != nil && record.Review.RollbackError != "" {
			count++
		}
	}
	return count
}
