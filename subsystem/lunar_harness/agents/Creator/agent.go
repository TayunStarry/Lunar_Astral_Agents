package Creator

// agent.go 创作事件对外入口：规划日志 → 调度编排 → 顺序执行 → 归档日志。

import (
	"context"
	"errors"
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// Run 执行一轮完整创作：读取历史规划日志，由调度者编排，再按顺序调用专属智能体。
func Run(ctx context.Context, instruction string) (string, error) {
	instruction = strings.TrimSpace(instruction)
	if instruction == "" {
		return "", errors.New("请用自然语言说明本次创作要做什么")
	}

	runMu.Lock()
	defer runMu.Unlock()

	runCtx, cancel := context.WithTimeout(ctx, runTimeout)
	defer cancel()

	history := loadJournal()
	runSession := &session{}
	plan, err := runSession.planRound(runCtx, instruction, history)
	if err != nil {
		runSession.addError(err)
		history.Errors = runSession.errs
		history.UpdatedAt = nowUnix()
		saveJournal(history)
		logger.Warn(Module, "[自由创作] 规划失败: %v", err)
		return "", err
	}

	results := runSession.executePlan(runCtx, plan)
	summary := summarizeSteps(results, plan)
	saveJournal(journal{
		Version:     1,
		LastPlans:   plan.Steps,
		Memos:       mergeMemos(plan, runSession.memos),
		References:  append([]string(nil), runSession.refs...),
		Errors:      append([]string(nil), runSession.errs...),
		LastSummary: summary,
	})
	logger.Info(Module, "[自由创作] 本轮完成：%d 步规划，%d 条参考资料，%d 条错误",
		len(plan.Steps), len(runSession.refs), len(runSession.errs))
	return summary, nil
}
