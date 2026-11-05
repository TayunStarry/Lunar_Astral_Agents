package textutil

// text.go 文本裁剪公共工具。
//
// 这里只放无副作用、无业务依赖的纯函数，是全仓「截断 / 首非空」的唯一实现。
// protocol/agentbase、chain、agents、engine 各包均由此引用，禁止再复制一份实现，
// 也禁止为几个截断函数反向依赖 protocol/agentbase。

import "strings"

// TruncateRunes 按字符截断（超长时以省略号收尾）。
func TruncateRunes(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "…"
}

// HardCutRunes 去掉首尾空白后按字符硬切断，超长时追加「已截断」标记。
func HardCutRunes(text string, limit int) string {
	r := []rune(strings.TrimSpace(text))
	if limit <= 0 || len(r) <= limit {
		return string(r)
	}
	return string(r[:limit-1]) + "…（已截断）"
}

// CutRunes 按字符硬切断，超长时以省略号收尾（保留原首尾空白，不追加说明）。
// 与 TruncateRunes 的差别是总长度受 limit 约束（省略号占一个字符位）。
func CutRunes(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	if limit < 1 {
		limit = 1
	}
	return string(runes[:limit-1]) + "…"
}

// FirstNonEmpty 返回第一个非空白字符串。
func FirstNonEmpty(values ...string) string {
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}
