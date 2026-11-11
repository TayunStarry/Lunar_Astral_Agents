package CodeExecution

// agent.go 代码执行智能体：运行 HTML/JavaScript 项目并解析函数与依赖。

import (
	"context"
	"strings"

	"LunarSubsystem/LunarHarness/agents/toolagent"
	"LunarSubsystem/LunarHarness/prompts"
)

// Run 执行一次代码任务。
func Run(ctx context.Context, req Request) (Result, error) {
	collector := &collector{root: req.WorkspaceRoot}
	summary, err := toolagent.Run(ctx, toolagent.Request{
		AgentID:          AgentID,
		Role:             Role,
		SystemPrompt:     prompts.ReadOr(promptFile, "你是代码执行智能体，只通过工具运行或解析代码。"),
		Instruction:      withHandlePaths(req.Instruction, req.Handles),
		Tools:            definitions(),
		Handler:          toolHandler(collector),
		MaxRounds:        maxRounds,
		MaxTokens:        4096,
		SummaryMaxTokens: 320,
	})
	result := Result{Summary: strings.TrimSpace(summary), Handles: collector.handles()}
	if err != nil {
		return result, err
	}
	return result, nil
}

type collector struct {
	root   string
	handle []string
}

func (c *collector) add(handles ...string) {
	for _, handle := range handles {
		handle = strings.TrimSpace(handle)
		if handle == "" || contains(c.handle, handle) {
			continue
		}
		c.handle = append(c.handle, handle)
	}
}

func (c *collector) handles() []string { return append([]string(nil), c.handle...) }
