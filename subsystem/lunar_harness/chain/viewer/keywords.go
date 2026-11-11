package viewer

// keywords.go 选片关键词的读取与合并。
//
// 静态词来自 local_data/config/bilibili.json，动态词来自 viewer.db 的观看主题词池
// （衰减与筛选取自 watch_topic.go）。合并只在这里做一次：打分统一走 titleScore，
// 不区分词是静态还是动态。

import (
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// preferenceKeywords 取配置文件里的长期偏好词（去重后最多 preferenceKeywordLimit 个）。
func preferenceKeywords() []string {
	return dedupeKeywords(LoadConfig().PreferenceKeywords, preferenceKeywordLimit)
}

// dynamicKeywords 取观影历史里衰减后的高频主题词。
//
// 读取失败或无历史主题词时返回空列表：选片自动退化为仅用静态偏好，不阻断本轮。
func dynamicKeywords(at int64) []string {
	words, err := activeWatchedTopics(watchDynamicKeywordLimit, at)
	if err != nil {
		logger.Warn("BilibiliViewer", "待看列表：读取动态主题词失败，退回静态偏好: %v", err)
		return nil
	}
	return words
}

// selectionConfig 把静态偏好与动态主题词合并成打分配置。
//
// 同一词只保留在动态档（Desire 权重 2），静态档不再重复计入，避免退化成 2+1 的加权。
// 两档都为空时打分恒为 0，选片退回「首页顺序 + 历史排除」的等价行为。
func selectionConfig(static, dynamic []string) Config {
	desire := dedupeKeywords(dynamic, watchDynamicKeywordLimit)
	taken := make(map[string]bool, len(desire))
	for _, word := range desire {
		taken[keywordKey(word)] = true
	}
	preference := make([]string, 0, len(static))
	for _, word := range dedupeKeywords(static, preferenceKeywordLimit) {
		if taken[keywordKey(word)] {
			continue
		}
		preference = append(preference, word)
	}
	return Config{PreferenceKeywords: preference, DesireKeywords: desire}
}

// searchKeywords 搜索词池：动态词在前、静态词在后，去重后截断。
func searchKeywords(selection Config) []string {
	merged := make([]string, 0, len(selection.DesireKeywords)+len(selection.PreferenceKeywords))
	merged = append(merged, selection.DesireKeywords...)
	merged = append(merged, selection.PreferenceKeywords...)
	return dedupeKeywords(merged, watchSearchKeywordLimit)
}

// dedupeKeywords 去空白、大小写不敏感去重，最多保留 limit 个。
func dedupeKeywords(words []string, limit int) []string {
	if limit <= 0 || limit > len(words) {
		limit = len(words)
	}
	out := make([]string, 0, limit)
	seen := make(map[string]bool, len(words))
	for _, raw := range words {
		word := strings.TrimSpace(raw)
		if word == "" {
			continue
		}
		key := keywordKey(word)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, word)
		if len(out) >= limit {
			break
		}
	}
	return out
}

// keywordKey 生成关键词的去重键（大小写不敏感）。
func keywordKey(word string) string {
	return strings.ToLower(strings.TrimSpace(word))
}
