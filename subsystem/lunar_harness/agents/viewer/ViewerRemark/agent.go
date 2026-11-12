package ViewerRemark

// agent.go 观影者-观感：基于「视频印象」写 1-2 条可发布到评论区的中文评论，
// 每条评论各自决定是否想 @ 特别关注（人数 + 可选的候选名字/序号）。
// 只生成文本与 @ 意图，不发布、不做合规判定；发布与候选校验由 chain/viewer 负责。
//
// 结果只从 submit_remark 的参数读取，正文不解析，也不作纯文本兜底。

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
	call, err := viewer.Submit(ctx, viewer.SubmitRequest{
		Agent: AgentID, Role: Role, Model: in.Model, System: system,
		User: withCandidates(in.Material, in.Candidates), Tool: remarkTool, Attempts: attempts,
	})
	if err != nil {
		return Remark{}, err
	}
	remark, err = parseRemarkArgs(call.Function.Arguments)
	if err != nil {
		agentbase.ReportToolResult(Role, call.Function.Name, agentbase.ToolNotApplied, err.Error())
		return Remark{}, err
	}
	agentbase.ReportToolResult(Role, call.Function.Name, agentbase.ToolApplied,
		fmt.Sprintf("观感评论已受理：%d 条", len(remark.Comments)))
	return remark, nil
}

// parseRemarkArgs 校验 submit_remark 的参数：1-2 条评论，每条必须有 text 与 0-3 的 mention。
func parseRemarkArgs(raw string) (Remark, error) {
	var payload remarkToolArgs
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return Remark{}, fmt.Errorf("评论参数无法解析: %w", err)
	}
	if len(payload.Comments) == 0 {
		return Remark{}, fmt.Errorf("评论参数缺少 comments")
	}
	if len(payload.Comments) > maxComments {
		return Remark{}, fmt.Errorf("评论条数超过上限 %d: %d", maxComments, len(payload.Comments))
	}
	out := make([]RemarkComment, 0, len(payload.Comments))
	for index, item := range payload.Comments {
		text := strings.TrimSpace(item.Text)
		if text == "" {
			return Remark{}, fmt.Errorf("第 %d 条评论缺少 text", index+1)
		}
		if item.Mention == nil {
			return Remark{}, fmt.Errorf("第 %d 条评论缺少 mention", index+1)
		}
		if *item.Mention < 0 || *item.Mention > mentionMax {
			return Remark{}, fmt.Errorf("第 %d 条评论 mention 非法: %d", index+1, *item.Mention)
		}
		at, err := parseAt(item.At)
		if err != nil {
			return Remark{}, fmt.Errorf("第 %d 条评论 at 非法: %w", index+1, err)
		}
		out = append(out, RemarkComment{
			Text:    agentbase.TruncateRunes(text, commentMaxRunes),
			Mention: *item.Mention,
			At:      at,
		})
	}
	return Remark{Comments: out}, nil
}

// parseAt 解析模型指定的候选：兼容字符串（名字）与数字（候选序号），最多 3 个。
func parseAt(raw []json.RawMessage) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	if len(raw) > mentionMax {
		return nil, fmt.Errorf("最多 %d 个候选", mentionMax)
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		var name string
		if err := json.Unmarshal(item, &name); err == nil {
			if name = strings.TrimSpace(name); name != "" {
				out = append(out, name)
				continue
			}
			return nil, fmt.Errorf("候选名字为空")
		}
		var index int
		if err := json.Unmarshal(item, &index); err == nil && index > 0 {
			out = append(out, strconv.Itoa(index))
			continue
		}
		return nil, fmt.Errorf("无法识别的候选: %s", string(item))
	}
	return out, nil
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
