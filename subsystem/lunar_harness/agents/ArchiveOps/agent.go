package ArchiveOps

// agent.go 自然语言入口：一条任务描述 → 模型解读 → 压缩包写操作 → 结构化结果。
//
// 文件编辑通过 task_scheduling(type=archive_ops) 单方面调度本智能体；
// 这里复用共享模型调用层（agentchat），推理实况、循环截断与失败上报与其他智能体一致。

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/model/agentchat"
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

	messages := []agentchat.Message{
		{Role: "system", Content: agentchat.PromptText("archiveOps.md")},
		{Role: "user", Content: instruction},
	}
	executed := false
	last := Result{Op: ScheduleOp}
	handles := make([]string, 0, 4)
	seen := map[string]bool{}
	for round := 0; round < maxScheduleRounds; round++ {
		reply, err := agentchat.ChatStream(roleLabel, messages, toolDefinitions())
		if err != nil {
			return Result{}, err
		}
		text := strings.TrimSpace(agentchat.StringOf(reply.Content))
		if len(reply.ToolCalls) == 0 {
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
		// assistant 的工具调用必须原样回填，再逐条给出配对结果
		reply.Content = text
		messages = append(messages, reply)
		for _, call := range reply.ToolCalls {
			outcome := executeTool(call)
			executed = true
			last = outcome
			for _, handle := range outcome.Handles {
				if handle != "" && !seen[handle] {
					seen[handle] = true
					handles = append(handles, handle)
				}
			}
			messages = append(messages, agentchat.Message{
				Role:       "tool",
				ToolCallID: call.ID,
				Content:    PreviewRunes(outcome.JSON(), maxToolResultRunes),
			})
		}
	}
	logger.Warn(Module, "归档操作在 %d 轮内没有完成", maxScheduleRounds)
	return failResult(ScheduleOp, last.Path, fmt.Errorf("在 %d 轮内没有完成归档操作", maxScheduleRounds)), nil
}
