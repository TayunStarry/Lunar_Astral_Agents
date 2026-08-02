package learner

import (
	"fmt"
	"strings"
	"time"
)

// ============================================================
// 工具函数
// ============================================================

// getRuntimeContext 获取运行时上下文（时间 + 位置）
func (d *DebateSystem) getRuntimeContext() string {
	now := time.Now()
	timeStr := now.Format("2006-01-02 15:04:05")
	weekDay := [...]string{"星期日", "星期一", "星期二", "星期三", "星期四", "星期五", "星期六"}[now.Weekday()]

	// 位置信息暂时无法在 Go 层直接获取，由 TS 层注入或使用默认值
	return fmt.Sprintf("当前时间: %s %s", timeStr, weekDay)
}

// extractJSON 从文本中提取 JSON 内容
// 处理 markdown 代码块包裹等情况
func extractJSON(text string) string {
	text = strings.TrimSpace(text)

	// 去除 markdown 代码块包裹
	if strings.HasPrefix(text, "```json") {
		text = strings.TrimPrefix(text, "```json")
		text = strings.TrimSuffix(text, "```")
		text = strings.TrimSpace(text)
	} else if strings.HasPrefix(text, "```") {
		text = strings.TrimPrefix(text, "```")
		text = strings.TrimSuffix(text, "```")
		text = strings.TrimSpace(text)
	}

	// 尝试找到 JSON 数组或对象
	startIdx := -1
	endIdx := -1

	for i, ch := range text {
		if ch == '[' || ch == '{' {
			if startIdx == -1 {
				startIdx = i
			}
		}
		if ch == ']' || ch == '}' {
			endIdx = i
		}
	}

	if startIdx >= 0 && endIdx > startIdx {
		return text[startIdx : endIdx+1]
	}

	return text
}
