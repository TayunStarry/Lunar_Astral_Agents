package textutil

// text.go 文本裁剪公共工具。
//
// 这里只放无副作用、无业务依赖的纯函数，是全仓「截断 / 首非空」的唯一实现。
// protocol/agentbase、chain、agents、engine 各包均由此引用，禁止再复制一份实现，
// 也禁止为几个截断函数反向依赖 protocol/agentbase。

import (
	"strings"
	"unicode"
)

// TruncateRunes 按字符截断（超长时以省略号收尾）。
func TruncateRunes(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "…"
}

// TruncateRunesTrimmed 去掉首尾空白后按字符截断（超长时以省略号收尾）。
func TruncateRunesTrimmed(text string, limit int) string {
	text = strings.TrimSpace(text)
	runes := []rune(text)
	if limit <= 0 || len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "…"
}

// HardCutRunesWithNotice 去掉首尾空白后按字符硬切断，超长时追加「已截断」标记。
// 名字里的 Notice 指末尾的「…（已截断）」；普通硬截断请用 CutRunes / ClipRunes。
func HardCutRunesWithNotice(text string, limit int) string {
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

// ClipRunes 按字符硬切断，不追加任何标记（保留原首尾空白），结果长度不超过 limit。
func ClipRunes(text string, limit int) string {
	runes := []rune(text)
	if limit <= 0 || len(runes) <= limit {
		return text
	}
	return string(runes[:limit])
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

// TruncateAtSentence 在句读处切断，避免把一句话截成半句。
//
// 规则：在 limit 之前寻找最后一个句末标点（。！？；!?;…）或分句标点（，、,：:）或换行；
// 优先句末标点，其次分句标点，最后退化为按空白切断；都没有时才硬切并补省略号。
func TruncateAtSentence(text string, limit int) string {
	return cutAtSentence(text, limit, 0)
}

// TruncateAtSentenceMinFill 与 TruncateAtSentence 相同，但句读位置过早（保留不足
// limit*3/5）时宁可硬切到 limit：例如长段落第一个逗号出现在第 60 字，按逗号收尾
// 会丢掉后面 88% 的内容。
func TruncateAtSentenceMinFill(text string, limit int) string {
	return cutAtSentence(text, limit, 60)
}

// cutAtSentence 句读截断的统一实现；minFillPercent 为 0 表示不限制句读位置。
func cutAtSentence(text string, limit int, minFillPercent int) string {
	trimmed := strings.TrimSpace(text)
	runes := []rune(trimmed)
	if limit <= 0 || len(runes) <= limit {
		return trimmed
	}
	cut := 0
	// 第一轮：找离 limit 最近的句末标点
	for i := limit - 1; i >= 0; i-- {
		if isSentenceEnd(runes[i]) {
			cut = i + 1
			break
		}
	}
	// 第二轮：没有句末标点时，退到最近的分句标点/换行
	if cut == 0 {
		for i := limit - 1; i >= 0; i-- {
			if isClauseBreak(runes[i]) {
				cut = i + 1
				break
			}
		}
	}
	// 第三轮：按空白切断
	if cut <= 0 {
		for i := limit - 1; i > 0; i-- {
			if unicode.IsSpace(runes[i]) {
				cut = i
				break
			}
		}
	}
	if cut <= 0 {
		cut = limit
	}
	if minFillPercent > 0 && cut < limit*minFillPercent/100 {
		cut = limit
	}
	out := strings.TrimSpace(string(runes[:cut]))
	if cut < len(runes) && !isSentenceEnd(runes[cut-1]) {
		out += "…"
	}
	return out
}

// isSentenceEnd 句末标点（全角与半角）
func isSentenceEnd(r rune) bool {
	switch r {
	case '。', '！', '？', '；', '!', '?', ';', '…':
		return true
	}
	return false
}

// isClauseBreak 分句标点与换行
func isClauseBreak(r rune) bool {
	switch r {
	case '，', '、', ',', '：', ':', '\n':
		return true
	}
	return false
}
