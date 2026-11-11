package ContentRollback

// agent.go 内容回滚智能体：列出文件历史版本并回滚。

import (
	"context"
	"strings"

	"LunarSubsystem/LunarHarness/agents/file"
	"LunarSubsystem/LunarHarness/prompts"
)

// Run 执行一次回滚任务。
func Run(ctx context.Context, req Request) (Result, error) {
	collector := &collector{root: req.WorkspaceRoot}
	summary, err := file.Run(ctx, file.Request{
		AgentID:          AgentID,
		Role:             Role,
		SystemPrompt:     prompts.ReadOr(promptFile, "你是内容回滚智能体，只通过工具查看和恢复文件版本。"),
		Instruction:      withHandlePaths(req.Instruction, req.Handles),
		Tools:            definitions(),
		Handler:          toolHandler(collector),
		MaxRounds:        maxRounds,
		MaxTokens:        2048,
		SummaryMaxTokens: 320,
	})
	result := Result{Summary: strings.TrimSpace(summary), Handles: collector.handles(), Changed: collector.changed()}
	if err != nil {
		return result, err
	}
	return result, nil
}

type collector struct {
	root         string
	handle       []string
	changedPaths []string
}

func (c *collector) addHandle(handles ...string) {
	for _, handle := range handles {
		handle = strings.TrimSpace(handle)
		if handle == "" || contains(c.handle, handle) {
			continue
		}
		c.handle = append(c.handle, handle)
	}
}

func (c *collector) addChanged(rel string) {
	if rel == "" || contains(c.changedPaths, rel) {
		return
	}
	c.changedPaths = append(c.changedPaths, rel)
}

func (c *collector) handles() []string { return append([]string(nil), c.handle...) }
func (c *collector) changed() []string { return append([]string(nil), c.changedPaths...) }

func contains(list []string, item string) bool {
	for _, value := range list {
		if value == item {
			return true
		}
	}
	return false
}
