package file

// args.go 工具型智能体共用的参数解析、结果构造与 JSON Schema 辅助。

import (
	"encoding/json"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// StringArg 读取字符串参数并去除首尾空白。
func StringArg(args map[string]any, key string) string {
	value, _ := args[key].(string)
	return strings.TrimSpace(value)
}

// IntArg 读取整数参数。
func IntArg(args map[string]any, key string) int {
	switch value := args[key].(type) {
	case float64:
		return int(value)
	case int:
		return value
	case int64:
		return int(value)
	}
	return 0
}

// BoolArg 读取布尔参数。
func BoolArg(args map[string]any, key string) bool {
	value, _ := args[key].(bool)
	return value
}

// StringSliceArg 读取字符串数组参数。
func StringSliceArg(args map[string]any, key string) []string {
	raw, ok := args[key].([]any)
	if !ok {
		if list, ok := args[key].([]string); ok {
			return append([]string(nil), list...)
		}
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
			out = append(out, strings.TrimSpace(text))
		}
	}
	return out
}

// JSON 把值序列化为 JSON 字符串。
func JSON(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(raw)
}

// Failure 构造失败工具结果。
func Failure(name string, err error) Outcome {
	return Outcome{
		JSON:    JSON(map[string]any{"error": err.Error()}),
		Summary: name + " 失败：" + err.Error(),
	}
}

// Def 构造函数工具声明。
func Def(name, description string, properties map[string]any, required []string) agentbase.ToolDefinition {
	if required == nil {
		required = []string{}
	}
	return agentbase.ToolDefinition{Type: "function", Function: agentbase.ToolFunctionDef{
		Name:        name,
		Description: description,
		Parameters: map[string]any{
			"type":       "object",
			"properties": properties,
			"required":   required,
		},
	}}
}

// StrProp 字符串属性。
func StrProp(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

// IntProp 整数属性。
func IntProp(description string, minimum, maximum int) map[string]any {
	return map[string]any{"type": "integer", "description": description, "minimum": minimum, "maximum": maximum}
}

// BoolProp 布尔属性。
func BoolProp(description string) map[string]any {
	return map[string]any{"type": "boolean", "description": description}
}

// ArrayProp 字符串数组属性。
func ArrayProp(description string) map[string]any {
	return map[string]any{
		"type":        "array",
		"description": description,
		"items":       map[string]any{"type": "string"},
	}
}
