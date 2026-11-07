package ViewerShare

// agent.go 观影者-分享：只生成对外发布文案（观影动态、@分享评论），不做合规判定、不直接发布。
// 模型调用、推理实况与循环监管统一走 protocol/agentbase。

import (
	"context"
	"fmt"

	"LunarSubsystem/LunarHarness/agents/viewbase"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// Dynamic 生成本轮观影动态正文。
func Dynamic(ctx context.Context, in DynamicInput) (text string, err error) {
	defer func() { agentbase.ReportRun(AgentID, err) }()
	task := loadPrompt(promptDynamic)
	if task == "" {
		return "", fmt.Errorf("读取动态提示词失败")
	}
	system := withPersona(in.Persona, task)
	text, err = viewbase.Chat(ctx, viewbase.ChatRequest{
		Agent: AgentID, Role: Role, Model: in.Model, System: system, User: in.Material, Attempts: attempts,
	})
	if err != nil {
		return "", err
	}
	recordShare("dynamic", text)
	return text, nil
}

// ShareComment 生成一条 @分享评论（基于本轮整体感受，不逐条枚举）。
func ShareComment(ctx context.Context, in ShareInput) (text string, err error) {
	defer func() { agentbase.ReportRun(AgentID, err) }()
	task := loadPrompt(promptShare)
	if task == "" {
		return "", fmt.Errorf("读取分享提示词失败")
	}
	material := fmt.Sprintf("视频：《%s》\nUP主：%s\n我的初步印象：%s\n",
		in.Title, in.Owner, agentbase.TruncateRunes(in.Summary, summaryMaxRunes))
	text, err = viewbase.Chat(ctx, viewbase.ChatRequest{
		Agent: AgentID, Role: Role, Model: in.Model, System: task, User: material, Attempts: attempts,
	})
	if err != nil {
		return "", err
	}
	recordShare("share_comment", text)
	return text, nil
}
