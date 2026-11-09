package WishManager

// tools.go 愿望管理工具：查询 / 新增 / 改写 / 完成 / 删除 / 调整优先级。
//
// 待实现愿望最多 16 条；新增时若已达上限，返回 NeedConfirm 并附上当前完整列表，
// 由人格对话向对话者确认需要保留哪些愿望。

import (
	"errors"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/chain/wishchain"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// toolDefinitions 愿望管理工具定义。
func toolDefinitions() []agentbase.ToolDefinition {
	object := func(props map[string]any, required ...string) map[string]any {
		return map[string]any{"type": "object", "properties": props, "required": required}
	}
	text := func(description string) map[string]any {
		return map[string]any{"type": "string", "description": description}
	}
	integer := func(description string) map[string]any {
		return map[string]any{"type": "integer", "description": description}
	}
	return []agentbase.ToolDefinition{
		{Type: "function", Function: agentbase.ToolFunctionDef{
			Name:        ToolQuery,
			Description: "查询愿望清单：可按关键词筛选；不确定愿望 ID 时先用它查看当前完整的待实现列表。",
			Parameters: object(map[string]any{
				"keywords": map[string]any{
					"type":        "array",
					"description": "关键词（可空，空表示全部）",
					"items":       map[string]any{"type": "string"},
				},
			}),
		}},
		{Type: "function", Function: agentbase.ToolFunctionDef{
			Name:        ToolAdd,
			Description: "新增一条愿望：content 为愿望内容，due 为期望达成时间（可空），priority 为 0-15（0 最高，可空自动分配）。",
			Parameters: object(map[string]any{
				"content":  text("愿望内容"),
				"due":      text("期望达成时间（如 2026-06-14 15:30；可空）"),
				"priority": integer("优先级 0-15，0 最高；可空时自动分配"),
			}, "content"),
		}},
		{Type: "function", Function: agentbase.ToolFunctionDef{
			Name:        ToolUpdate,
			Description: "改写已有愿望：id 必填；content / due / priority 至少提供一个。",
			Parameters: object(map[string]any{
				"id":       text("愿望 ID"),
				"content":  text("新的愿望内容（可空）"),
				"due":      text("新的期望达成时间（可空）"),
				"priority": integer("新的优先级 0-15（可空）"),
			}, "id"),
		}},
		{Type: "function", Function: agentbase.ToolFunctionDef{
			Name:        ToolComplete,
			Description: "把一条愿望标记为已完成：id 必填。",
			Parameters: object(map[string]any{
				"id": text("愿望 ID"),
			}, "id"),
		}},
		{Type: "function", Function: agentbase.ToolFunctionDef{
			Name:        ToolDelete,
			Description: "删除一条愿望：id 必填。",
			Parameters: object(map[string]any{
				"id": text("愿望 ID"),
			}, "id"),
		}},
		{Type: "function", Function: agentbase.ToolFunctionDef{
			Name:        ToolPriority,
			Description: "只调整愿望优先级：id 必填，priority 为 0-15（0 最高）。",
			Parameters: object(map[string]any{
				"id":       text("愿望 ID"),
				"priority": integer("优先级 0-15，0 最高"),
			}, "id", "priority"),
		}},
	}
}

// executeTool 执行一次愿望管理工具。
func executeTool(call agentbase.ToolCall) Result {
	args, err := agentbase.ParseToolArgs(call.Function.Arguments)
	if err != nil {
		return failResult(call.Function.Name, fmt.Errorf("参数不是合法 JSON"))
	}
	switch call.Function.Name {
	case ToolQuery:
		text := wishchain.Query(argStringList(args, "keywords"))
		return Result{
			OK:      true,
			Action:  "query",
			Handles: []string{},
			Summary: previewRunes(text, 512),
			Detail:  map[string]any{"result": text},
		}
	case ToolAdd:
		item, addErr := wishchain.Add(argString(args, "content"), argString(args, "due"), argPriority(args))
		if addErr != nil {
			if errors.Is(addErr, wishchain.ErrCapacity) {
				return capacityResult()
			}
			return failResult(ToolAdd, addErr)
		}
		return okResult("add", item, fmt.Sprintf("新增愿望 %s：P%d %s", item.ID, item.Priority, item.Content))
	case ToolUpdate:
		item, updateErr := wishchain.Update(argString(args, "id"), argString(args, "content"), argString(args, "due"), argPriority(args))
		if updateErr != nil {
			return failResult(ToolUpdate, updateErr)
		}
		return okResult("update", item, fmt.Sprintf("改写愿望 %s：P%d %s", item.ID, item.Priority, item.Content))
	case ToolComplete:
		item, completeErr := wishchain.Complete(argString(args, "id"))
		if completeErr != nil {
			return failResult(ToolComplete, completeErr)
		}
		return okResult("complete", item, fmt.Sprintf("愿望已完成 %s：%s", item.ID, item.Content))
	case ToolDelete:
		item, deleteErr := wishchain.Delete(argString(args, "id"))
		if deleteErr != nil {
			return failResult(ToolDelete, deleteErr)
		}
		return okResult("delete", item, fmt.Sprintf("删除愿望 %s：%s", item.ID, item.Content))
	case ToolPriority:
		priority, ok := argInt(args, "priority")
		if !ok {
			return failResult(ToolPriority, fmt.Errorf("priority 必须是 0-15 的整数"))
		}
		item, priorityErr := wishchain.SetPriority(argString(args, "id"), priority)
		if priorityErr != nil {
			return failResult(ToolPriority, priorityErr)
		}
		return okResult("priority", item, fmt.Sprintf("调整愿望 %s 优先级为 P%d：%s", item.ID, item.Priority, item.Content))
	default:
		return failResult(call.Function.Name, fmt.Errorf("未知工具"))
	}
}

// capacityResult 容量上限的确认结果。
func capacityResult() Result {
	items := wishchain.List()
	return Result{
		OK:          false,
		Action:      "add",
		Handles:     []string{},
		NeedConfirm: true,
		Summary: fmt.Sprintf("待实现愿望已达上限（%d 条），请向对话者确认要保留哪些愿望，删除或完成腾出位置后再新增。",
			wishchain.MaxPending),
		Detail: map[string]any{
			"capacity": wishchain.MaxPending,
			"wishes":   wishTexts(items),
			"hint":     "请把当前列表展示给对话者，询问保留哪些、舍弃哪些；确认后再删除/完成并重试新增。",
		},
	}
}

// okResult 构造成功结果。
func okResult(action string, item wishchain.Wish, summary string) Result {
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
		Action:  strings.TrimPrefix(op, "wish_"),
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

// argInt 读取整数参数。
func argInt(args map[string]any, key string) (int, bool) {
	switch value := args[key].(type) {
	case float64:
		return int(value), true
	case int:
		return value, true
	case int64:
		return int(value), true
	}
	return 0, false
}

// argPriority 读取优先级；缺省返回自动分配。
func argPriority(args map[string]any) int {
	if value, ok := argInt(args, "priority"); ok {
		return value
	}
	return wishchain.AutoPriority
}

// previewRunes 按字符截断。
func previewRunes(text string, limit int) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "…"
}
