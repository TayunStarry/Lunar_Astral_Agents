package ArchiveOps

// tools.go 归档操作对模型暴露的写操作工具：创建 / 删除压缩包、增删条目。
//
// 读取类能力（结构查询 / 条目读取 / GGUF）属于归档解析智能体，这里不重复提供。

import (
	"encoding/json"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/engine/model/agentchat"
)

// toolDefinitions 归档操作的函数工具定义（只列写操作）
func toolDefinitions() []agentchat.ToolDefinition {
	object := func(props map[string]any, required ...string) map[string]any {
		return map[string]any{"type": "object", "properties": props, "required": required}
	}
	text := func(description string) map[string]any {
		return map[string]any{"type": "string", "description": description}
	}
	paths := func(description string) map[string]any {
		return map[string]any{
			"type":        "array",
			"items":       map[string]any{"type": "string"},
			"description": description,
		}
	}
	return []agentchat.ToolDefinition{
		{Type: "function", Function: agentchat.ToolFunctionDef{
			Name:        OpCreate,
			Description: "创建压缩包（.zip / .ltpx）：target 为压缩包路径，sources 为要打包的文件 / 目录路径。",
			Parameters: object(map[string]any{
				"target":  text("压缩包路径，如 pkg.zip 或 /document/notes/a.zip"),
				"sources": paths("要打包的文件 / 目录路径"),
			}, "target", "sources"),
		}},
		{Type: "function", Function: agentchat.ToolFunctionDef{
			Name:        OpDelete,
			Description: "删除整个压缩包文件。",
			Parameters: object(map[string]any{
				"target": text("要删除的压缩包路径"),
			}, "target"),
		}},
		{Type: "function", Function: agentchat.ToolFunctionDef{
			Name:        OpAdd,
			Description: "在压缩包内新增或覆盖一个条目：source 与 content 二选一。",
			Parameters: object(map[string]any{
				"target":  text("压缩包路径"),
				"entry":   text("压缩包内条目名（含子目录，如 src/main.go）"),
				"source":  text("来源文件路径（与 content 二选一）"),
				"content": text("要写入的文本内容（与 source 二选一）"),
			}, "target", "entry"),
		}},
		{Type: "function", Function: agentchat.ToolFunctionDef{
			Name:        OpRemove,
			Description: "从压缩包内移除指定条目。",
			Parameters: object(map[string]any{
				"target": text("压缩包路径"),
				"entry":  text("要移除的条目名（含子目录）"),
			}, "target", "entry"),
		}},
	}
}

// executeTool 执行一条模型发起的归档操作
func executeTool(call agentchat.ToolCall) Result {
	args, err := parseToolArgs(call.Function.Arguments)
	if err != nil {
		return failResult(call.Function.Name, "", fmt.Errorf("参数不是合法 JSON"))
	}
	target := argString(args, "target")
	switch call.Function.Name {
	case OpCreate:
		return Create(target, argStringList(args, "sources"))
	case OpDelete:
		return Delete(target)
	case OpAdd:
		return AddEntry(target, argString(args, "entry"), argString(args, "source"), argString(args, "content"))
	case OpRemove:
		return RemoveEntry(target, argString(args, "entry"))
	default:
		return failResult(call.Function.Name, target, fmt.Errorf("未知工具"))
	}
}

// parseToolArgs 解析工具参数；空串视为空对象
func parseToolArgs(raw string) (map[string]any, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return map[string]any{}, nil
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return nil, err
	}
	if args == nil {
		args = map[string]any{}
	}
	return args, nil
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
