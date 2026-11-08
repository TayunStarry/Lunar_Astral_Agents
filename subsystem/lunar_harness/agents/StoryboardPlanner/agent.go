package StoryboardPlanner

// agent.go 分镜规划-拆解：叙事推演 + 逐镜结构化分镜。
//
// 选择「推演 + NDJSON」两段式而不是纯 JSON 的原因：
//   - 纯结构化输出没有可读内容，用户在推理实况里看不到智能体在分析什么；
//   - 两段式仍是一次模型调用：推演文本逐行回调给调用方，分隔行之后的 NDJSON
//     逐行解析成结构化分镜，过程感与精确进度同时具备。
//
// 分隔行缺失时，以第一行可解析的 JSON 分镜作为自动切换点兜底。

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// base 智能体基座：拥有自己的 ID、实况角色名与调用超时。
var base = agentbase.NewAgent(agentbase.AgentConfig{
	ID:        AgentID,
	Label:     modelRole,
	Timeout:   planTimeout,
	MaxRounds: 1,
})

// Plan 流式完成分镜规划。
//
// req.OnThought 接收叙事推演的可读文本增量；req.OnShot 在每解析出一镜时回调。
// 若流中途失败但已解析出分镜，则使用已解析部分继续演绎，不整任务失败。
func Plan(ctx context.Context, req Request) (result Result, err error) {
	defer func() { agentbase.ReportRun(AgentID, err) }()
	if ctx == nil {
		ctx = context.Background()
	}
	story := clipRunes(strings.TrimSpace(req.Story), maxStoryRunes)
	if story == "" {
		return Result{}, fmt.Errorf("故事正文为空")
	}
	ratio := strings.TrimSpace(req.Ratio)
	if ratio == "" {
		ratio = defaultRatio
	}
	style := strings.TrimSpace(req.Style)
	system := loadPrompt(promptName)
	if system == "" {
		return Result{}, fmt.Errorf("读取分镜师提示词失败")
	}
	messages := []map[string]string{
		{"role": "system", "content": system},
		{"role": "user", "content": userPrompt(story, ratio, req.Research)},
	}
	temperature := 0.7

	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		plan, streamErr := streamPlan(ctx, messages, temperature, style, ratio, req.OnThought, req.OnShot)
		if streamErr == nil {
			return plan, nil
		}
		lastErr = streamErr
		if len(plan.Shots) > 0 {
			// 已经向用户展示了部分分镜：不再重试，避免重复推送。
			logger.Warn(AgentID, "分镜流式解析中断，使用已解析的 %d 镜: %v", len(plan.Shots), streamErr)
			return plan, nil
		}
		if ctx.Err() != nil {
			return Result{}, fmt.Errorf("分镜规划超时：%w", ctx.Err())
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("模型没有返回可用的分镜")
	}
	return Result{}, fmt.Errorf("分镜规划失败：%v", lastErr)
}

// streamPlan 发起一次流式调用：推演段逐行回调，分镜段逐行解析。
func streamPlan(
	ctx context.Context,
	messages []map[string]string,
	temperature float64,
	style, ratio string,
	onThought func(string),
	onShot func(title string, shot Shot),
) (Result, error) {
	var (
		title  string
		shots  []Shot
		buffer strings.Builder
		inPlan bool
	)
	// parseLine 解析一行结构化分镜；返回是否被当作结构化内容处理。
	parseLine := func(line string) bool {
		body := extractJSONObject(line)
		if body == "" {
			return false
		}
		var raw plannedLine
		if err := json.Unmarshal([]byte(body), &raw); err != nil {
			return false
		}
		if raw.Scene == "" && raw.Prompt == "" && raw.Narration == "" {
			if name := strings.TrimSpace(raw.Title); name != "" && title == "" {
				title = name
			}
			return true
		}
		shot := normalizeShot(Shot{
			Act:        strings.TrimSpace(raw.Act),
			Scene:      strings.TrimSpace(raw.Scene),
			Narration:  strings.TrimSpace(raw.Narration),
			Continuity: strings.TrimSpace(raw.Continuity),
			Prompt:     strings.TrimSpace(raw.Prompt),
			Ratio:      strings.TrimSpace(raw.Ratio),
		}, len(shots)+1, style, ratio)
		shots = append(shots, shot)
		if onShot != nil {
			onShot(title, shot)
		}
		return true
	}

	emit := func(line string) {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "```") {
			return
		}
		if !inPlan {
			if trimmed == planDelimiter {
				inPlan = true
				return
			}
			// 兜底：模型没写分隔行时，以第一条可解析的 JSON 分镜为切换点。
			if strings.HasPrefix(trimmed, "{") {
				inPlan = true
				parseLine(trimmed)
				return
			}
			if onThought != nil {
				onThought(line + "\n")
			}
			return
		}
		parseLine(trimmed)
	}
	onDelta := func(kind, text string) {
		if kind != "content" || text == "" {
			return
		}
		buffer.WriteString(text)
		for {
			raw := buffer.String()
			index := strings.IndexByte(raw, '\n')
			if index < 0 {
				return
			}
			emit(raw[:index])
			buffer.Reset()
			buffer.WriteString(raw[index+1:])
		}
	}

	_, err := base.Stream(ctx, agentbase.StreamRequest{
		Messages:    messages,
		Temperature: &temperature,
		OnDelta:     onDelta,
	})
	// 处理最后一行没有换行结尾的残余内容。
	emit(buffer.String())
	buffer.Reset()

	plan := Result{Title: title, Style: style, Shots: shots}
	if err != nil {
		return plan, err
	}
	if len(shots) == 0 {
		return plan, fmt.Errorf("模型没有输出可解析的分镜")
	}
	if plan.Title == "" {
		plan.Title = fallbackTitle
	}
	return plan, nil
}

// normalizeShot 补默认值并把单镜叙事与镜头衔接整理成提示词。
func normalizeShot(shot Shot, index int, style, ratio string) Shot {
	shot.Index = index
	if shot.Scene == "" {
		shot.Scene = fmt.Sprintf("镜 %d", index)
	}
	if shot.Ratio == "" {
		shot.Ratio = ratio
	}
	shot.Style = style
	shot.Prompt = ComposePrompt(shot)
	return shot
}

// ComposePrompt 只整理分镜叙事、镜头衔接与画面要求。
//
// 角色外观与画风的一致性由图像绘制智能体自己维护：它有内置/自定义角色库、
// 统一角色画风模板、跨任务上下文与参考图（图生图）机制。链条不再把
// 「统一角色设定」「统一画风」重复注入每一镜。
func ComposePrompt(shot Shot) string {
	var b strings.Builder
	if shot.Act != "" {
		b.WriteString("【幕】" + shot.Act + "\n")
	}
	if shot.Scene != "" {
		b.WriteString("【镜次】" + shot.Scene + "\n")
	}
	if shot.Narration != "" {
		b.WriteString("【本镜叙事】" + shot.Narration + "\n")
	}
	if shot.Continuity != "" {
		b.WriteString("【镜头衔接】" + shot.Continuity + "\n")
	}
	b.WriteString("【画面要求】" + shot.Prompt + "\n")
	return clipRunes(b.String(), maxPaintPromptRunes)
}

// extractJSONObject 从正文中截取第一个 JSON 对象。
func extractJSONObject(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end <= start {
		return ""
	}
	return text[start : end+1]
}

// clipRunes 按字符数截断。
func clipRunes(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "…"
}
