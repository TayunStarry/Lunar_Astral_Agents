package lunar

import (
	"regexp"
	"strings"
)

// ==== emoji / 颜文字 ====

const emojiClass = `\x{1F600}-\x{1F64F}\x{1F300}-\x{1F5FF}\x{1F680}-\x{1F6FF}` +
	`\x{1F900}-\x{1F9FF}\x{1FA70}-\x{1FAFF}\x{2600}-\x{26FF}\x{2700}-\x{27BF}` +
	`\x{FE00}-\x{FE0F}\x{1F1E0}-\x{1F1FF}\x{200D}\x{20E3}`

// kaomojiMarks 颜文字特征字符
const kaomojiMarks = `_^･・。.、；;～~ノﾉゞヾω￣▽△□○●☆★°´｀♪♫＞＜><｡一≧≦∇∀ﾟ⌒⌣◕`

var (
	emojiRegex   = regexp.MustCompile(`[` + emojiClass + `]+`)
	kaomojiRegex = regexp.MustCompile(
		`[<＜＼]?[（(](?:[^\s（()）\x{4e00}-\x{9fff}]|\x{4e00}){0,5}[` + kaomojiMarks + `]` +
			`(?:[^\s（()）\x{4e00}-\x{9fff}]|\x{4e00}){0,5}[）)]` +
			`(?:[<＜>＞／\x{30ce}\x{309e}\x{266a}\x{266b}]*)?` +
			`|[>＜^TtOo0vV][_\-=^><。.・oO][<＞^TtOo0vV]`)
	emotionRegex = regexp.MustCompile(`(?:` + emojiRegex.String() + `)|(?:` + kaomojiRegex.String() + `)`)
)

// ==== 思考区 / 代码块 ====

var (
	thinkingRegex  = regexp.MustCompile(`(?is)<think>(.*?)</think>`)
	codeBlockRegex = regexp.MustCompile("(?s)```[a-zA-Z0-9+#-]*.*?```")
)

// extractThinkingBlocks 提取思考区内容并返回剩余文本
func extractThinkingBlocks(text string) ([]string, string) {
	blocks := make([]string, 0)
	for _, match := range thinkingRegex.FindAllStringSubmatch(text, -1) {
		content := strings.TrimSpace(match[1])
		if content != "" {
			blocks = append(blocks, content)
		}
	}
	return blocks, thinkingRegex.ReplaceAllString(text, "")
}

// extractCodeBlocks 提取代码块（保留 ``` 标记）
func extractCodeBlocks(text string) ([]string, string) {
	blocks := codeBlockRegex.FindAllString(text, -1)
	return blocks, codeBlockRegex.ReplaceAllString(text, "")
}

// extractActionBlocks 提取最外层括号包裹的动作区
func extractActionBlocks(text string) ([]string, string) {
	runes := []rune(text)
	blocks := make([]string, 0)
	stack := make([]int, 0)
	ranges := make([][2]int, 0)
	for i, ch := range runes {
		switch ch {
		case '(', '（':
			stack = append(stack, i)
		case ')', '）':
			if len(stack) == 0 {
				continue
			}
			start := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if len(stack) != 0 {
				continue
			}
			content := strings.TrimSpace(string(runes[start+1 : i]))
			if content != "" {
				blocks = append(blocks, content)
			}
			ranges = append(ranges, [2]int{start, i + 1})
		}
	}
	return blocks, removeRuneRanges(runes, ranges)
}

// extractEmotionBlocks 提取连续 emoji / 颜文字并返回剩余文本
func extractEmotionBlocks(text string) ([]string, string) {
	blocks := make([]string, 0)
	current := ""
	lastEnd := -1
	matchRanges := make([][2]int, 0)
	for _, loc := range emotionRegex.FindAllStringIndex(text, -1) {
		matched := text[loc[0]:loc[1]]
		if current != "" && loc[0] == lastEnd {
			current += matched
		} else {
			if current != "" {
				blocks = append(blocks, current)
			}
			current = matched
		}
		lastEnd = loc[1]
		matchRanges = append(matchRanges, [2]int{loc[0], loc[1]})
	}
	if current != "" {
		blocks = append(blocks, current)
	}
	return blocks, removeByteRanges(text, matchRanges)
}

// removeRuneRanges 移除按 rune 下标标记的区间
func removeRuneRanges(runes []rune, ranges [][2]int) string {
	if len(ranges) == 0 {
		return string(runes)
	}
	var builder strings.Builder
	last := 0
	for _, item := range ranges {
		if item[0] > last {
			builder.WriteString(string(runes[last:item[0]]))
		}
		last = item[1]
	}
	if last < len(runes) {
		builder.WriteString(string(runes[last:]))
	}
	return builder.String()
}

// removeByteRanges 移除按字节下标标记的区间
func removeByteRanges(text string, ranges [][2]int) string {
	if len(ranges) == 0 {
		return text
	}
	var builder strings.Builder
	last := 0
	for _, item := range ranges {
		if item[0] > last {
			builder.WriteString(text[last:item[0]])
		}
		last = item[1]
	}
	if last < len(text) {
		builder.WriteString(text[last:])
	}
	return builder.String()
}

// ==== 文本清洗 ====

var (
	inlineCodeRegex = regexp.MustCompile("`[^`]*`")
	imageMarkRegex  = regexp.MustCompile(`!\[.*?\]\(.*?\)`)
	linkMarkRegex   = regexp.MustCompile(`\[([^\]]*)\]\(.*?\)`)
	htmlTagRegex    = regexp.MustCompile(`<[^>]*>`)
	newlineRegex    = regexp.MustCompile(`\r?\n`)
	cnBracketRegex  = regexp.MustCompile(`（[^）]*）`)
	enBracketRegex  = regexp.MustCompile(`\([^)]*\)`)
	commaRunRegex   = regexp.MustCompile(`,{2,}`)
	spaceRunRegex   = regexp.MustCompile(`\s+`)
	whiteListRegex  = regexp.MustCompile(`[^\x{4e00}-\x{9fff}a-zA-Z0-9\s_~\-，。？！：；、—…《》〈〉“”‘’（）【】,.'"?:!;()\[\]]`)
	leadingPunctRe  = regexp.MustCompile(`^[。，、：；:;,?!？！—～"'“”‘’()（）\[\]【】{}<>…\s]+`)
	trailingCommaRe = regexp.MustCompile(`[，,]+$`)
)

// cleanTextForTTS 清洗文本，去除不适合语音合成的内容
func cleanTextForTTS(text string) string {
	if text == "" {
		return ""
	}
	processed := inlineCodeRegex.ReplaceAllString(text, "")
	processed = imageMarkRegex.ReplaceAllString(processed, "")
	processed = linkMarkRegex.ReplaceAllString(processed, "$1")
	processed = htmlTagRegex.ReplaceAllString(processed, "")
	processed = emojiRegex.ReplaceAllString(processed, "")
	processed = strings.ReplaceAll(processed, "*", "")
	processed = newlineRegex.ReplaceAllString(processed, " ")
	processed = cnBracketRegex.ReplaceAllString(processed, "")
	processed = enBracketRegex.ReplaceAllString(processed, "")
	processed = whiteListRegex.ReplaceAllString(processed, ",")
	processed = commaRunRegex.ReplaceAllString(processed, ",")
	processed = spaceRunRegex.ReplaceAllString(processed, " ")
	return strings.TrimSpace(processed)
}

// removeEmojiSymbols 移除文本中的 emoji
func removeEmojiSymbols(text string) string {
	return emojiRegex.ReplaceAllString(text, "")
}

// ==== 断句 ====

func isLevel1Punct(ch rune) bool {
	return strings.ContainsRune("。？！—～?!", ch)
}

func isLevel2Punct(ch rune) bool {
	return strings.ContainsRune("，,、：；:;", ch)
}

func formatChunk(chunk string) string {
	chunk = leadingPunctRe.ReplaceAllString(chunk, "")
	chunk = trailingCommaRe.ReplaceAllString(chunk, "")
	return strings.TrimSpace(chunk)
}

// splitByPunct 按标点集拆分为片段（标点保留在片段尾部）
func splitByPunct(source string, punct func(rune) bool) []string {
	runes := []rune(source)
	result := make([]string, 0)
	start := 0
	for i := 0; i < len(runes); i++ {
		if !punct(runes[i]) {
			continue
		}
		end := i + 1
		for end < len(runes) && punct(runes[end]) {
			end++
		}
		fragment := strings.TrimSpace(string(runes[start:end]))
		if fragment != "" {
			result = append(result, fragment)
		}
		start = end
		i = end - 1
	}
	if start < len(runes) {
		fragment := strings.TrimSpace(string(runes[start:]))
		if fragment != "" {
			result = append(result, fragment)
		}
	}
	return result
}

func isInsideBracket(runes []rune, pos int) bool {
	depth := 0
	for i := 0; i < pos && i < len(runes); i++ {
		switch runes[i] {
		case '（', '(':
			depth++
		case '）', ')':
			depth--
		}
	}
	return depth > 0
}

// splitSentences 智能切分文本为句子
func splitSentences(text string) []string {
	if text == "" {
		return nil
	}
	const ideal = 35
	level1 := splitByPunct(text, isLevel1Punct)
	result := make([]string, 0, len(level1))
	for _, content := range level1 {
		runes := []rune(content)
		if len(runes) <= ideal {
			if formatted := formatChunk(content); formatted != "" {
				result = append(result, formatted)
			}
			continue
		}
		for len(runes) > ideal {
			splitPos := -1
			for i := minInt(len(runes)-1, ideal-1); i >= 0; i-- {
				if !isLevel2Punct(runes[i]) || isInsideBracket(runes, i) {
					continue
				}
				end := i + 1
				for end < len(runes) && isLevel2Punct(runes[end]) {
					end++
				}
				splitPos = end
				break
			}
			if splitPos <= 0 {
				break
			}
			if slice := formatChunk(string(runes[:splitPos])); slice != "" {
				result = append(result, slice)
			}
			runes = runes[splitPos:]
		}
		if tail := formatChunk(string(runes)); tail != "" {
			result = append(result, tail)
		}
	}
	return result
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// parseContent 解析原始文本：思考区、代码块、动作区与正文切片
func parseContent(rawText string) ParsedContent {
	if rawText == "" {
		return ParsedContent{}
	}
	thinkingBlocks, afterThinking := extractThinkingBlocks(rawText)
	codeBlocks, afterCode := extractCodeBlocks(afterThinking)
	emotionBlocks, afterEmotion := extractEmotionBlocks(afterCode)
	actionBlocks, afterAction := extractActionBlocks(afterEmotion)
	actionBlocks = append(actionBlocks, emotionBlocks...)
	displayText := removeEmojiSymbols(afterAction)
	chunks := splitSentences(displayText)
	textChunks := make([]TextChunk, 0, len(chunks))
	for _, chunk := range chunks {
		textChunks = append(textChunks, TextChunk{Display: chunk, TTS: cleanTextForTTS(chunk)})
	}
	return ParsedContent{
		ThinkingBlocks: thinkingBlocks,
		CodeBlocks:     codeBlocks,
		ActionBlocks:   actionBlocks,
		TextChunks:     textChunks,
		DisplayText:    displayText,
	}
}
