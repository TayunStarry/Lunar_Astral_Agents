package PlanManager

// agent.go 计划管理智能体入口：自然语言指令 → 计划表工具循环 → 结构化结果。
//
// 只负责计划表的新增 / 删除 / 改写 / 查询；不读写文件、不调度其它智能体。
// 循环、重复检测与失败上报统一复用 protocol/agentbase。

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/plans"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// executeMu 串行化计划管理，避免同一计划表并发写入。
var executeMu sync.Mutex

// Execute 执行一次计划管理任务。
func Execute(ctx context.Context, instruction string) (result Result, err error) {
	defer func() { recordRun(instruction, result, err) }()
	instruction = strings.TrimSpace(instruction)
	if instruction == "" {
		return failResult("plan", fmt.Errorf("请用自然语言说明要如何调整计划表")), nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	executeMu.Lock()
	defer executeMu.Unlock()

	base := agentbase.NewAgent(agentbase.AgentConfig{
		ID:                 "plan_manager",
		Label:              roleLabel,
		MaxRounds:          maxRounds,
		MaxRepeats:         maxRepeats,
		MaxToolResultRunes: maxToolResultRunes,
	})
	executed := false
	last := Result{}
	loopResult, err := base.RunToolLoop(ctx, agentbase.ToolLoopRequest{
		Messages: []agentbase.Message{
			{Role: "system", Content: systemPrompt()},
			{Role: "user", Content: instruction + "\n\n【当前时间】" + time.Now().Format("2006-01-02 15:04:05")},
		},
		Tools: toolDefinitions(),
		Stream: agentbase.StreamRequest{
			Agent:   "plan_manager",
			Role:    roleLabel,
			Timeout: modelTimeout,
		},
		Execute: func(_ context.Context, call agentbase.ToolCall) (string, bool, error) {
			outcome := executeTool(call)
			executed = true
			last = outcome
			return outcome.JSON(), false, nil
		},
	})
	if err != nil {
		if errors.Is(err, agentbase.ErrMaxRounds) {
			logger.Warn(Module, "计划管理在 %d 轮内没有完成", maxRounds)
			return failResult("plan", fmt.Errorf("在 %d 轮内没有完成计划操作", maxRounds)), nil
		}
		return Result{}, err
	}
	if !executed {
		return failResult("plan", fmt.Errorf("没有执行任何计划操作")), nil
	}
	if !last.OK {
		return last, nil
	}
	summary := strings.TrimSpace(loopResult.Reply.Text)
	if summary == "" {
		summary = last.Summary
	}
	last.Summary = summary
	logger.Info(Module, "[计划管理] 本轮完成：%s", previewRunes(summary, 512))
	return last, nil
}

// systemPrompt 计划管理智能体的系统提示。
func systemPrompt() string {
	raw := agentbase.PromptText("planManager.md")
	if strings.TrimSpace(raw) == "" {
		return "你是计划管理智能体，只能通过工具新增/删除/改写/查询计划表，不要臆造结果。"
	}
	return raw
}

// Query 直接查询计划表（供测试与内部只读调用）。
func Query(keywords []string, date, scope string) (string, error) {
	return plans.Query(keywords, date, scope)
}
