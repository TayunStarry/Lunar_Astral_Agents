package library

// state.go 状态推导与字段裁剪等纯函数。

import "strings"

// statusOf 推断文件名配色：运行失败红 > 运行成功绿 > 有操作记录黄 > 默认。
func statusOf(state *FileState) string {
	switch {
	case state.Failed:
		return StatusError
	case state.hasRun && state.runOK:
		return StatusOK
	case state.At > 0:
		return StatusActive
	default:
		return StatusNormal
	}
}

// boolInt 把布尔值转成 SQLite 的 0/1。
func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

// clipRunes 按字符截断片段（超出时在尾部标注省略）。
func clipRunes(text string, limit int) string {
	text = strings.TrimSpace(text)
	if limit <= 0 || len([]rune(text)) <= limit {
		return text
	}
	runes := []rune(text)
	return string(runes[:limit]) + "…"
}
