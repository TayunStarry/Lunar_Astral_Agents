package ArchiveOps

// agent.go 自然语言入口：一条任务描述 → 模型解读 → 压缩包写操作 → 结构化结果。
//
// 文件编辑通过 task_scheduling(type=archive_ops) 单方面调度本智能体；
// 这里复用统一智能体基座（protocol/agentbase），推理实况、循环截断与失败上报与其他智能体一致。

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// scheduleMu 串行化调度，避免模型通道与他人交错
var scheduleMu sync.Mutex

// Execute 用自然语言执行一次归档任务，返回结构化结果。
func Execute(ctx context.Context, instruction string) (Result, error) {
	instruction = strings.TrimSpace(instruction)
	if instruction == "" {
		return failResult(ScheduleOp, "", fmt.Errorf("请用自然语言说明要执行的归档操作")), nil
	}
	scheduleMu.Lock()
	defer scheduleMu.Unlock()

	executed := false
	last := Result{Op: ScheduleOp}
	handles := make([]string, 0, 4)
	seen := map[string]bool{}
	base := agentbase.NewAgent(agentbase.AgentConfig{
		ID:                 "archive_ops",
		Label:              roleLabel,
		MaxRounds:          maxScheduleRounds,
		MaxRepeats:         -1, // 迁移前无跨轮重复检测，保持原有行为
		MaxToolResultRunes: -1, // 使用 PreviewRunes 的原有截断语义
	})
	result, err := base.RunToolLoop(ctx, agentbase.ToolLoopRequest{
		Messages: []agentbase.Message{
			{Role: "system", Content: agentbase.PromptText("archiveOps.md")},
			{Role: "user", Content: instruction},
		},
		Tools: toolDefinitions(),
		Stream: agentbase.StreamRequest{
			Agent: "archive_ops",
			Role:  roleLabel,
		},
		Execute: func(_ context.Context, call agentbase.ToolCall) (string, bool, error) {
			outcome := executeTool(call)
			executed = true
			last = outcome
			for _, handle := range outcome.Handles {
				if handle != "" && !seen[handle] {
					seen[handle] = true
					handles = append(handles, handle)
				}
			}
			return PreviewRunes(outcome.JSON(), maxToolResultRunes), false, nil
		},
	})
	if err != nil {
		if errors.Is(err, agentbase.ErrMaxRounds) {
			logger.Warn(Module, "归档操作在 %d 轮内没有完成", maxScheduleRounds)
			return failResult(ScheduleOp, last.Path, fmt.Errorf("在 %d 轮内没有完成归档操作", maxScheduleRounds)), nil
		}
		return Result{}, err
	}

	text := strings.TrimSpace(result.Reply.Text)
	if !executed {
		return failResult(ScheduleOp, "", fmt.Errorf("没有执行任何归档操作")), nil
	}
	if !last.OK {
		return last, nil
	}
	summary := text
	if summary == "" {
		summary = last.Summary
	}
	return Result{
		OK:      true,
		Op:      last.Op,
		Path:    last.Path,
		Handles: handles,
		Summary: summary,
		Entries: last.Entries,
		Detail:  last.Detail,
	}, nil
}
