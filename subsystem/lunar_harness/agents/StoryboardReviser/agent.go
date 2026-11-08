package StoryboardReviser

// agent.go 分镜规划-修订：把用户审查确认后的分镜整理成可直接绘制的最终稿。
//
// 审查内容是用户资产：本智能体只补全空字段、统一编号与衔接说明，
// 用户已经写下的叙事与画面描述一律原样保留（解析后再做一次合并保护）。

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// base 智能体基座：拥有自己的 ID、实况角色名与调用超时。
var base = agentbase.NewAgent(agentbase.AgentConfig{
	ID:        AgentID,
	Label:     modelRole,
	Timeout:   reviseTimeout,
	MaxRounds: 1,
})

// Revise 整理审查稿。
func Revise(ctx context.Context, req Request) (result Result, err error) {
	defer func() { agentbase.ReportRun(AgentID, err) }()
	if ctx == nil {
		ctx = context.Background()
	}
	if len(req.Shots) == 0 {
		return Result{}, fmt.Errorf("审查稿为空")
	}
	if len(req.Shots) > maxShots {
		return Result{}, fmt.Errorf("分镜数量 %d 超过上限 %d", len(req.Shots), maxShots)
	}
	system := loadPrompt(promptName)
	if system == "" {
		return Result{}, fmt.Errorf("读取修订师提示词失败")
	}
	temperature := 0.3
	reply, err := base.Stream(ctx, agentbase.StreamRequest{
		Messages: []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": userPrompt(req)},
		},
		Temperature: &temperature,
	})
	if err != nil {
		return Result{}, err
	}
	title, shots := parse(reply.Text)
	if len(shots) != len(req.Shots) {
		return Result{}, fmt.Errorf("修订后的镜头数 %d 与审查稿 %d 不一致", len(shots), len(req.Shots))
	}
	ratio := strings.TrimSpace(req.Ratio)
	if ratio == "" {
		ratio = defaultRatio
	}
	merged := make([]Shot, len(shots))
	for index := range shots {
		merged[index] = mergeShot(req.Shots[index], shots[index], ratio)
	}
	if title = strings.TrimSpace(title); title == "" {
		title = strings.TrimSpace(req.Title)
	}
	return Result{Title: title, Shots: merged}, nil
}

// mergeShot 以用户审查稿为准，只接受模型补全的空字段。
func mergeShot(original, revised Shot, ratio string) Shot {
	out := revised
	if strings.TrimSpace(original.Act) != "" {
		out.Act = original.Act
	}
	if strings.TrimSpace(original.Scene) != "" {
		out.Scene = original.Scene
	}
	if strings.TrimSpace(original.Narration) != "" {
		out.Narration = original.Narration
	}
	if strings.TrimSpace(original.Continuity) != "" {
		out.Continuity = original.Continuity
	}
	if strings.TrimSpace(original.Prompt) != "" {
		out.Prompt = original.Prompt
	}
	if strings.TrimSpace(original.Ratio) != "" {
		out.Ratio = original.Ratio
	}
	if strings.TrimSpace(out.Ratio) == "" {
		out.Ratio = ratio
	}
	return out
}

// parse 解析 NDJSON：第一行为标题，其余行按顺序为分镜。
func parse(text string) (string, []Shot) {
	var (
		title string
		shots []Shot
	)
	for _, line := range strings.Split(text, "\n") {
		body := extractJSONObject(line)
		if body == "" {
			continue
		}
		var raw revisedLine
		if err := json.Unmarshal([]byte(body), &raw); err != nil {
			continue
		}
		if raw.Scene == "" && raw.Prompt == "" && raw.Narration == "" {
			if title == "" {
				title = strings.TrimSpace(raw.Title)
			}
			continue
		}
		shots = append(shots, Shot{
			Act:        strings.TrimSpace(raw.Act),
			Scene:      strings.TrimSpace(raw.Scene),
			Narration:  strings.TrimSpace(raw.Narration),
			Continuity: strings.TrimSpace(raw.Continuity),
			Prompt:     strings.TrimSpace(raw.Prompt),
			Ratio:      strings.TrimSpace(raw.Ratio),
		})
	}
	return title, shots
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
