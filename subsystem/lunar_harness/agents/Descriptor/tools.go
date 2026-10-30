package Descriptor

// tools.go 通用描述的文件查询工具（file_summary / file_outline / read_lines / file_references）

import (
	"encoding/json"
	"fmt"
	"strings"
)

// fileToolDefs 文件查询工具定义
func fileToolDefs() []toolDef {
	noArgs := map[string]any{
		"type":       "object",
		"properties": map[string]any{},
		"required":   []string{},
	}
	return []toolDef{
		{
			Type: "function",
			Function: toolFunctionDef{
				Name:        "file_summary",
				Description: "读取该文件的已有摘要；没有摘要时返回文件规模、结构要点与开头片段，供你快速建立整体印象。",
				Parameters:  noArgs,
			},
		},
		{
			Type: "function",
			Function: toolFunctionDef{
				Name:        "file_outline",
				Description: "返回文件的结构大纲：每一项都带行区间（L起始-结束）与内容标签，用于定位「从哪一行到哪一行是什么内容」。",
				Parameters:  noArgs,
			},
		},
		{
			Type: "function",
			Function: toolFunctionDef{
				Name:        "read_lines",
				Description: "按行区间读取正文片段。文件较大时必须分片读取；单次最多返回 300 行 / 12000 字，超出会截断并提示。",
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
			Function: toolFunctionDef{
				Name:        "file_references",
				Description: "返回该文件引用的其它文件：脚本 import / require、C 系 include、Markdown 链接、HTML src/href 等，并给出行号与解析结果。",
				Parameters:  noArgs,
			},
		},
	}
}

// executeFileTool 执行一次文件查询工具并返回文本结果
func executeFileTool(doc *docContext, call toolCall) string {
	switch call.Function.Name {
	case "file_summary":
		return docSummary(doc)
	case "file_outline":
		return formatOutline(doc.Outline, 0)
	case "file_references":
		return formatReferences(doc.References, 0)
	case "read_lines":
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
	fmt.Fprintf(&builder, "【文件规模】%d 行 / %d 字\n", len(doc.Lines), len([]rune(doc.Text)))
	if len(doc.Outline) > 0 {
		builder.WriteString("【结构要点】\n")
		builder.WriteString(formatOutline(doc.Outline, 12))
		builder.WriteString("\n")
	}
	builder.WriteString("【开头片段】\n")
	builder.WriteString(hardCutRunes(firstMeaningfulLines(doc.Lines, 8), 800))
	builder.WriteString("\n（尚无现成摘要：请结合结构要点与正文片段给出简述）")
	return builder.String()
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
		text = hardCutRunes(text, maxToolReadChars)
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
