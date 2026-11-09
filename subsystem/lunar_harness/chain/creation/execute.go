package creation

// execute.go 按调度顺序依次调用专属智能体执行计划。
// 单步出现循环输出或 500 类异常时自动重试一次，仍失败则跳过并继续后续步骤。

import (
	"context"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/agents/CreatorPlanner"
	"LunarSubsystem/LunarHarness/engine/fileflow"
)

// executePlan 顺序执行计划并返回每步结果。
func (s *session) executePlan(ctx context.Context, plan planResult) []stepResult {
	results := make([]stepResult, 0, len(plan.Steps))
	prior := "（无）"
	defer func() { s.current = nil }()
	for i := range plan.Steps {
		step := plan.Steps[i]
		s.stepIndex = i
		s.current = &plan.Steps[i]
		label := agentLabels[step.Agent]
		instruction := fmt.Sprintf(
			"本轮第 %d/%d 步。\n专属智能体：%s\n任务：%s\n前面步骤的结果：%s\n"+
				"请用工具完成本步骤；完成后用中文简要说明产出（文件路径与关键内容），不要臆造结果。",
			i+1, len(plan.Steps), label, step.Task, prior,
		)
		s.report(ProgressEvent{
			Status:   statusExecuting,
			Stage:    fmt.Sprintf("第 %d/%d 步：%s", i+1, len(plan.Steps), label),
			Progress: stepPercent(i, len(plan.Steps)),
		})

		var (
			text    string
			handles []string
			err     error
		)
		if step.Agent == agentPainter {
			text, handles, err = s.runPaintStep(ctx, step)
		} else {
			text, err = s.runStep(ctx, step.Agent, instruction)
		}
		if retryable(err) {
			s.addError(fmt.Errorf("第 %d 步首次执行失败: %w", i+1, err))
			retryInstruction := instruction + "\n\n注意：上一次执行失败（" +
				clipAtSentence(err.Error(), 300) + "）。请换一种方式完成，避免重复相同的工具调用与相同输出。"
			if step.Agent == agentPainter {
				text, handles, err = s.runPaintStep(ctx, step)
			} else {
				text, err = s.runStep(ctx, step.Agent, retryInstruction)
			}
		}
		failures := s.stepReviewFailures(i + 1)
		if err != nil {
			s.addError(fmt.Errorf("第 %d 步[%s]失败并跳过: %w", i+1, label, err))
			s.addMemo("未完成：" + step.Task)
			results = append(results, stepResult{
				Agent:          step.Agent,
				Task:           step.Task,
				Skipped:        true,
				Error:          err.Error(),
				ReviewFailures: failures,
			})
			prior = fmt.Sprintf("第 %d 步（%s）被跳过：%s", i+1, label, previewRunes(err.Error(), 512))
			prior = appendReviewFailures(prior, failures)
			prior = appendFlowDiffs(prior, s.flowDiffs(i+1))
			continue
		}
		text = strings.TrimSpace(text)
		if text == "" {
			text = "已完成本步骤的工具操作"
		}
		results = append(results, stepResult{
			Agent:          step.Agent,
			Task:           step.Task,
			Result:         text,
			Handles:        handles,
			ReviewFailures: failures,
		})
		prior = fmt.Sprintf("第 %d 步（%s）完成：%s", i+1, label, clipAtSentence(text, 400))
		prior = appendReviewFailures(prior, failures)
		prior = appendFlowDiffs(prior, s.flowDiffs(i+1))
	}
	return results
}

// flowDiffs 返回本流程截至指定步骤的内容级操作差异（来自增量库，不依赖自然语言摘要）。
func (s *session) flowDiffs(uptoStep int) string {
	if s.flow == nil {
		return ""
	}
	ops, err := fileflow.FlowOps(s.flow.FlowID)
	if err != nil {
		return ""
	}
	filtered := make([]fileflow.OpRecord, 0, len(ops))
	for _, op := range ops {
		if op.Step > 0 && op.Step > uptoStep {
			continue
		}
		filtered = append(filtered, op)
	}
	if len(filtered) == 0 {
		return ""
	}
	if len(filtered) > maxContextOperations {
		filtered = filtered[len(filtered)-maxContextOperations:]
	}
	return formatOpRecords(filtered)
}

// appendFlowDiffs 把内容级差异并入下一节点可见的前序结果。
func appendFlowDiffs(prior, diffs string) string {
	if strings.TrimSpace(diffs) == "" {
		return prior
	}
	return prior + "\n本流程已执行操作及其内容差异：\n" + diffs
}

// stepReviewFailures 收集指定步骤内审查失败并回滚的上下文。
func (s *session) stepReviewFailures(step int) []string {
	if s.flow == nil {
		return nil
	}
	out := make([]string, 0)
	for _, record := range s.flow.Records {
		if record.Step != step || record.Review == nil || record.Review.OK {
			continue
		}
		text := record.Review.Context
		if text == "" {
			text = record.Review.Summary()
		}
		out = append(out, text)
	}
	return out
}

// appendReviewFailures 把审查失败上下文并入下一节点可见的前序结果。
func appendReviewFailures(prior string, failures []string) string {
	if len(failures) == 0 {
		return prior
	}
	return prior + "\n校验失败并已回滚：" + strings.Join(failures, "；")
}

// runStep 用指定专属智能体执行一步。
func (s *session) runStep(ctx context.Context, agent, instruction string) (string, error) {
	prompt := CreatorPlanner.ExecutorPrompt(agent)
	tools := definitionsFor(toolsFor(agent))
	handler := func(name string, args map[string]any) (toolOutcome, bool, error) {
		return s.executeTool(ctx, name, args), false, nil
	}
	return s.runToolLoop(ctx, agentLabels[agent], prompt, instruction, tools, handler)
}

// summarizeSteps 汇总本轮执行结果，作为生活事件记录。
// 每个步骤用「简述」而不是硬截断：优先保留完整句子，超长才在标点处收尾。
func summarizeSteps(results []stepResult, plan planResult) string {
	if len(results) == 0 {
		return "创作没有产生可执行的步骤"
	}
	done := 0
	var b strings.Builder
	for i, result := range results {
		if result.Skipped {
			fmt.Fprintf(&b, "%d.[%s] 跳过：%s\n", i+1, agentLabels[result.Agent], clipAtSentence(result.Error, 240))
			continue
		}
		done++
		fmt.Fprintf(&b, "%d.[%s] %s\n", i+1, agentLabels[result.Agent], clipAtSentence(result.Result, 240))
	}
	prefix := fmt.Sprintf("本轮创作 %d 步，完成 %d 步：\n", len(plan.Steps), done)
	return clipAtSentence(prefix+b.String(), maxSummaryRunes)
}

// creativeActivity 生成一句「刚刚在做什么」的人话，不复述步骤明细。
//
// 活动状态是给人格对话与用户看的，不是执行日志：优先用选题时写好的自然描述，
// 全部步骤被跳过或没有可执行步骤时如实收口，避免把「6 步完成」当成成果。
func creativeActivity(planned *PlanItem, results []stepResult) string {
	if planned == nil {
		return "完成了一次创作"
	}
	base := strings.TrimPrefix(strings.TrimSpace(planned.Activity), "刚")
	if base == "" {
		base = "完成了一次创作"
	}
	if len(results) == 0 {
		return "想创作，但这次没排上可执行的步骤"
	}
	done := countDone(results)
	switch {
	case done == 0:
		return "想创作，但这次没做成"
	case done < len(results):
		return base + "，但有几项没完成"
	default:
		return base
	}
}

// countDone 统计未被跳过的步骤数。
func countDone(results []stepResult) int {
	done := 0
	for _, result := range results {
		if !result.Skipped {
			done++
		}
	}
	return done
}

// mergeMemos 合并计划备忘与执行遗留备忘，去重并限长。
func mergeMemos(plan planResult, extra []string) []string {
	out := make([]string, 0, maxMemos)
	seen := map[string]bool{}
	for _, memo := range append(append([]string{}, plan.Memos...), extra...) {
		memo = strings.TrimSpace(memo)
		if memo == "" || seen[memo] {
			continue
		}
		seen[memo] = true
		out = append(out, clipAtSentence(memo, maxMemoRunes))
		if len(out) >= maxMemos {
			break
		}
	}
	return out
}
