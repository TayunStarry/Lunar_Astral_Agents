package life

import (
	"strings"
	"unicode"
)

func emptyAs(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func truncateRunes(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "…"
}

// truncateAtSentence 在句读处切断，避免把一句话截成半句。
//
// 规则：在 limit 之前寻找最后一个句末标点（。！？；!?;…）或分句标点（，、,：:）或换行；
// 优先句末标点，其次分句标点，最后退化为按空白切断；都没有时才硬切并补省略号。
func truncateAtSentence(text string, limit int) string {
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
