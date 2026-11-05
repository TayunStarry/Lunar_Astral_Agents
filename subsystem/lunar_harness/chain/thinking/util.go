package thinking

import (
	"encoding/json"
	"math/rand"
	"reflect"
	"strings"
)

// asItems 把消息内容归一化为内容项数组（纯字符串消息返回 nil）
//
// 必须接受任意切片类型：QQ 桥接给出的是 []map[string]any，HTTP 入口解出的是
// []any，内部构造的是 []ContentItem。只认其中一两种会让图文消息在部分入口
// 静默退化为「无内容项」——图片标签、媒体入库与日志全部跳过，原图直送模型。
func asItems(content any) []ContentItem {
	if content == nil {
		return nil
	}
	switch value := content.(type) {
	case string:
		return nil
	case []ContentItem:
		return value
	case ContentItem:
		return []ContentItem{value}
	case map[string]any:
		if item, ok := decodeContentItem(value); ok {
			return []ContentItem{item}
		}
		return nil
	}
	rv := reflect.ValueOf(content)
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return nil
	}
	items := make([]ContentItem, 0, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		element := rv.Index(i).Interface()
		switch item := element.(type) {
		case ContentItem:
			items = append(items, item)
		case *ContentItem:
			if item != nil {
				items = append(items, *item)
			}
		case map[string]any:
			if parsed, ok := decodeContentItem(item); ok {
				items = append(items, parsed)
			}
		default:
			if parsed, ok := decodeContentItem(element); ok {
				items = append(items, parsed)
			}
		}
	}
	if len(items) == 0 {
		return nil
	}
	return items
}

// decodeContentItem 把任意对象解码为内容项（失败或缺少 type 时返回 false）
func decodeContentItem(value any) (ContentItem, bool) {
	raw, err := json.Marshal(value)
	if err != nil {
		return ContentItem{}, false
	}
	var item ContentItem
	if json.Unmarshal(raw, &item) != nil || item.Type == "" {
		return ContentItem{}, false
	}
	return item, true
}

// messageText 提取消息中的全部文本内容
func messageText(message PostMessage) string {
	if text, ok := message.Content.(string); ok {
		return text
	}
	items := asItems(message.Content)
	if len(items) == 0 {
		return ""
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		if item.Type == ContentText && item.Text != "" {
			parts = append(parts, item.Text)
		}
	}
	return strings.Join(parts, " ")
}

// jsonText 把任意值序列化为 JSON 文本（失败时返回空串）
func jsonText(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(raw)
}

// safeJSON 把任意值序列化为 JSON 文本，失败时返回给定兜底
func safeJSON(value any, fallback string) string {
	if text := jsonText(value); text != "" {
		return text
	}
	return fallback
}

// stringArg 读取字符串参数
func stringArg(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	if value, ok := args[key].(string); ok {
		return value
	}
	return ""
}

// numberArg 读取数值参数（兼容 float64 / json.Number / 整型）
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

// randomFloor 生成区间内的随机整数（含端点）
func randomFloor(min, max int) int {
	if max <= min {
		return min
	}
	return rand.Intn(max-min+1) + min
}

// randomFloat 生成区间内的随机浮点数并保留两位小数
func randomFloat(min, max float64) float64 {
	return float64(int((min+rand.Float64()*(max-min))*100)) / 100
}

// isHTTPURL 是否为可直接打开的 http(s) 地址
func isHTTPURL(raw string) bool {
	lower := strings.ToLower(strings.TrimSpace(raw))
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}
