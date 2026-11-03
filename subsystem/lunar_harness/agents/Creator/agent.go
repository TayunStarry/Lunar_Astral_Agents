package Creator

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

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/fileflow"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// TaskResult 一次指派任务的执行结果。
type TaskResult struct {
	// Summary 本轮结果简述
	Summary string
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

// RunWithReference 执行一轮自由创作，reference 为本轮创作参考（来自 life_planning）。
func RunWithReference(ctx context.Context, instruction, reference string) (string, error) {
	result, err := runFlow(ctx, fileflow.ModeCreator, instruction, reference, nil)
	if err != nil {
		return "", err
	}
	return result.Summary, nil
}

// RunAssigned 执行一次对话者指派的任务：只带入历史操作记录，其余上下文不进入本轮。
// handles 为对话者消息中真实出现的资源句柄，会附在指令中供流程节点定位目标。
func RunAssigned(ctx context.Context, instruction string, handles []string) (TaskResult, error) {
	return runFlow(ctx, fileflow.ModeAssigned, instruction, "", handles)
}

// runFlow 两种模式共用的流程执行主体。
func runFlow(ctx context.Context, mode fileflow.Mode, instruction, reference string, handles []string) (TaskResult, error) {
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

	flowLabel := "文件操作"
	if mode == fileflow.ModeCreator {
		flowLabel = "自由创作"
	}

	// 只保留最近若干流程快照目录，避免长期占用磁盘；增量库按流程数 / 时间清理
	fileflow.CleanupFlows(0)
	if err := fileflow.CleanupDeltas(0, 0); err != nil {
		logger.Warn(Module, "[%s] 清理增量记录失败: %v", flowLabel, err)
	}

	path := journalPath(mode)
	history := loadJournal(path)
	recentOps, err := fileflow.RecentOps(maxOperationHistory)
	if err != nil {
		logger.Warn(Module, "[%s] 读取增量记录失败: %v", flowLabel, err)
	}
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
		mode:      mode,
		flowID:    flowID,
		flow:      fileflow.NewContext(mode, flowID),
		stepIndex: -1,
	}
	plan, err := runSession.planRound(runCtx, instruction, history, recentOps)
	if err != nil {
		runSession.addError(err)
		history.Errors = runSession.errs
		history.Mode = string(mode)
		history.UpdatedAt = nowUnix()
		saveJournal(path, pruneJournal(history, mode))
		runSession.writeTrace(instruction, planResult{}, nil, "规划失败："+err.Error())
		logger.Warn(Module, "[%s] 规划失败: %v", flowLabel, err)
		// 交给智能修复：规划失败常由参考上下文过长/损坏引起
		agentbase.ReportRun("creative", err)
		return TaskResult{}, err
	}

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
	saveJournal(path, pruneJournal(next, mode))
	runSession.writeTrace(instruction, plan, results, summary)
	logger.Info(Module, "[%s] 本轮完成：%d 步规划，%d 条操作记录，%d 个错误",
		flowLabel, len(plan.Steps), len(runSession.flowRecords()), len(runSession.errs))
	return runSession.taskResult(summary), nil
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

// writeTrace 把完整流程记录写入快照目录（两种模式都写，供追溯；不参与下一轮上下文）。
func (s *session) writeTrace(instruction string, plan planResult, results []stepResult, summary string) {
	if s.flow == nil {
		return
	}
	trace := map[string]any{
		"mode":        s.flow.Mode,
		"flow_id":     s.flow.FlowID,
		"instruction": instruction,
		"plan":        plan,
		"results":     results,
		"operations":  s.flow.Records,
		"errors":      s.flow.Errors,
		"summary":     summary,
	}
	if err := fileflow.WriteTrace(s.flow.FlowID, trace); err != nil {
		logger.Warn(Module, "[自由创作] 流程追踪写入失败: %v", err)
	}
}
