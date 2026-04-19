package utils

import (
	"fmt"
	"strings"
)

// GetFloat64Value 获取float64值
func GetFloat64Value(data map[string]any, key string) float64 {
	if val, ok := data[key].(float64); ok {
		return val
	} else if val, ok := data[key].(int); ok {
		return float64(val)
	} else if val, ok := data[key].(string); ok {
		var floatVal float64
		fmt.Sscanf(val, "%f", &floatVal)
		return floatVal
	}
	return 0
}

// GetStringValue 获取string值
func GetStringValue(data map[string]any, key string) string {
	if val, ok := data[key].(string); ok {
		return val
	}
	return ""
}

// ProcessImageURL 处理图片URL并处理HTML实体
func ProcessImageURL(url string) string {
	if url == "" {
		return ""
	}
	// 处理HTML实体
	url = strings.ReplaceAll(url, "\u0026", "&")
	url = strings.ReplaceAll(url, "&amp;", "&")
	url = strings.ReplaceAll(url, "&lt;", "<")
	url = strings.ReplaceAll(url, "&gt;", ">")
	url = strings.ReplaceAll(url, "&quot;", "\"")
	url = strings.ReplaceAll(url, "&#39;", "'")
	return url
}
