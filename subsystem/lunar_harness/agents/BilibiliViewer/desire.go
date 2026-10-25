package BilibiliViewer

// desire.go 对话者本轮关键词：把 viewer 工具传来的 instruction 提炼成选片关键词。
//
// 结合语义：提炼时同时看「用户这句话」与配置文件里的 preference_keywords，
// 综合给出本轮使用的关键词（用户明确主题优先，默认偏好作补充/兜底）。
// 结果写进 Config.DesireKeywords（json:"-"），本轮选片只按这些词打分与搜索；
// 只在本次调用内生效，不写回 bilibili_life.json。

import (
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

const (
	// desireKeywordMax 单轮最多使用几个关键词
	desireKeywordMax = 3
	// desireKeywordMinRunes / MaxRunes 合法关键词的长度范围
	desireKeywordMinRunes = 2
	desireKeywordMaxRunes = 12
)

// prepareDesireKeywords 提炼并写入本轮关键词（调用方须在 manager 锁之外调用）
func (r *runner) prepareDesireKeywords() {
	if r == nil || strings.TrimSpace(r.instruction) == "" {
		return
	}
	words := extractDesireKeywords(r.instruction, r.cfg.PreferenceKeywords)
	if len(words) == 0 {
		return
	}
	r.cfg.DesireKeywords = words
	logger.Info("BilibiliViewer", "本轮选片关键词（结合配置默认偏好，仅本次生效）: %s", formatKeywords(words))
}

// extractDesireKeywords 综合提炼本轮关键词：同时参考用户指令与配置默认偏好，
// 优先让模型归纳，失败时按标点切分并与默认偏好合并兜底。
// 返回空表示本次没有可用的欲求词，调用方会回退到配置默认偏好。
func extractDesireKeywords(instruction string, defaults []string) []string {
	instruction = strings.TrimSpace(instruction)
	if instruction == "" {
		return nil
	}
	// 选片也要参考历史记忆：最近看过的主题、想接着看什么，都由记忆决定本轮关键词
	system := "你是 B 站选片助手。结合用户这句话与配置里的默认偏好，给出本轮 1-3 个适合站内搜索的中文关键词。\n" +
		"规则：用户明确说了想看的主题时以用户为准，默认偏好只在相关时补充；" +
		"用户没有明确主题时，从默认偏好里挑合适的词。\n"
	if len(defaults) > 0 {
		system += "配置默认偏好：" + strings.Join(defaults, "、") + "。\n"
	}
	system += "只输出关键词本身（用顿号分隔），不要解释、不要标点、不要句子。" + memoryPromptBlock()
	if text, err := chatText(system, instruction); err == nil {
		if words := splitDesireWords(text); len(words) > 0 {
			return words
		}
	} else {
		logger.Warn("BilibiliViewer", "关键词提炼失败，改用指令原文兜底: %v", err)
	}
	return mergeDesireWords(splitDesireWords(instruction), defaults)
}

// mergeDesireWords 合并本轮词与配置默认偏好：本轮词优先，默认偏好补足剩余名额，去重。
func mergeDesireWords(words, defaults []string) []string {
	out := make([]string, 0, desireKeywordMax)
	seen := make(map[string]bool, len(words)+len(defaults))
	for _, list := range [][]string{words, defaults} {
		for _, raw := range list {
			word := strings.TrimSpace(raw)
			if word == "" || seen[word] {
				continue
			}
			seen[word] = true
			out = append(out, word)
			if len(out) >= desireKeywordMax {
				return out
			}
		}
	}
	return out
}

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
