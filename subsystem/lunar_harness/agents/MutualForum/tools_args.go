package MutualForum

// tools_args.go 工具调用参数的宽松读取。

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

/* ---------------- 参数读取 ---------------- */

func stringArg(args map[string]any, key string) string {
	value, ok := args[key]
	if !ok || value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	return strings.TrimSpace(fmt.Sprintf("%v", value))
}

func intArg(args map[string]any, key string) int {
	switch value := args[key].(type) {
	case float64:
		return int(value)
	case int:
		return value
	case int64:
		return int(value)
	case json.Number:
		parsed, _ := value.Int64()
		return int(parsed)
	case string:
		parsed, _ := strconv.Atoi(strings.TrimSpace(value))
		return parsed
	default:
		return 0
	}
}

func int64Arg(args map[string]any, key string) int64 {
	return int64(intArg(args, key))
}

func boolArg(args map[string]any, key string) bool {
	switch value := args[key].(type) {
	case bool:
		return value
	case string:
		return strings.EqualFold(strings.TrimSpace(value), "true")
	case float64:
		return value != 0
	default:
		return false
	}
}

func stringSliceArg(args map[string]any, key string) []string {
	value, ok := args[key]
	if !ok || value == nil {
		return nil
	}
	switch items := value.(type) {
	case []string:
		return normalizeMentions(items)
	case []any:
		out := make([]string, 0, len(items))
		for _, item := range items {
			if text, ok := item.(string); ok {
				out = append(out, text)
			}
		}
		return normalizeMentions(out)
	case string:
		if strings.TrimSpace(items) == "" {
			return nil
		}
		return normalizeMentions(strings.Split(items, ","))
	default:
		return nil
	}
}

func intSliceArg(args map[string]any, key string) []int {
	value, ok := args[key]
	if !ok || value == nil {
		return nil
	}
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]int, 0, len(items))
	for _, item := range items {
		switch number := item.(type) {
		case float64:
			out = append(out, int(number))
		case int:
			out = append(out, number)
		case string:
			if parsed, err := strconv.Atoi(strings.TrimSpace(number)); err == nil {
				out = append(out, parsed)
			}
		}
	}
	return out
}
