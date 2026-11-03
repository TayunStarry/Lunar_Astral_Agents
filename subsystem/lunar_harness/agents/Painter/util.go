package Painter

// util.go 工具参数解析、路径归一化与资源句柄查询。

import (
	"encoding/json"
	"math/rand"
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/resource"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// parseToolArgs 容错解析工具参数（对象直接使用，字符串交给基座解析）。
func parseToolArgs(raw any) map[string]any {
	switch value := raw.(type) {
	case map[string]any:
		return value
	case string:
		if parsed, err := agentbase.ParseToolArgs(value); err == nil {
			return parsed
		}
	}
	return map[string]any{}
}

// stringArg 读取字符串参数。
func stringArg(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	if value, ok := args[key].(string); ok {
		return value
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

// stringListArg 读取字符串数组参数（兼容单个字符串）。
func stringListArg(args map[string]any, key string) []string {
	if args == nil {
		return nil
	}
	switch value := args[key].(type) {
	case []string:
		return value
	case []any:
		out := make([]string, 0, len(value))
		for _, item := range value {
			if text, ok := item.(string); ok {
				out = append(out, text)
			}
		}
		return out
	case string:
		if strings.TrimSpace(value) == "" {
			return nil
		}
		return []string{value}
	}
	return nil
}

// randomFloor 生成区间内的随机整数（含端点）。
func randomFloor(min, max int) int {
	if max <= min {
		return min
	}
	return rand.Intn(max-min+1) + min
}

// truncateRunes 按字符截断并追加省略号，统一使用基座实现。
func truncateRunes(text string, limit int) string {
	if limit <= 0 {
		return text
	}
	return agentbase.TruncateRunes(text, limit)
}

func pick[T any](condition bool, whenTrue, whenFalse T) T {
	if condition {
		return whenTrue
	}
	return whenFalse
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func defaultFloat(value, fallback float64) float64 {
	if value <= 0 {
		return fallback
	}
	return value
}

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

func toInt64Value(value any) int64 {
	switch number := value.(type) {
	case float64:
		return int64(number)
	case int:
		return int64(number)
	case int64:
		return number
	}
	return 0
}

// localPathToRel 把 /file/read/ 地址或本地路径归一化为 multimedia/ 相对路径。
func localPathToRel(source string) string {
	normalized := strings.ReplaceAll(source, "\\", "/")
	if cut := strings.IndexAny(normalized, "?#"); cut >= 0 {
		normalized = normalized[:cut]
	}
	lower := strings.ToLower(normalized)
	if marker := strings.LastIndex(lower, "/multimedia/"); marker >= 0 {
		return normalized[marker+1:]
	}
	if strings.HasPrefix(lower, "multimedia/") {
		return normalized
	}
	return ""
}

// readURLOf 本地路径 → /file/read/ 地址（仅 multimedia/ 下）。
func readURLOf(source string) string {
	rel := localPathToRel(source)
	if rel == "" {
		return ""
	}
	return fileReadPath + rel
}

// listImages 列出最近的图片资源。
func listImages() []resource.Resource {
	entries, err := resource.List(imageKind, listLimit)
	if err != nil {
		logger.Warn(module, "[资源] 列目录失败: %v", err)
		return nil
	}
	return entries
}

// resolveImageRef 把图片句柄解析为可取用来源。
func resolveImageRef(raw string) (handle, source, desc string, ok bool) {
	parsed, valid := resource.ParseHandle(raw)
	if !valid || parsed.Kind != imageKind {
		return "", "", "", false
	}
	normalized := resource.FormatHandle(parsed.Kind, parsed.Seq)
	entry, found := resource.Lookup(normalized)
	if !found {
		return "", "", "", false
	}
	if entry.State == resource.StateLost {
		_ = resource.Delete(normalized)
		return "", "", "", false
	}
	if entry.State == "active" && entry.BlobPath != "" {
		source = entry.BlobPath
	}
	return normalized, source, entry.Summary, true
}

// sanitizeImageDesc 规整图片描述，保证可安全嵌入占位。
func sanitizeImageDesc(raw string, maxChars int) string {
	text := strings.Join(strings.Fields(raw), " ")
	if text == "" {
		return ""
	}
	text = strings.TrimLeft(text, "「『\"'“”‘’")
	text = strings.TrimRight(text, "」』\"'“”‘’")
	text = strings.TrimSpace(text)
	replacer := strings.NewReplacer("[", "", "]", "", "【", "", "】", "", "〔", "", "〕", "", "（", "", "）", "", "(", "", ")", "")
	text = replacer.Replace(text)
	text = strings.TrimSpace(text)
	text = strings.TrimLeft(text, "，。、；：,.!?！？~～…-")
	text = strings.TrimRight(text, "，。、；：,.!?！？~～…-")
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	runes := []rune(text)
	if len(runes) > maxChars {
		return strings.TrimSpace(string(runes[:maxChars])) + "…"
	}
	return text
}
