package ViewerCritic

// agent.go 观影者-评论：只生成点评类文本（回应互动、观后回顾），不做合规判定、不对外发布。
// 模型调用、推理实况与循环监管统一走 protocol/agentbase。

import (
	"context"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/agents/viewbase"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// InboxReply 按场景生成一句自然回应（注入人设与长期记忆）。
func InboxReply(ctx context.Context, in InboxInput) (text string, err error) {
	defer func() { agentbase.ReportRun(AgentID, err) }()
	task := loadPrompt(promptInbox)
	if task == "" {
		return "", fmt.Errorf("读取回应提示词失败")
	}
	task = renderPrompt(task, "{scene}", strings.TrimSpace(in.Scene))
	task += "\n回应必须接住对方这句话的具体内容（提到的人、事、情绪），不要用「谢谢支持」这类空话。"
	system := withPersona(in.Persona, task)
	if block := strings.TrimSpace(in.Memory); block != "" {
		system += "\n\n" + block
	}
	text, err = viewbase.Chat(ctx, viewbase.ChatRequest{
		Agent: AgentID, Role: Role, Model: in.Model, System: system, User: in.Material, Attempts: attempts,
	})
	if err != nil {
		return "", err
	}
	text = strings.TrimSpace(strings.ReplaceAll(text, "@", ""))
	if text == "" {
		return "", fmt.Errorf("回应为空")
	}
	recordCritic("inbox_reply", text)
	return text, nil
}

// RoundMemory 把一整轮的观影体验写成人格化记忆（不写数量、条目等运行信息）。
func RoundMemory(ctx context.Context, in MemoryInput) (text string, err error) {
	defer func() { agentbase.ReportRun(AgentID, err) }()
	task := loadPrompt(promptMemory)
	if task == "" {
		return "", fmt.Errorf("读取回顾提示词失败")
	}
	system := withPersona(in.Persona, task)
	text, err = viewbase.Chat(ctx, viewbase.ChatRequest{
		Agent: AgentID, Role: Role, Model: in.Model, System: system, User: in.Material, Attempts: attempts,
	})
	if err != nil {
		return "", err
	}
	recordCritic("round_memory", text)
	return text, nil
}
