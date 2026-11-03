package resourcehub

// run.go 统一资源查询入口。
//
// 四种模式共用同一套解析与渲染：
//   - memory 只查长期记忆；
//   - handle 按句柄读内容（非句柄输入自动解析为句柄）；
//   - path 按文件名 / 路径读内容（解析并登记为句柄）；
//   - list 列出符合标准的句柄数组。

import (
	"context"
	"fmt"
	"strings"
)

// RunArgs 从工具调用参数构造查询并执行（供各智能体的工具处理器直接复用）。
func RunArgs(ctx context.Context, args map[string]any) Result {
	return Run(ctx, Query{
		Type:  stringArg(args, "type"),
		Query: stringListArg(args, "query"),
		Limit: int(numberArg(args, "limit")),
	})
}

// Run 执行一次统一资源查询。
func Run(ctx context.Context, q Query) Result {
	mode := strings.ToLower(strings.TrimSpace(q.Type))
	switch mode {
	case TypeMemory:
		return runMemory(ctx, q)
	case TypeHandle:
		return runHandles(q, false)
	case TypePath:
		return runHandles(q, true)
	case TypeList:
		return runList(q)
	default:
		return failure("type 只能取 memory / handle / path / list")
	}
}

// runHandles handle / path 模式的共用实现：逐项解析并渲染。
//
// annotate 为 true 时在块前补一行「路径 → 句柄」，让模型明确看到解析结果。
func runHandles(q Query, annotate bool) Result {
	if len(q.Query) == 0 {
		if annotate {
			return failure("query 需要文件名或相对路径，例如 [\"notes/a.md\"]")
		}
		return failure("query 需要句柄，例如 [\"@D.003\"]")
	}
	limit := q.Limit
	if limit <= 0 {
		limit = defaultLimit
	}
	inputs := q.Query
	if len(inputs) > limit {
		inputs = inputs[:limit]
	}
	blocks := make([]string, 0, len(inputs))
	handles := make([]string, 0, len(inputs))
	missing := make([]string, 0)
	for _, raw := range inputs {
		// handle 模式只认句柄；文件名 / 路径由 path 模式负责。
		if !annotate && parseHandleText(raw) == "" {
			missing = append(missing, raw+"（不是句柄；按文件名 / 路径读取请用 type=\"path\"）")
			continue
		}
		block, handle, err := RenderHandle(raw, "")
		if err != nil {
			missing = append(missing, raw+"（"+err.Error()+"）")
			continue
		}
		if annotate {
			block = fmt.Sprintf("路径「%s」→ 句柄 %s\n%s", raw, handle, block)
		}
		blocks = append(blocks, block)
		handles = append(handles, handle)
	}
	if len(blocks) == 0 {
		hint := "。请先用 fetch_resources（type=\"list\"）确认可用句柄"
		message := "取回失败：" + strings.Join(missing, "、") + " —— " + handleInvalidNotice + hint
		return failure(message)
	}
	text := strings.Join(blocks, "\n\n---\n\n")
	if len(missing) > 0 {
		text += "\n\n以下目标无法取回：" + strings.Join(missing, "、") + "（" + handleInvalidNotice + "）"
	}
	return Result{Text: text, Handle: handles}
}
