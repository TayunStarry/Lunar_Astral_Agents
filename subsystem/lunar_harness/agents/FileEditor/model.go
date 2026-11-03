package FileEditor

// model.go 文件编辑的模型调用：统一走 protocol/agentbase 的调用、实况、
// 循环监管与智能修复；本文件只保留工具循环与结果回填。

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// runToolLoop 工具推理循环：模型通过 function calling 逐步调用工具。
// 返回本轮简述与实际涉及的资源句柄（去重、保持首次出现顺序）。
//
// 循环编排、重复检测与失败上报统一由 protocol/agentbase 的 RunToolLoop 承担。
func runToolLoop(ctx context.Context, systemPrompt, instruction string) (string, []string, error) {
	handles := make([]string, 0, 8)
	handleSeen := map[string]bool{}
	base := agentbase.NewAgent(agentbase.AgentConfig{
		ID:                 "file_editor",
		Label:              Module,
		MaxRounds:          maxRounds,
		MaxRepeats:         maxRepeats,
		MaxToolResultRunes: -1, // 工具结果由本包的截断逻辑处理，保留原有提示文案
	})
	result, err := base.RunToolLoop(ctx, agentbase.ToolLoopRequest{
		Messages: []agentbase.Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: instruction},
		},
		Tools: toolsForRound(),
		Stream: agentbase.StreamRequest{
			Agent:       "file_editor",
			Role:        Module,
			NonStream:   true,
			ReportTools: true,
			Timeout:     modelTimeout,
		},
		Execute: func(ctx context.Context, call agentbase.ToolCall) (string, bool, error) {
			args, parseErr := agentbase.ParseToolArgs(call.Function.Arguments)
			var outcome OpResult
			if parseErr != nil {
				outcome = failResult(call.Function.Name, "", fmt.Errorf("参数不是合法 JSON"))
			} else {
				outcome = dispatchTool(ctx, call.Function.Name, args)
			}
			for _, handle := range outcome.Handles {
				if handle != "" && !handleSeen[handle] {
					handleSeen[handle] = true
					handles = append(handles, handle)
				}
			}
			// 聚合调度结果必须在 JSON 内部截断，避免整串截断破坏统一输出格式
			content := outcome.JSON()
			if outcome.Dispatch != nil {
				content = outcome.Dispatch.Truncate(toolMessageMaxRunes).JSON()
			} else {
				content = truncateToolResult(content)
			}
			return content, false, nil
		},
		// 保持迁移前的重复错误文案（不带角色前缀）。
		RepeatError: func(round int) error {
			return fmt.Errorf("%w（第 %d 轮出现重复的工具调用）", agentbase.ErrLoop, round)
		},
		// 失败由 Run/Execute 外层统一上报，避免同一错误重复进入修复队列。
		DisableReport: true,
	})
	if err != nil {
		if errors.Is(err, agentbase.ErrMaxRounds) {
			return "", handles, fmt.Errorf("在 %d 轮内没有完成文件编辑任务", maxRounds)
		}
		return "", handles, err
	}
	return strings.TrimSpace(result.Reply.Text), handles, nil
}

// toolsForRound 返回本轮工具定义（保持与原有工具集一致）。
func toolsForRound() []toolDefinition {
	return definitionsFor()
}

// truncateToolResult 单条工具结果回填模型时的长度上限。
func truncateToolResult(text string) string {
	if len([]rune(text)) <= toolMessageMaxRunes {
		return text
	}
	return agentbase.TruncateRunes(text, toolMessageMaxRunes) + "…（结果过长，已截断；需要更多内容请用 offset 分批读取）"
}
