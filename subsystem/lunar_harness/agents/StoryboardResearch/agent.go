package StoryboardResearch

// agent.go 分镜规划-资料：判断剧情需要补全哪些现实资料，产出检索意图。
//
// 模型调用、推理实况、循环监管与失败上报统一走 protocol/agentbase：
// 调用方（演绎链）只拿结构化结果，不再自行构造请求或解析模型流。

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
	Timeout:   researchTimeout,
	MaxRounds: 1,
})

// Plan 读取故事并产出资料准备计划。
//
// 输出分两段：先可读的检索判断（进入推理实况），再是结构化检索意图。
func Plan(ctx context.Context, story string) (result Result, err error) {
	defer func() { agentbase.ReportRun(AgentID, err) }()
	if ctx == nil {
		ctx = context.Background()
	}
	story = clipRunes(strings.TrimSpace(story), maxStoryRunes)
	if story == "" {
		return Result{}, fmt.Errorf("故事正文为空")
	}
	system := loadPrompt(promptName)
	if system == "" {
		return Result{}, fmt.Errorf("读取资料员提示词失败")
	}
	temperature := 0.3
	reply, err := base.Stream(ctx, agentbase.StreamRequest{
		Messages: []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": userPrompt(story)},
		},
		Temperature: &temperature,
	})
	if err != nil {
		return Result{}, err
	}
	text := strings.TrimSpace(reply.Text)
	if text == "" {
		return Result{}, fmt.Errorf("模型返回空内容")
	}
	result = parsePlan(text)
	if result.Analysis == "" && len(result.Queries) == 0 {
		return Result{}, fmt.Errorf("模型没有返回可用的资料判断")
	}
	return result, nil
}

// userPrompt 拼装资料员的用户消息。
func userPrompt(story string) string {
	return "故事正文：\n" + story +
		"\n\n请先给出检索判断，然后输出分隔行 `" + queryDelimiter +
		"`，最后输出一行 JSON 检索意图。"
}

// parsePlan 从模型正文中拆出可读判断与检索意图。
func parsePlan(text string) Result {
	analysis := text
	tail := ""
	if index := strings.Index(text, queryDelimiter); index >= 0 {
		analysis = strings.TrimSpace(text[:index])
		tail = strings.TrimSpace(text[index+len(queryDelimiter):])
	}
	queries := parseQueries(tail)
	if queries == nil && tail == "" {
		// 没有分隔行时，兼容整段就是一个 JSON 对象的老式输出。
		if body := extractJSONObject(text); body != "" {
			var raw struct {
				Analysis string   `json:"analysis"`
				Queries  []string `json:"queries"`
			}
			if err := json.Unmarshal([]byte(body), &raw); err == nil {
				if strings.TrimSpace(raw.Analysis) != "" {
					analysis = strings.TrimSpace(raw.Analysis)
				}
				queries = normalizeQueries(raw.Queries)
			}
		}
	}
	return Result{Analysis: strings.TrimSpace(analysis), Queries: queries}
}

// parseQueries 解析检索词数组；无法解析时返回 nil。
func parseQueries(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	body := extractJSONObject(text)
	if body == "" {
		start := strings.Index(text, "[")
		end := strings.LastIndex(text, "]")
		if start < 0 || end <= start {
			return nil
		}
		body = text[start : end+1]
	}
	var raw struct {
		Queries []string `json:"queries"`
	}
	if err := json.Unmarshal([]byte(body), &raw); err == nil && raw.Queries != nil {
		return normalizeQueries(raw.Queries)
	}
	var list []string
	if err := json.Unmarshal([]byte(body), &list); err == nil {
		return normalizeQueries(list)
	}
	return nil
}

// normalizeQueries 去空、去重并限制条数。
func normalizeQueries(queries []string) []string {
	out := make([]string, 0, len(queries))
	seen := make(map[string]bool, len(queries))
	for _, query := range queries {
		query = clipRunes(strings.TrimSpace(query), maxQueryRunes)
		if query == "" || seen[query] {
			continue
		}
		seen[query] = true
		out = append(out, query)
		if len(out) >= maxQueries {
			break
		}
	}
	return out
}

// extractJSONObject 从正文中截取第一个 JSON 对象。
func extractJSONObject(text string) string {
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
