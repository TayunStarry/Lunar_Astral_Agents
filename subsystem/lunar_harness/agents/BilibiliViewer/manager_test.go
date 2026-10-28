package BilibiliViewer

import (
	"os"
	"path/filepath"
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
)

// TestConfigHotReload 钉住 bilibili_life.json 的改动会在下一次调用时重新加载。
func TestConfigHotReload(t *testing.T) {
	tmp := t.TempDir()
	orig := config.LocalDir
	config.LocalDir = tmp
	t.Cleanup(func() { config.LocalDir = orig })

	path := filepath.Join(tmp, "bilibili_life.json")
	write := func(body string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("写入配置失败: %v", err)
		}
	}

	m := &lifeManager{}
	write(`{"preference_keywords":["猫咪"],"replies_per_round":10}`)
	m.reloadConfigLocked()
	if len(m.cfg.PreferenceKeywords) != 1 || m.cfg.PreferenceKeywords[0] != "猫咪" || m.cfg.RepliesPerRound != 10 {
		t.Fatalf("首次加载不正确: %+v", m.cfg)
	}

	write(`{"preference_keywords":["猫咪","游戏"],"replies_per_round":20}`)
	m.reloadConfigLocked()
	if len(m.cfg.PreferenceKeywords) != 2 || m.cfg.RepliesPerRound != 20 {
		t.Fatalf("配置未热重载: %+v", m.cfg)
	}
}

// TestResultText 钉住工具回复：简短陈述看了几个视频、回了几条消息。
func TestResultText(t *testing.T) {
	empty := newRunner(Config{}, "")
	if got := empty.resultText(); got != "这次没找到合适的视频，已经先记下来了。" {
		t.Fatalf("没有通过审核的视频时应给出简短说明，实际 %q", got)
	}

	watched := newRunner(Config{}, "")
	watched.replied = 2
	watched.approved = []*videoRecord{
		{Info: videoInfo{Title: "A"}, Impression: videoImpression{Summary: "看起来在讲本地模型部署"}},
		{Info: videoInfo{Title: "B"}, Impression: videoImpression{Summary: "大概是游戏实况"}},
	}
	got := watched.resultText()
	if got == "" || !containsAll(got, "2 个视频", "回了 2 条消息", "本地模型部署") {
		t.Fatalf("结果应为简短陈述且包含数量与印象，实际 %q", got)
	}
}

func containsAll(text string, parts ...string) bool {
	for _, part := range parts {
		if !containsText(text, part) {
			return false
		}
	}
	return true
}

func containsText(text, part string) bool {
	for i := 0; i+len(part) <= len(text); i++ {
		if text[i:i+len(part)] == part {
			return true
		}
	}
	return false
}

// TestWatchQuota 钉住每日刷视频额度：按自然日计数，用完后拒绝。
func TestWatchQuota(t *testing.T) {
	tmp := t.TempDir()
	orig := config.LocalDir
	config.LocalDir = tmp
	t.Cleanup(func() { config.LocalDir = orig })

	cfg := defaultConfig()
	cfg.DailyLimit = 2
	if watchQuotaExhausted(cfg) {
		t.Fatal("初始不应判定为额度用尽")
	}
	consumeWatchQuota(cfg)
	consumeWatchQuota(cfg)
	if !watchQuotaExhausted(cfg) {
		t.Fatal("用满 2 次后应判定为额度用尽")
	}
}

// TestChargeQuotaOnce 钉住一轮流程只扣一次额度。
func TestChargeQuotaOnce(t *testing.T) {
	tmp := t.TempDir()
	orig := config.LocalDir
	config.LocalDir = tmp
	t.Cleanup(func() { config.LocalDir = orig })

	r := newRunner(defaultConfig(), "")
	r.chargeQuota()
	r.chargeQuota()
	if got := todayWatchCount(); got != 1 {
		t.Fatalf("一轮只应扣 1 次额度，实际 %d", got)
	}
}
