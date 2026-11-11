package ManipulateArchive

// agent.go 归档操纵智能体：创建、删除、列出、提取与追加归档条目。

import (
	"context"
	"strings"

	"LunarSubsystem/LunarHarness/agents/toolagent"
	"LunarSubsystem/LunarHarness/prompts"
)

// Run 执行一次归档操纵任务。
func Run(ctx context.Context, req Request) (Result, error) {
	collector := &collector{root: req.WorkspaceRoot}
	instruction := withHandlePaths(req.Instruction, req.Handles)
	summary, err := toolagent.Run(ctx, toolagent.Request{
		AgentID:          AgentID,
		Role:             Role,
		SystemPrompt:     prompts.ReadOr(promptFile, "你是归档操纵智能体，只通过工具操作压缩包。"),
		Instruction:      instruction,
		Tools:            definitions(),
		Handler:          toolHandler(collector),
		MaxRounds:        maxRounds,
		MaxTokens:        4096,
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
		return result, err
	}
	return result, nil
}
