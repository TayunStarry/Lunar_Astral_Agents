package creation

// journal.go 跨轮信息视图：从统一库 local_data/database/creation.db 读取上一轮调用记录，
// 按模式构造可注入提示的历史信息，并在本轮结束时归档。
//
// 两种留存规则：
//   - 自由创作模式：references / memos / steps_free / errors_free + 上一轮 change 全部保留；
//   - 指派模式：只保留 steps_assign / errors_assign 与 change，参考资料与备忘录不落库。
//
// 文件变更一律是内容级增量（change 表），不保存自然语言流水账。

import (
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/fileflow"
)

// loadRoundJournal 读取指定模式上一轮已完成调用，构建成调度者可读的日志视图。
func loadRoundJournal(mode fileflow.Mode) journal {
	prev, err := fileflow.PrevRound(mode)
	if err != nil {
		logger.Warn(Module, "[%s] 读取上一轮记录失败: %v", mode, err)
		return journal{Version: 1, Mode: string(mode)}
	}
	return journalFromRound(prev)
}

// journalFromRound 把库中的上一轮快照转换为日志视图。
func journalFromRound(prev fileflow.RoundSnapshot) journal {
	steps := make([]planStep, 0, len(prev.Steps))
	for _, item := range prev.Steps {
		steps = append(steps, planStep{Agent: item.Agent, Task: item.Task, Expect: item.Expects})
	}
	return journal{
		Version:     1,
		Mode:        string(prev.Call.Mode),
		LastPlans:   steps,
		Memos:       prev.Memos,
		References:  prev.References,
		Errors:      prev.Errors,
		LastSummary: prev.Call.Summary,
		Changes:     prev.Changes,
		UpdatedAt:   prev.Call.EndedAt,
	}
}

// saveRoundJournal 归档本轮：自由创作写入全部字段，指派模式只写步骤与错误。
func saveRoundJournal(callID string, mode fileflow.Mode, j journal) {
	if callID == "" {
		return
	}
	steps := make([]fileflow.RoundStep, 0, len(j.LastPlans))
	for i, step := range j.LastPlans {
		steps = append(steps, fileflow.RoundStep{Ordinal: i, Agent: step.Agent, Task: step.Task, Expects: step.Expect})
	}
	refs, memos := []string(nil), []string(nil)
	if mode == fileflow.ModeCreator {
		refs, memos = j.References, j.Memos
	}
	if err := fileflow.FinishRound(callID, mode, fileflow.CallStatusDone, j.LastSummary, steps, refs, memos, j.Errors); err != nil {
		logger.Warn(Module, "[%s] 轮次归档失败: %v", mode, err)
	}
}

// saveRoundFailure 归档一轮失败调用：只写错误与状态，不落参考资料 / 备忘录。
func saveRoundFailure(callID string, mode fileflow.Mode, errs []string) {
	if callID == "" {
		return
	}
	if err := fileflow.FinishRound(callID, mode, fileflow.CallStatusFailed, "", nil, nil, nil, errs); err != nil {
		logger.Warn(Module, "[%s] 失败轮次归档失败: %v", mode, err)
	}
}

// formatJournal 把日志视图整理成调度者可读的历史信息（按模式区分可见范围）。
func formatJournal(j journal) string {
	if j.Mode == string(fileflow.ModeAssigned) {
		var b strings.Builder
		b.WriteString("【上一轮指派任务的步骤】\n")
		writeSteps(&b, j.LastPlans)
		b.WriteString("\n【上一轮指派任务的错误与异常】\n")
		writeList(&b, j.Errors)
		b.WriteString("\n【文件变更】\n可用 change_query 按路径查看任意文件的内容级变更（最近 10 次与更早合并的原始基准）。\n")
		return b.String()
	}
	var b strings.Builder
	b.WriteString("【上一轮规划】\n")
	writeSteps(&b, j.LastPlans)
	b.WriteString("\n【上一轮遗留的备忘与期望】\n")
	writeList(&b, j.Memos)
	b.WriteString("\n【上一轮查询到的参考资料】\n")
	writeList(&b, j.References)
	b.WriteString("\n【上一轮的错误与异常】\n")
	writeList(&b, j.Errors)
	b.WriteString("\n【上一轮文件变更的内容差异】\n")
	b.WriteString(formatChanges(j.Changes))
	if strings.TrimSpace(j.LastSummary) != "" {
		b.WriteString("\n【上一轮收尾】\n")
		b.WriteString(clipAtSentence(j.LastSummary, maxSummaryRunes))
		b.WriteByte('\n')
	}
	return b.String()
}

// writeSteps 输出规划步骤。
func writeSteps(b *strings.Builder, steps []planStep) {
	if len(steps) == 0 {
		b.WriteString("（空）\n")
		return
	}
	for i, step := range steps {
		fmt.Fprintf(b, "%d. [%s] %s\n", i+1, agentLabels[step.Agent], step.Task)
		if len(step.Expect) > 0 {
			fmt.Fprintf(b, "   预期：%s\n", formatExpects(step.Expect))
		}
	}
}
