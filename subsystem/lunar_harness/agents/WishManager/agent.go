package WishManager

// agent.go 愿望管理智能体入口：自然语言指令 → 愿望工具循环 → 结构化结果。
//
// 只负责愿望的新增 / 改写 / 完成 / 删除 / 优先级 / 查询；不读写其它文件、
// 不调度其它智能体。循环、轮次上限与失败上报统一复用 protocol/agentbase。

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"LunarSubsystem/LunarHarness/chain/wishchain"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// executeMu 串行化愿望管理，避免同一状态并发写入。
var executeMu sync.Mutex

// Execute 执行一次愿望管理任务。
func Execute(ctx context.Context, instruction string) (result Result, err error) {
	defer func() { agentbase.ReportRun(AgentID, err) }()
	defer func() { recordRun(instruction, result, err) }()
	instruction = strings.TrimSpace(instruction)
	if instruction == "" {
		return failResult("wish", fmt.Errorf("请用自然语言说明要如何调整愿望")), nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	executeMu.Lock()
	defer executeMu.Unlock()

	base := agentbase.NewAgent(agentbase.AgentConfig{
		ID:                 AgentID,
		Label:              roleLabel,
		MaxRounds:          maxRounds,
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
			Agent:       AgentID,
			Role:        roleLabel,
			Timeout:     modelTimeout,
			ReportTools: true,
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
			logger.Warn(Module, "愿望管理在 %d 轮内没有完成", maxRounds)
			return failResult("wish", fmt.Errorf("在 %d 轮内没有完成愿望操作", maxRounds)), nil
		}
		return Result{}, err
	}
	if !executed {
		return failResult("wish", fmt.Errorf("没有执行任何愿望操作")), nil
	}
	if !last.OK {
		return last, nil
	}
	summary := strings.TrimSpace(loopResult.Reply.Text)
	if summary == "" {
		summary = last.Summary
	}
	last.Summary = summary
	logger.Info(Module, "[愿望管理] 本轮完成：%s", previewRunes(summary, 512))
	return last, nil
}

// systemPrompt 愿望管理智能体的系统提示。
func systemPrompt() string {
	raw := agentbase.PromptText("wishManager.md")
	if strings.TrimSpace(raw) == "" {
		raw = "你是愿望管理智能体，只能通过工具新增/改写/完成/删除/调整优先级/查询愿望，不要臆造结果。"
	}
	return raw + "\n\n【当前愿望清单】\n" + wishchain.FormatList()
}

// Query 直接查询愿望（供测试与内部只读调用）。
func Query(keywords []string) string {
	return wishchain.Query(keywords)
}
