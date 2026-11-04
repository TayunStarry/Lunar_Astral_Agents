package PlanManager

// tools.go 计划管理工具：查询 / 新增 / 改写 / 删除计划表。
// 模型可以先 plan_query 查看目标时间段，再决定如何增删改。

import (
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/engine/plans"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// toolDefinitions 计划管理工具定义。
func toolDefinitions() []agentbase.ToolDefinition {
	object := func(props map[string]any, required ...string) map[string]any {
		return map[string]any{"type": "object", "properties": props, "required": required}
	}
	text := func(description string) map[string]any {
		return map[string]any{"type": "string", "description": description}
	}
	return []agentbase.ToolDefinition{
		{Type: "function", Function: agentbase.ToolFunctionDef{
			Name:        ToolQuery,
			Description: "查询计划表：可按日期范围与关键词筛选；不确定目标时间段时先用它查看已有计划。",
			Parameters: object(map[string]any{
				"keywords": map[string]any{
					"type":        "array",
					"description": "关键词（可空）",
					"items":       map[string]any{"type": "string"},
				},
				"date":  text("日期，如 2026-06-14 / 今天 / 明天；可空"),
				"scope": text("日期为空时的粒度：day / month / year / all"),
			}),
		}},
		{Type: "function", Function: agentbase.ToolFunctionDef{
			Name:        ToolCreate,
			Description: "新增一条计划项：time 为绝对时间，content 为计划内容。",
			Parameters: object(map[string]any{
				"time":    text("计划执行时间（如 2026-06-14 15:30）"),
				"content": text("计划内容"),
			}, "time", "content"),
		}},
		{Type: "function", Function: agentbase.ToolFunctionDef{
			Name:        ToolUpdate,
			Description: "改写已有计划项：id 必填，time 与 content 至少提供一个。",
			Parameters: object(map[string]any{
				"id":      text("计划项 ID"),
				"time":    text("新的执行时间（可空）"),
				"content": text("新的计划内容（可空）"),
			}, "id"),
		}},
		{Type: "function", Function: agentbase.ToolFunctionDef{
			Name:        ToolDelete,
			Description: "删除一条计划项：id 必填。",
			Parameters: object(map[string]any{
				"id": text("计划项 ID"),
			}, "id"),
		}},
	}
}

// executeTool 执行一次计划管理工具。
func executeTool(call agentbase.ToolCall) Result {
	args, err := agentbase.ParseToolArgs(call.Function.Arguments)
	if err != nil {
		return failResult(call.Function.Name, fmt.Errorf("参数不是合法 JSON"))
	}
	switch call.Function.Name {
	case ToolQuery:
		text, queryErr := plans.Query(argStringList(args, "keywords"), argString(args, "date"), argString(args, "scope"))
		if queryErr != nil {
			return failResult(ToolQuery, queryErr)
		}
		return Result{
			OK:      true,
			Action:  "query",
			Handles: []string{},
			Summary: previewRunes(text, 512),
			Detail:  map[string]any{"result": text},
		}
	case ToolCreate:
		item, createErr := plans.Create(argString(args, "time"), argString(args, "content"))
		if createErr != nil {
			return failResult(ToolCreate, createErr)
		}
		return okResult("create", item, fmt.Sprintf("新增计划项 %s：%s %s", item.ID, item.Time, item.Content))
	case ToolUpdate:
		item, updateErr := plans.Update(argString(args, "id"), argString(args, "time"), argString(args, "content"))
		if updateErr != nil {
			return failResult(ToolUpdate, updateErr)
		}
		return okResult("update", item, fmt.Sprintf("改写计划项 %s：%s %s", item.ID, item.Time, item.Content))
	case ToolDelete:
		item, deleteErr := plans.Delete(argString(args, "id"))
		if deleteErr != nil {
			return failResult(ToolDelete, deleteErr)
		}
		return okResult("delete", item, fmt.Sprintf("删除计划项 %s：%s", item.ID, item.Content))
	default:
		return failResult(call.Function.Name, fmt.Errorf("未知工具"))
	}
}

// okResult 构造成功结果。
func okResult(action string, item plans.Item, summary string) Result {
	value := item
	return Result{
		OK:      true,
		Action:  action,
		Item:    &value,
		Handles: []string{},
		Summary: summary,
		Detail:  map[string]any{"item_id": item.ID},
	}
}

// failResult 构造失败结果。
func failResult(op string, err error) Result {
	return Result{
		OK:      false,
		Action:  strings.TrimPrefix(op, "plan_"),
		Handles: []string{},
		Summary: previewRunes(op+" 失败："+err.Error(), 512),
		Detail:  map[string]any{"error": err.Error()},
	}
}

func argString(args map[string]any, key string) string {
	value, _ := args[key].(string)
	return strings.TrimSpace(value)
}

func argStringList(args map[string]any, key string) []string {
	switch value := args[key].(type) {
	case []any:
		out := make([]string, 0, len(value))
		for _, item := range value {
			if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
				out = append(out, strings.TrimSpace(text))
			}
		}
		return out
	case []string:
		return value
	case string:
		if strings.TrimSpace(value) == "" {
			return nil
		}
		return []string{strings.TrimSpace(value)}
	}
	return nil
}

// previewRunes 按字符截断。
func previewRunes(text string, limit int) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "…"
}
