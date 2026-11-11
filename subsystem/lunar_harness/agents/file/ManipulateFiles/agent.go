package ManipulateFiles

// agent.go 文件操纵智能体：按自然语言完成按行替换、追加、删除、创建与删除文件。

import (
	"context"
	"strings"

	"LunarSubsystem/LunarHarness/agents/file"
	"LunarSubsystem/LunarHarness/prompts"
)

// Run 执行一次文件操纵任务。
func Run(ctx context.Context, req Request) (Result, error) {
	collector := &collector{root: req.WorkspaceRoot}
	instruction := withHandlePaths(req.Instruction, req.Handles)
	tools := definitions()
	handler := toolHandler(collector)
	summary, err := file.Run(ctx, file.Request{
		AgentID:          AgentID,
		Role:             Role,
		SystemPrompt:     prompts.ReadOr(promptFile, "你是文件操纵智能体，只通过工具修改文件。"),
		Instruction:      instruction,
		Tools:            tools,
		Handler:          handler,
		MaxRounds:        maxRounds,
		MaxTokens:        8192,
		SummaryMaxTokens: 320,
	})
	result := Result{
		Summary:  strings.TrimSpace(summary),
		Handles:  collector.handles(),
		Changed:  collector.changed(),
		Evidence: collector.evidence(),
	}
	if result.Summary == "" {
		result.Summary = strings.Join(result.Evidence, "\n")
	}
	if err != nil {
		// 已经落盘的修改必须保留，交由上层判断是部分成功还是回滚。
		return result, err
	}
	return result, nil
}
