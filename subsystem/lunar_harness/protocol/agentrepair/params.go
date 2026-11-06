package agentrepair

// params.go 修复动作参数的宽松读取。
//
// 参数由模型生成的 JSON 传入，数字统一解析为 float64；各目标包不应各写一份
// 类型断言，这里集中提供带默认值与区间夹取的小工具。

import (
	"encoding/json"
	"strings"
)

// IntParam 读取整数参数；缺失或非法时返回 def，并夹取到 [low, high]（high<=0 表示不设上限）。
func IntParam(params map[string]any, key string, def, low, high int) int {
	value, ok := params[key]
	if !ok {
		return def
	}
	number := 0
	switch typed := value.(type) {
	case float64:
		number = int(typed)
	case int:
		number = typed
	case int64:
		number = int(typed)
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil {
			return def
		}
		number = int(parsed)
	default:
		return def
	}
	if number < low {
		number = low
	}
	if high > 0 && number > high {
		number = high
	}
	return number
}

// StringParam 读取字符串参数并去除首尾空白。
func StringParam(params map[string]any, key string) string {
	value, ok := params[key]
	if !ok {
		return ""
	}
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

// BoolParam 读取布尔参数；缺失或非法时返回 def。
func BoolParam(params map[string]any, key string, def bool) bool {
	value, ok := params[key]
	if !ok {
		return def
	}
	flag, ok := value.(bool)
	if !ok {
		return def
	}
	return flag
}
