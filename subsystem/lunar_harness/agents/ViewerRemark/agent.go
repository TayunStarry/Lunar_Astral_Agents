package ViewerRemark

// agent.go 观影者-观感：基于片段画面与视频印象写一条可发布到评论区的中文评论。
// 只生成文本，不发布、不做合规判定；发布由 chain/viewer 负责。

import (
	"context"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/agents/viewbase"
	"LunarSubsystem/LunarHarness/prompts"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// Remark 生成一条符合语境、贴合画面的观感评论。
func Remark(ctx context.Context, in RemarkInput) (text string, err error) {
	defer func() { agentbase.ReportRun(AgentID, err) }()
	task := loadPrompt(promptRemark)
	if task == "" {
		return "", fmt.Errorf("读取观感评论提示词失败")
	}
	system := withPersona(in.Persona, task)
	text, err = viewbase.Chat(ctx, viewbase.ChatRequest{
		Agent: AgentID, Role: Role, Model: in.Model, System: system, User: in.Material, Attempts: attempts,
	})
	if err != nil {
		return "", err
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "", fmt.Errorf("观感评论为空")
	}
	return text, nil
}

// loadPrompt 读取内嵌提示词（失败返回空串）。
func loadPrompt(name string) string {
	data, err := prompts.Read(name)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// withPersona 拼人设与任务段落。
func withPersona(persona, task string) string {
	persona = strings.TrimSpace(persona)
	if persona == "" {
		return task
	}
	return persona + taskBlock + task
}
