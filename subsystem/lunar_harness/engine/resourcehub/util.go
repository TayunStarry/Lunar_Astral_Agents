package resourcehub

// util.go 统一资源基座的小工具：类型码、句柄规整、参数解析与文本格式化。
//
// 句柄文法、类型码中文名全部委托 engine/resource，不在本包维护第二份正则或标签表。

import (
	"fmt"
	"path/filepath"
	"strings"

	"LunarSubsystem/LunarHarness/engine/resource"
)

// kindLabel 类型码的中文名
func kindLabel(kind string) string {
	return resource.KindLabel(kind)
}

// isKnownKind 判断类型码是否为已定义的类型（D/I/V/A/G/U）
func isKnownKind(kind string) bool {
	return kind != "" && kindLabel(kind) != kind
}

// parseHandleText 解析并返回规范句柄文本（非法句柄返回空串）
func parseHandleText(raw string) string {
	handle, ok := resource.ParseHandle(raw)
	if !ok || handle.Seq <= 0 {
		return ""
	}
	return handle.FullText()
}

// entryExt 资源扩展名（记录为空时按文件名推导）
func entryExt(entry resource.Resource) string {
	if ext := strings.TrimSpace(entry.Ext); ext != "" {
		return strings.ToLower(ext)
	}
	return strings.ToLower(filepath.Ext(entry.Name))
}

// mediaName 从来源推断展示文件名（无扩展名时补 fallbackExt）
func mediaName(source, fallbackExt string) string {
	base := filepath.Base(strings.ReplaceAll(pathPartOf(source), `\`, "/"))
	if base == "" || base == "." || base == "/" {
		base = "media" + fallbackExt
	}
	if filepath.Ext(base) == "" && fallbackExt != "" {
		base += fallbackExt
	}
	return base
}

// pathPartOf 取来源中路径部分（去掉查询参数与锚点）
func pathPartOf(source string) string {
	text := strings.TrimSpace(source)
	if index := strings.IndexAny(text, "?#"); index >= 0 {
		text = text[:index]
	}
	return text
}

// previewRunes 按字符截断文本（不追加省略号，保持原文可直接引用）
func previewRunes(text string, limit int) string {
	runes := []rune(text)
	if limit <= 0 || len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "…"
}

// formatBytes 字节数格式化
func formatBytes(bytes int64) string {
	if bytes <= 0 {
		return "0 B"
	}
	units := []string{"B", "KB", "MB", "GB"}
	value := float64(bytes)
	unit := 0
	for value >= 1024 && unit < len(units)-1 {
		value /= 1024
		unit++
	}
	if value >= 100 || unit == 0 {
		return fmt.Sprintf("%.0f %s", value, units[unit])
	}
	return fmt.Sprintf("%.1f %s", value, units[unit])
}

// dedupeStrings 去重并保持首次出现顺序。
func dedupeStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

// numberArg 读取数值参数
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
	}
	return 0
}

// stringListArg 读取字符串数组参数（兼容单个字符串）
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
			if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
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

// failure 构造统一失败结果（Text 与 Error 都给出原因，便于不同调用方取用）
func failure(message string) Result {
	return Result{Text: message, Handle: []string{}, Error: message}
}
