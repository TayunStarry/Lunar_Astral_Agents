package viewer

// desire.go 选片关键词的切分与清洗。
//
// 待看列表补齐时，长期记忆里的主题词与配置偏好都走同一套切分规则，
// 保证「记忆 → 检索词」的转换口径一致。

import (
	"strings"
)

const (
	// desireKeywordMax 单轮最多使用几个关键词
	desireKeywordMax = 3
	// desireKeywordMinRunes / MaxRunes 合法关键词的长度范围
	desireKeywordMinRunes = 2
	desireKeywordMaxRunes = 12
)

// splitDesireWords 按顿号/逗号/空格/换行切分并过滤非法长度，去重后取前若干个
func splitDesireWords(text string) []string {
	// 模型有时会把关键词包在引号或列表符号里，先统一成基本分隔符
	replacer := strings.NewReplacer(
		"、", "\n", "，", "\n", ",", "\n", "；", "\n", ";", "\n",
		"/", "\n", "｜", "\n", "|", "\n", " ", "\n",
		"「", "", "」", "", "“", "", "”", "", "\"", "",
	)
	fields := strings.Split(replacer.Replace(text), "\n")
	seen := make(map[string]bool, len(fields))
	out := make([]string, 0, desireKeywordMax)
	for _, field := range fields {
		word := cleanDesireFragment(field)
		length := len([]rune(word))
		if length < desireKeywordMinRunes || length > desireKeywordMaxRunes || seen[word] {
			continue
		}
		seen[word] = true
		out = append(out, word)
		if len(out) >= desireKeywordMax {
			break
		}
	}
	if len(out) == 0 {
		// 整句都超过关键词长度时，去掉意图词后取前若干字，保证「有想法」不会完全落空
		trimmed := []rune(cleanDesireFragment(text))
		if len(trimmed) > desireKeywordMaxRunes {
			trimmed = trimmed[:desireKeywordMaxRunes]
		}
		if len(trimmed) >= desireKeywordMinRunes {
			out = append(out, string(trimmed))
		}
	}
	return out
}

// cleanDesireFragment 去掉自然语言里的意图词与冗余后缀，让兜底关键词更接近检索词
func cleanDesireFragment(raw string) string {
	word := strings.TrimSpace(raw)
	for _, prefix := range []string{
		"我想看", "我想看看", "想看", "想看一些", "我要看", "要看", "给我看",
		"找一些", "找点", "来点", "推荐一些", "推荐点", "推荐", "关于",
	} {
		word = strings.TrimPrefix(word, prefix)
	}
	for _, suffix := range []string{
		"的视频", "视频", "的内容", "内容", "相关", "吧", "。",
	} {
		word = strings.TrimSuffix(word, suffix)
	}
	word = strings.Trim(word, "-*#0123456789. 　")
	return strings.TrimSpace(word)
}
