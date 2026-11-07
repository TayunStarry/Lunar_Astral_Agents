package viewer

// keywords_test.go 静态偏好与动态主题词的合并、去重与 2:1 权重。

import (
	"strings"
	"testing"
)

// TestSelectionConfigDynamicTierWins 同一词只留在动态档，Desire 权重 2、Preference 权重 1。
func TestSelectionConfigDynamicTierWins(t *testing.T) {
	cfg := selectionConfig([]string{"AI", "美食"}, []string{"AI", "猫咪"})
	if strings.Join(cfg.DesireKeywords, ",") != "AI,猫咪" {
		t.Fatalf("动态档不符: %v", cfg.DesireKeywords)
	}
	if strings.Join(cfg.PreferenceKeywords, ",") != "美食" {
		t.Fatalf("静态档应剔除已进入动态档的词: %v", cfg.PreferenceKeywords)
	}
	// 两个词都命中动态档：2 词 × 3 分 × 2 份。
	if score := titleScore("AI 猫咪", cfg); score != 12 {
		t.Fatalf("动态档 2:1 权重未生效，得分 %d", score)
	}
}

// TestSelectionConfigFallsBackToStatic 无动态词时只用静态词，排序行为与历史等价。
func TestSelectionConfigFallsBackToStatic(t *testing.T) {
	cfg := selectionConfig([]string{"AI", "美食", "AI"}, nil)
	if len(cfg.DesireKeywords) != 0 {
		t.Fatalf("无观影历史时动态档应为空，实际 %v", cfg.DesireKeywords)
	}
	if strings.Join(cfg.PreferenceKeywords, ",") != "AI,美食" {
		t.Fatalf("静态档应去重保序: %v", cfg.PreferenceKeywords)
	}
	if score := titleScore("AI 视频", cfg); score != 3 {
		t.Fatalf("静态单词应加 3 分，实际 %d", score)
	}
	if pool := searchKeywords(cfg); strings.Join(pool, ",") != "AI,美食" {
		t.Fatalf("无动态词时搜索池应退回静态词: %v", pool)
	}
}

// TestSelectionConfigCapsAndOrdersSearchPool 搜索池动态优先、去重并按上限截断。
func TestSelectionConfigCapsAndOrdersSearchPool(t *testing.T) {
	static := []string{"静态一", "静态二", "静态三", "静态四", "静态五",
		"静态六", "静态七", "静态八", "静态九", "静态十"}
	dynamic := make([]string, 0, watchDynamicKeywordLimit)
	for index := 0; index < watchDynamicKeywordLimit; index++ {
		dynamic = append(dynamic, "动态"+string(rune('A'+index)))
	}
	cfg := selectionConfig(append(static, "动态A"), dynamic)
	pool := searchKeywords(cfg)
	if len(pool) != watchSearchKeywordLimit {
		t.Fatalf("搜索池应截断到 %d 个，实际 %d: %v", watchSearchKeywordLimit, len(pool), pool)
	}
	if pool[0] != "动态A" {
		t.Fatalf("动态词应排在搜索池前面: %v", pool)
	}
	seen := make(map[string]bool, len(pool))
	for _, word := range pool {
		if seen[word] {
			t.Fatalf("搜索池出现重复词 %q: %v", word, pool)
		}
		seen[word] = true
	}
}
