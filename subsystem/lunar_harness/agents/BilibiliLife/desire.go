package BilibiliLife

// desire.go 对话者本轮关键词：把 viewer 工具传来的 instruction 提炼成选片偏好。
//
// 只在本次调用内生效：提取结果写进 Config.DesireKeywords（json:"-"），
// 由选片打分临时读取；不写回 bilibili_life.json，也不影响下一轮。

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
	words := extractDesireKeywords(r.instruction)
	if len(words) == 0 {
		return
	}
	r.cfg.DesireKeywords = words
	logger.Info("BilibiliLife", "本轮关键词（仅本次生效）: %s", formatKeywords(words))
}

// extractDesireKeywords 提炼本轮关键词：优先让模型归纳，失败时按标点切分兜底
func extractDesireKeywords(instruction string) []string {
	instruction = strings.TrimSpace(instruction)
	if instruction == "" {
		return nil
	}
	system := "你是 B 站选片助手。把用户这句话里的观看意愿提炼成 1-3 个适合站内搜索的中文关键词，" +
		"只输出关键词本身（用顿号分隔），不要解释、不要标点、不要句子。"
	if text, err := chatText(system, instruction); err == nil {
		if words := splitDesireWords(text); len(words) > 0 {
			return words
		}
	} else {
		logger.Warn("BilibiliLife", "关键词提炼失败，改用指令原文兜底: %v", err)
	}
	return splitDesireWords(instruction)
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
		word := strings.TrimSpace(field)
		word = strings.Trim(word, "-*#0123456789. ")
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
		// 整句都超过关键词长度时，取前若干字作为检索词，保证「有想法」不会完全落空
		trimmed := []rune(strings.TrimSpace(text))
		if len(trimmed) > desireKeywordMaxRunes {
			trimmed = trimmed[:desireKeywordMaxRunes]
		}
		if len(trimmed) >= desireKeywordMinRunes {
			out = append(out, string(trimmed))
		}
	}
	return out
}
