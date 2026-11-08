package MemoryTagger

// parse.go 从模型响应中解析标签数组。

import (
	"encoding/json"
	"fmt"
	"strings"
)

// parseTagsJSON 提取 JSON 字符串数组，兼容纯 JSON、markdown 代码块与前后夹杂文字。
func parseTagsJSON(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)

	// 去除 markdown 代码块标记
	if strings.HasPrefix(raw, "```") {
		if idx := strings.Index(raw, "\n"); idx != -1 {
			raw = raw[idx+1:]
		}
		raw = strings.TrimRight(raw, " \t\n\r")
		raw = strings.TrimSuffix(raw, "```")
		raw = strings.TrimSpace(raw)
	}

	start := strings.Index(raw, "[")
	end := strings.LastIndex(raw, "]")
	if start == -1 || end == -1 || start >= end {
		return nil, fmt.Errorf("未找到 JSON 数组")
	}

	var tags []string
	if err := json.Unmarshal([]byte(raw[start:end+1]), &tags); err != nil {
		return nil, fmt.Errorf("JSON 解析失败: %w", err)
	}

	result := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag != "" {
			result = append(result, tag)
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("解析到的标签数组为空")
	}
	return result, nil
}
