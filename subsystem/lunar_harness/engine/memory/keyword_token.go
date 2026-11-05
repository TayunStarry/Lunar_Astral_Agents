package memory

// keyword_token.go 关键词分词

import (
	"hash/fnv"
	"strings"
	"unicode"
	"unicode/utf8"
)

func keywordTokenize(text string) map[string]uint16 {
	terms := make(map[string]uint16)
	var cjkRun []rune
	var asciiRun []rune

	flushCJK := func() {
		if len(cjkRun) == 1 {
			terms[string(cjkRun)]++
		}
		for i := 0; i+1 < len(cjkRun); i++ {
			terms[string(cjkRun[i:i+2])]++
		}
		cjkRun = cjkRun[:0]
	}
	flushASCII := func() {
		appendKeywordASCIITerms(terms, asciiRun)
		asciiRun = asciiRun[:0]
	}

	for _, r := range text {
		switch {
		case unicode.IsSpace(r):
			flushCJK()
			flushASCII()
		case isKeywordCJKRune(r):
			flushASCII()
			cjkRun = append(cjkRun, r)
		case r < utf8.RuneSelf:
			// ASCII 连续串（字母/数字/符号）不因符号断开：保证 /usr/bin/go、ERR_CONN_RESET 整串命中
			flushCJK()
			asciiRun = append(asciiRun, r)
		default:
			// 中文标点、emoji 等作为分隔符
			flushCJK()
			flushASCII()
		}
	}
	flushCJK()
	flushASCII()
	return terms
}

// appendKeywordASCIITerms 处理一段 ASCII 连续串：
//   - 含符号的整串（路径/错误码）作为精确词元；
//   - 另外拆分出长度 ≥ KeywordMinASCIIWordLen 的字母数字子词，兼顾普通英文词召回。
func appendKeywordASCIITerms(terms map[string]uint16, run []rune) {
	if len(run) == 0 {
		return
	}
	raw := strings.ToLower(string(run))
	raw = strings.Trim(raw, "\"'`()[]{}<>,;!?")
	if raw == "" {
		return
	}

	hasSymbol := false
	var word []rune
	flushWord := func() {
		if len(word) >= KeywordMinASCIIWordLen {
			terms[string(word)]++
		}
		word = word[:0]
	}
	for _, r := range raw {
		if isKeywordWordRune(r) {
			word = append(word, r)
			continue
		}
		hasSymbol = true
		flushWord()
	}
	flushWord()

	if hasSymbol && utf8.RuneCountInString(raw) >= 2 {
		terms[raw]++
	}
}

func isKeywordCJKRune(r rune) bool {
	return unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) ||
		unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Hangul, r)
}

func isKeywordWordRune(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

// keywordTokenExact 判断词元是否属于“精确串”（路径/错误码/编号/版本号）：
// 含符号，或字母数字混合，或足够长的纯数字。
func keywordTokenExact(token string) bool {
	hasDigit := false
	hasAlpha := false
	hasSymbol := false
	for _, r := range token {
		switch {
		case r >= '0' && r <= '9':
			hasDigit = true
		case r >= 'a' && r <= 'z':
			hasAlpha = true
		case r >= utf8.RuneSelf:
			// 中文等多字节文字属于普通词，不按精确串加权
		default:
			hasSymbol = true
		}
	}
	if hasSymbol {
		return true
	}
	if hasDigit && hasAlpha {
		return true
	}
	return hasDigit && utf8.RuneCountInString(token) >= 3
}

// ===== 索引维护 =====

func keywordContentHash(content string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(content))
	return h.Sum32()
}
