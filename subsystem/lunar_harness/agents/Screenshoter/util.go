package Screenshoter

// util.go 截图者的参数取值辅助。

import (
	"encoding/json"
	"strings"
)

// stringArg 读取字符串参数。
func stringArg(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	if value, ok := args[key].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

// numberArg 读取数值参数（兼容 float64 / json.Number / 整型）。
func numberArg(args map[string]any, key string) float64 {
	if args == nil {
		return 0
	}
	switch value := args[key].(type) {
	case float64:
		return value
	case float32:
		return float64(value)
	case int:
		return float64(value)
	case int64:
		return float64(value)
	case json.Number:
		f, _ := value.Float64()
		return f
	}
	return 0
}

// boolArg 读取布尔参数。
func boolArg(args map[string]any, key string) bool {
	if args == nil {
		return false
	}
	value, _ := args[key].(bool)
	return value
}

// defaultString 空串时返回兜底值。
func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

// toIntValue 把引擎返回的数值转为 int。
func toIntValue(value any) int {
	switch number := value.(type) {
	case float64:
		return int(number)
	case int:
		return number
	case int64:
		return int(number)
	}
	return 0
}
