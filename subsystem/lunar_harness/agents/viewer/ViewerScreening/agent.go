package ViewerScreening

// agent.go 观影者-阅片：只看内容本身（画面、公开信息与观后感受），不做合规判定、不生成对外文案。
// 模型调用、推理实况与循环监管统一走 protocol/agentbase。

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/agents/viewer"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// Impress 依据采集材料整理结构化视频印象，结果只从 submit_impression 的参数读取。
func Impress(ctx context.Context, material, model string) (impression Impression, err error) {
	defer func() { agentbase.ReportRun(AgentID, err) }()
	system := loadPrompt(promptImpress)
	if system == "" {
		return Impression{}, fmt.Errorf("读取阅片提示词失败")
	}
	call, err := viewer.Submit(ctx, viewer.SubmitRequest{
		Agent: AgentID, Role: Role, Model: model, System: system, User: material,
		Tool: impressionTool, Attempts: impressAttempts,
	})
	if err != nil {
		return Impression{}, err
	}
	impression, err = parseImpressionArgs(call.Function.Arguments)
	if err != nil {
		agentbase.ReportToolResult(Role, call.Function.Name, agentbase.ToolNotApplied, err.Error())
		return Impression{}, err
	}
	agentbase.ReportToolResult(Role, call.Function.Name, agentbase.ToolApplied, "视频印象已受理")
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
	return viewer.Chat(ctx, viewer.ChatRequest{
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
	return viewer.Chat(ctx, viewer.ChatRequest{
		Agent: AgentID, Role: Role, Model: model, System: system, User: material, Attempts: impressAttempts,
	})
}

// Slice 观看一个 60 秒切片：人设在包内注入，单次调用、不重试，命中防死循环时直接放弃该切片。
func Slice(ctx context.Context, task, model string, images []string) (text string, err error) {
	defer func() { agentbase.ReportRun(AgentID, err) }()
	persona := loadPersona()
	if strings.TrimSpace(persona) == "" {
		return "", fmt.Errorf("读取人设提示词失败")
	}
	return viewer.Chat(ctx, viewer.ChatRequest{
		Agent: AgentID, Role: Role, Model: model, System: persona, User: task, Images: images, Attempts: onceAttempts,
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
	return viewer.Chat(ctx, viewer.ChatRequest{
		Agent: AgentID, Role: Role, Model: model, System: system, User: prompt, Attempts: impressAttempts,
	})
}

// parseImpressionArgs 校验 submit_impression 的参数：摘要/主题/氛围都必填，主题 2-5 个。
func parseImpressionArgs(raw string) (Impression, error) {
	var payload struct {
		Summary string   `json:"summary"`
		Topics  []string `json:"topics"`
		Tone    string   `json:"tone"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return Impression{}, fmt.Errorf("印象参数无法解析: %w", err)
	}
	summary := agentbase.TruncateRunes(strings.TrimSpace(payload.Summary), summaryMaxRunes)
	if summary == "" {
		return Impression{}, fmt.Errorf("印象参数缺少 summary")
	}
	topics := make([]string, 0, len(payload.Topics))
	for _, topic := range payload.Topics {
		if topic = strings.TrimSpace(topic); topic != "" {
			topics = append(topics, topic)
		}
	}
	if len(topics) < 2 || len(topics) > 5 {
		return Impression{}, fmt.Errorf("印象主题词数量非法: %d", len(topics))
	}
	tone := strings.TrimSpace(payload.Tone)
	if tone == "" {
		return Impression{}, fmt.Errorf("印象参数缺少 tone")
	}
	return Impression{Summary: summary, Topics: topics, Tone: tone}, nil
}
