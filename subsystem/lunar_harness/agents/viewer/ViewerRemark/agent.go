package ViewerRemark

// agent.go 观影者-观感：基于「视频印象」写 1-2 条可发布到评论区的中文评论，
// 每条评论各自决定是否想 @ 特别关注（人数 + 可选的候选名字/序号）。
// 只生成文本与 @ 意图，不发布、不做合规判定；发布与候选校验由 chain/viewer 负责。

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"LunarSubsystem/LunarHarness/agents/viewer"
	"LunarSubsystem/LunarHarness/prompts"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// Generate 生成 1-2 条评论与 @ 意图。
func Generate(ctx context.Context, in RemarkInput) (remark Remark, err error) {
	defer func() { agentbase.ReportRun(AgentID, err) }()
	persona := loadPersona()
	if strings.TrimSpace(persona) == "" {
		return Remark{}, fmt.Errorf("读取人设提示词失败")
	}
	task := loadPrompt(promptRemark)
	if task == "" {
		return Remark{}, fmt.Errorf("读取观感评论提示词失败")
	}
	system := withPersona(persona, task)
	text, err := viewer.Chat(ctx, viewer.ChatRequest{
		Agent: AgentID, Role: Role, Model: in.Model, System: system,
		User: withCandidates(in.Material, in.Candidates), Attempts: attempts,
	})
	if err != nil {
		return Remark{}, err
	}
	remark = parseRemark(text)
	if len(remark.Comments) == 0 {
		return Remark{}, fmt.Errorf("观感评论为空")
	}
	return remark, nil
}

// parseRemark 解析结构化输出；JSON 不可用时退回「整段文本算一条评论，不 @」。
func parseRemark(text string) Remark {
	if raw := extractJSONObject(text); raw != "" {
		var payload struct {
			Comments []json.RawMessage `json:"comments"`
			Comment  string            `json:"comment"`
		}
		if err := json.Unmarshal([]byte(raw), &payload); err == nil {
			comments := parseComments(payload.Comments)
			if len(comments) == 0 && strings.TrimSpace(payload.Comment) != "" {
				comments = []RemarkComment{{Text: payload.Comment}}
			}
			if len(comments) > 0 {
				return Remark{Comments: cleanComments(comments)}
			}
		}
	}
	return Remark{Comments: cleanComments([]RemarkComment{{Text: text}})}
}

// parseComments 解析每条评论：兼容纯字符串（不 @）与带 text / mention / at 的对象。
func parseComments(raw []json.RawMessage) []RemarkComment {
	out := make([]RemarkComment, 0, maxComments)
	for _, item := range raw {
		var text string
		if err := json.Unmarshal(item, &text); err == nil {
			if text = strings.TrimSpace(text); text != "" {
				out = append(out, RemarkComment{Text: text})
			}
			continue
		}
		var payload struct {
			Text    string            `json:"text"`
			Comment string            `json:"comment"`
			Content string            `json:"content"`
			Mention *int              `json:"mention"`
			At      []json.RawMessage `json:"at"`
		}
		if err := json.Unmarshal(item, &payload); err != nil {
			continue
		}
		text = firstNonEmpty(payload.Text, payload.Comment, payload.Content)
		if text == "" {
			continue
		}
		mention := 0
		if payload.Mention != nil {
			mention = *payload.Mention
		}
		out = append(out, RemarkComment{
			Text:    text,
			Mention: clampMention(mention),
			At:      parseAt(payload.At),
		})
		if len(out) >= maxComments {
			break
		}
	}
	return out
}

// firstNonEmpty 返回第一个去空白后非空的文本。
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

// parseAt 解析模型指定的候选：兼容字符串（名字）与数字（候选序号），最多保留 3 个。
func parseAt(raw []json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	out := make([]string, 0, mentionMax)
	for _, item := range raw {
		var name string
		if err := json.Unmarshal(item, &name); err == nil {
			if name = strings.TrimSpace(name); name != "" {
				out = append(out, name)
			}
		} else {
			var index int
			if err := json.Unmarshal(item, &index); err == nil && index > 0 {
				out = append(out, strconv.Itoa(index))
			}
		}
		if len(out) >= mentionMax {
			break
		}
	}
	return out
}

// withCandidates 把特别关注候选清单附在材料末尾；没有候选时明确告诉模型不要指定人。
func withCandidates(material string, candidates []MentionCandidate) string {
	if len(candidates) == 0 {
		return material + "\n\n【可 @ 的特别关注】\n（当前没有可 @ 的人，每条评论的 mention 请填 0，at 留空）"
	}
	var b strings.Builder
	b.WriteString(material)
	b.WriteString("\n\n【可 @ 的特别关注】只能从下列清单里选择（写序号或名字都行）：\n")
	for _, candidate := range candidates {
		fmt.Fprintf(&b, "%d. %s\n", candidate.Index, candidate.Name)
	}
	return strings.TrimRight(b.String(), "\n")
}

// cleanComments 去空白、限长，最多保留 2 条；每条保留自己的 @ 决策。
func cleanComments(raw []RemarkComment) []RemarkComment {
	out := make([]RemarkComment, 0, maxComments)
	for _, comment := range raw {
		text := agentbase.TruncateRunes(strings.TrimSpace(comment.Text), commentMaxRunes)
		if text == "" {
			continue
		}
		comment.Text = text
		out = append(out, comment)
		if len(out) >= maxComments {
			break
		}
	}
	return out
}

// clampMention 把 @ 人数钳制到 0-3。
func clampMention(value int) int {
	if value < 0 {
		return 0
	}
	if value > mentionMax {
		return mentionMax
	}
	return value
}

// extractJSONObject 从模型输出里截出第一个 JSON 对象（容忍代码块包裹）。
func extractJSONObject(text string) string {
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end <= start {
		return ""
	}
	return text[start : end+1]
}

// loadPrompt 读取内嵌提示词（失败返回空串）。
func loadPrompt(name string) string {
	data, err := prompts.Read(name)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// loadPersona 读取人设提示词（复用基座进程内缓存，同一包内多次调用只读一次文件）。
//
// 读取失败时基座返回空串并记日志；调用方必须把空串当失败处理，不能静默降级成无人设。
func loadPersona() string {
	return agentbase.PromptText(personaFile)
}

// withPersona 拼人设与任务段落。
func withPersona(persona, task string) string {
	persona = strings.TrimSpace(persona)
	if persona == "" {
		return task
	}
	return persona + taskBlock + task
}
