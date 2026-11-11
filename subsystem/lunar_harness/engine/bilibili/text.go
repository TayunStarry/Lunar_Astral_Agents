package bilibili

// text.go 本包内的纯文本工具。

// truncateRunes 按字符数截断，不破坏多字节字符。
func truncateRunes(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit])
}
