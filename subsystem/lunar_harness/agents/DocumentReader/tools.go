package DocumentReader

// tools.go 文献解析的查询工具：摘要 / 大纲 / 按行读取 / 引用 / 格式画像 / 分批计划 / 按片读取

import (
	"encoding/json"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/engine/model/agentchat"
)

// fileToolDefs 文件查询工具定义
func fileToolDefs() []agentchat.ToolDefinition {
	noArgs := map[string]any{
		"type":       "object",
		"properties": map[string]any{},
		"required":   []string{},
	}
	return []agentchat.ToolDefinition{
		{
			Type: "function",
			Function: agentchat.ToolFunctionDef{
				Name:        ToolSummary,
				Description: "读取该文件的已有摘要与自动识别出的格式画像；没有摘要时返回文件规模、结构要点与开头片段。",
				Parameters:  noArgs,
			},
		},
		{
			Type: "function",
			Function: agentchat.ToolFunctionDef{
				Name:        ToolProfile,
				Description: "返回自动识别出的内容格式：按扩展名、内容结构、文件大小判定类型与处理策略。",
				Parameters:  noArgs,
			},
		},
		{
			Type: "function",
			Function: agentchat.ToolFunctionDef{
				Name:        ToolOutline,
				Description: "返回文件的结构大纲：每一项都带行区间（L起始-结束）与内容标签，用于定位「从哪一行到哪一行是什么内容」。",
				Parameters:  noArgs,
			},
		},
		{
			Type: "function",
			Function: agentchat.ToolFunctionDef{
				Name:        ToolChunkPlan,
				Description: "返回分批处理计划：文件超过 10000 字时按字符上限切分，每片给出行区间与字数，供 read_chunk 逐片读取。",
				Parameters:  noArgs,
			},
		},
		{
			Type: "function",
			Function: agentchat.ToolFunctionDef{
				Name:        ToolReadChunk,
				Description: "按片序号读取一个分批片段（单次最多返回 10000 字）；适合超长文件的顺序处理。",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"index": map[string]any{
							"type":        "integer",
							"description": "片序号（从 0 开始，见 chunk_plan）",
						},
					},
					"required": []string{"index"},
				},
			},
		},
		{
			Type: "function",
			Function: agentchat.ToolFunctionDef{
				Name:        ToolReadLines,
				Description: "按行区间读取正文片段。文件较大时必须分片读取；单次最多返回 300 行 / 10000 字，超出会截断并提示。",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"start": map[string]any{
							"type":        "integer",
							"description": "起始行号（1 基，含）",
						},
						"end": map[string]any{
							"type":        "integer",
							"description": "结束行号（1 基，含）",
						},
					},
					"required": []string{"start", "end"},
				},
			},
		},
		{
			Type: "function",
			Function: agentchat.ToolFunctionDef{
				Name:        ToolReferences,
				Description: "返回该文件引用的其它文件：脚本 import / require、C 系 include、Markdown 链接、HTML src/href 等，并给出行号与解析结果。",
				Parameters:  noArgs,
			},
		},
	}
}

// executeFileTool 执行一次文件查询工具并返回文本结果
func executeFileTool(doc *docContext, call agentchat.ToolCall) string {
	switch call.Function.Name {
	case ToolSummary:
		return docSummary(doc)
	case ToolProfile:
		return "【格式识别】" + formatProfileText(doc.Profile)
	case ToolOutline:
		return formatOutline(doc.Outline, 0)
	case ToolReferences:
		return formatReferences(doc.References, 0)
	case ToolChunkPlan:
		return chunkPlanText(doc)
	case ToolReadChunk:
		var args struct {
			Index any `json:"index"`
		}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return fmt.Sprintf("参数解析失败：%v（需要 {\"index\":整数}）", err)
		}
		if args.Index == nil {
			return "请提供片序号（从 0 开始，见 chunk_plan）"
		}
		return readChunk(doc, toInt(args.Index))
	case ToolReadLines:
		var args struct {
			Start any `json:"start"`
			End   any `json:"end"`
		}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return fmt.Sprintf("参数解析失败：%v（需要 {\"start\":整数,\"end\":整数}）", err)
		}
		if args.Start == nil || args.End == nil {
			return "请提供 start 与 end 行号（1 基，含端点）"
		}
		return readLineRange(doc, toInt(args.Start), toInt(args.End))
	}
	return fmt.Sprintf("未知工具：%s", call.Function.Name)
}

// toInt 把模型给的数值（数字或数字字符串）转成 int
func toInt(value any) int {
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	case int64:
		return int(typed)
	case string:
		var parsed int
		if _, err := fmt.Sscanf(strings.TrimSpace(typed), "%d", &parsed); err == nil {
			return parsed
		}
	}
	return 0
}

// docSummary 返回已有摘要，缺失时给出确定性要点
func docSummary(doc *docContext) string {
	if summary := strings.TrimSpace(doc.Resource.Summary); summary != "" {
		return "【已有摘要】\n" + summary
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, "【格式识别】%s\n", formatProfileText(doc.Profile))
	fmt.Fprintf(&builder, "【文件规模】%d 行 / %d 字\n", len(doc.Lines), doc.Profile.Runes)
	if len(doc.Chunks) > 0 {
		fmt.Fprintf(&builder, "【分批计划】共 %d 片（单片 ≤ %d 字）\n", len(doc.Chunks), chunkRunes)
	}
	if len(doc.Outline) > 0 {
		builder.WriteString("【结构要点】\n")
		builder.WriteString(formatOutline(doc.Outline, 12))
		builder.WriteString("\n")
	}
	builder.WriteString("【开头片段】\n")
	builder.WriteString(agentchat.HardCutRunes(firstMeaningfulLines(doc.Lines, 8), 800))
	builder.WriteString("\n（尚无现成摘要：请结合结构要点与正文片段给出简述）")
	return builder.String()
}

// chunkPlanText 渲染分批计划
func chunkPlanText(doc *docContext) string {
	if len(doc.Chunks) == 0 {
		return "（无需分批：文件未超过 10000 字，可直接按行读取）"
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, "共 %d 片，单片 ≤ %d 字：\n", len(doc.Chunks), chunkRunes)
	limit := len(doc.Chunks)
	if limit > maxChunkPlanItems {
		limit = maxChunkPlanItems
	}
	for _, span := range doc.Chunks[:limit] {
		fmt.Fprintf(&builder, "#%d L%d-%d（%d 字）\n", span.Index, span.StartLine, span.EndLine, span.Chars)
	}
	if limit < len(doc.Chunks) {
		fmt.Fprintf(&builder, "（仅列出前 %d 片，其余请用 read_lines 按行区间继续）", limit)
	}
	return strings.TrimRight(builder.String(), "\n")
}

// readChunk 按片序号读取一个分批片段（按字符上限双重限流）
func readChunk(doc *docContext, index int) string {
	if index < 0 || index >= len(doc.Chunks) {
		return fmt.Sprintf("片序号超出范围：共 %d 片（0-%d）", len(doc.Chunks), len(doc.Chunks)-1)
	}
	span := doc.Chunks[index]
	start := span.StartLine
	if start < 1 {
		start = 1
	}
	end := span.EndLine
	if end > len(doc.Lines) {
		end = len(doc.Lines)
	}
	text := strings.Join(doc.Lines[start-1:end], "\n")
	truncated := false
	if len([]rune(text)) > maxToolReadChars {
		text = agentchat.HardCutRunes(text, maxToolReadChars)
		truncated = true
	}
	note := ""
	if truncated {
		note = "\n（本片已按字符上限截断，可缩小行区间继续读）"
	}
	return fmt.Sprintf("#%d L%d-%d：\n%s%s", index, start, end, text, note)
}

// readLineRange 读取指定行区间（按行数与字符数双重限流）
func readLineRange(doc *docContext, start, end int) string {
	total := len(doc.Lines)
	if total == 0 {
		return "（文件为空）"
	}
	if start < 1 {
		start = 1
	}
	if end < start {
		end = start
	}
	if start > total {
		return fmt.Sprintf("行区间超出范围：文件共 %d 行", total)
	}
	if end > total {
		end = total
	}
	if end-start+1 > maxToolReadLines {
		end = start + maxToolReadLines - 1
	}
	text := strings.Join(doc.Lines[start-1:end], "\n")
	truncated := false
	if len([]rune(text)) > maxToolReadChars {
		text = agentchat.HardCutRunes(text, maxToolReadChars)
		truncated = true
	}
	note := ""
	if truncated {
		note = "\n（本次读取已按字符上限截断，可缩小行区间继续读）"
	}
	return fmt.Sprintf("L%d-%d：\n%s%s", start, end, text, note)
}

// firstMeaningfulLines 取前 n 个非空行
func firstMeaningfulLines(lines []string, n int) string {
	picked := make([]string, 0, n)
	for _, line := range lines {
		trimmed := strings.TrimRight(line, "\r")
		if strings.TrimSpace(trimmed) == "" {
			continue
		}
		picked = append(picked, trimmed)
		if len(picked) >= n {
			break
		}
	}
	return strings.Join(picked, "\n")
}

// truncateRunes 按字符截断
func truncateRunes(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "…"
}
