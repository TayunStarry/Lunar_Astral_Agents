package ViewerScreening

// agent.go 观影者-阅片：只看内容本身（画面、公开信息与观后感受），不做合规判定、不生成对外文案。
// 模型调用、推理实况与循环监管统一走 protocol/agentbase。

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/agents/viewbase"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// Impress 依据采集材料整理结构化视频印象。
func Impress(ctx context.Context, material, model string) (impression Impression, err error) {
	defer func() { agentbase.ReportRun(AgentID, err) }()
	system := loadPrompt(promptImpress)
	if system == "" {
		return Impression{}, fmt.Errorf("读取阅片提示词失败")
	}
	text, err := viewbase.Chat(ctx, viewbase.ChatRequest{
		Agent: AgentID, Role: Role, Model: model, System: system, User: material, Attempts: impressAttempts,
	})
	if err != nil {
		return Impression{}, err
	}
	if raw := extractJSONObject(text); raw != "" {
		if parseErr := json.Unmarshal([]byte(raw), &impression); parseErr != nil {
			return Impression{}, fmt.Errorf("印象 JSON 解析失败: %w", parseErr)
		}
	}
	impression.Summary = agentbase.TruncateRunes(strings.TrimSpace(impression.Summary), summaryMaxRunes)
	if impression.Summary == "" {
		return Impression{}, fmt.Errorf("印象摘要为空")
	}
	for index, topic := range impression.Topics {
		impression.Topics[index] = strings.TrimSpace(topic)
	}
	recordImpression(impression)
	return impression, nil
}

// QuickSummary 指定视频的快速速览：只读公开材料，写 2-4 句摘要。
func QuickSummary(ctx context.Context, material, instruction, model string) (text string, err error) {
	defer func() { agentbase.ReportRun(AgentID, err) }()
	system := loadPrompt(promptQuick)
	if system == "" {
		return "", fmt.Errorf("读取速览提示词失败")
	}
	if focus := strings.TrimSpace(instruction); focus != "" {
		system += "本次特别关注：" + agentbase.TruncateRunes(focus, instructionMaxRunes) + "。"
	}
	return viewbase.Chat(ctx, viewbase.ChatRequest{
		Agent: AgentID, Role: Role, Model: model, System: system, User: material, Attempts: impressAttempts,
	})
}

// DirectSummary 指定视频的 URL 直驱印象：写一段 120-220 字的第一人称印象。
func DirectSummary(ctx context.Context, material, instruction, model string) (text string, err error) {
	defer func() { agentbase.ReportRun(AgentID, err) }()
	system := loadPrompt(promptDirect)
	if system == "" {
		return "", fmt.Errorf("读取直驱印象提示词失败")
	}
	if focus := strings.TrimSpace(instruction); focus != "" {
		system += "本次特别关注：" + agentbase.TruncateRunes(focus, instructionMaxRunes) + "。"
	}
	return viewbase.Chat(ctx, viewbase.ChatRequest{
		Agent: AgentID, Role: Role, Model: model, System: system, User: material, Attempts: impressAttempts,
	})
}

// Slice 观看一个 60 秒切片：单次调用、不重试，命中防死循环时直接放弃该切片。
func Slice(ctx context.Context, system, prompt, model string, images []string) (text string, err error) {
	defer func() { agentbase.ReportRun(AgentID, err) }()
	return viewbase.Chat(ctx, viewbase.ChatRequest{
		Agent: AgentID, Role: Role, Model: model, System: system, User: prompt, Images: images, Attempts: onceAttempts,
	})
}

// WatchSummary 把各切片理解合成为第一人称观影感受（切片理解只作素材）。
func WatchSummary(ctx context.Context, system, material, model string) (text string, err error) {
	defer func() { agentbase.ReportRun(AgentID, err) }()
	return viewbase.Chat(ctx, viewbase.ChatRequest{
		Agent: AgentID, Role: Role, Model: model, System: system, User: material, Attempts: impressAttempts,
	})
}

// Keywords 综合指令、默认偏好、心情与记忆提炼本轮选片关键词（原样返回模型文本，由链切分）。
func Keywords(ctx context.Context, in KeywordInput, model string) (text string, err error) {
	defer func() { agentbase.ReportRun(AgentID, err) }()
	system := loadPrompt(promptKeywords)
	if system == "" {
		return "", fmt.Errorf("读取选片关键词提示词失败")
	}
	payload := promptPayload{
		Defaults: strings.Join(in.Defaults, "、"),
		Mood:     strings.TrimSpace(in.Mood),
		Memory:   strings.TrimSpace(in.Memory),
	}
	defaultsLine := ""
	if payload.Defaults != "" {
		defaultsLine = "配置默认偏好：" + payload.Defaults + "。\n"
	}
	moodLine := ""
	if payload.Mood != "" {
		moodLine = "当前心情：" + payload.Mood + "。\n"
	}
	system = renderPrompt(system,
		"{defaultsLine}", defaultsLine,
		"{moodLine}", moodLine,
		"{memory}", payload.Memory)
	prompt := strings.TrimSpace(in.Instruction)
	if prompt == "" {
		prompt = "这一轮没有特别要求，按我自己的兴趣和心情挑。"
	}
	return viewbase.Chat(ctx, viewbase.ChatRequest{
		Agent: AgentID, Role: Role, Model: model, System: system, User: prompt, Attempts: impressAttempts,
	})
}

// extractJSONObject 从模型输出里截出第一个 JSON 对象（容忍代码块包裹）
func extractJSONObject(text string) string {
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end <= start {
		return ""
	}
	return text[start : end+1]
}
