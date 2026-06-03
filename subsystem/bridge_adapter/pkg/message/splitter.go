package message

import (
	"regexp"
	"strings"
)

// sentenceEndPunctuation 匹配中英文句末标点符号：。！？…～~.!?
// 连续多个标点视为一个分隔符
var sentenceEndPunctuation = regexp.MustCompile(`[。！？…～~.!?]+`)

// SplitMessageByPunctuation 按句末标点符号拆分消息内容。
// 标点符号保留在每条拆分结果的末尾，拆分后每段去除首尾空白。
// 如果内容中没有匹配的标点符号，返回包含原始内容的单元素切片。
func SplitMessageByPunctuation(content string) []string {
	if content == "" {
		return nil
	}

	var parts []string
	lastEnd := 0

	locs := sentenceEndPunctuation.FindAllStringIndex(content, -1)
	for _, loc := range locs {
		end := loc[1]
		part := strings.TrimSpace(content[lastEnd:end])
		if part != "" {
			parts = append(parts, part)
		}
		lastEnd = end
	}

	// 处理最后一段（无标点结尾的剩余文本）
	if lastEnd < len(content) {
		part := strings.TrimSpace(content[lastEnd:])
		if part != "" {
			parts = append(parts, part)
		}
	}

	if len(parts) == 0 {
		return []string{content}
	}

	return parts
}