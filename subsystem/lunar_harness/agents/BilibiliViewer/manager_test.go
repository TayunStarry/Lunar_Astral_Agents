package BilibiliViewer

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
)

// TestConfigHotReload 钉住 bilibili_life.json 的改动会被自动调度重新加载：
// 否则新增的 @ 目标要等重启才生效，表现为「永远只 @ 同一个人」。
func TestConfigHotReload(t *testing.T) {
	tmp := t.TempDir()
	orig := config.LocalDir
	config.LocalDir = tmp
	t.Cleanup(func() { config.LocalDir = orig })

	path := filepath.Join(tmp, "bilibili_life.json")
	write := func(body string, mod time.Time) {
		t.Helper()
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("写入配置失败: %v", err)
		}
		// 显式设定修改时间，避免同一时刻写入导致 mtime 无法区分
		if err := os.Chtimes(path, mod, mod); err != nil {
			t.Fatalf("设置修改时间失败: %v", err)
		}
	}

	m := &lifeManager{}
	write(`{"mention_targets":[{"name":"甲"}]}`, time.Now().Add(-2*time.Hour))
	m.reloadConfigLocked()
	if len(m.cfg.MentionTargets) != 1 || m.cfg.MentionTargets[0].Name != "甲" {
		t.Fatalf("首次加载不正确: %+v", m.cfg.MentionTargets)
	}

	// 每次调用都会重读配置，新的 @ 目标应立刻进入候选池
	write(`{"mention_targets":[{"name":"甲"},{"name":"乙"}]}`, time.Now().Add(-time.Hour))
	m.reloadConfigLocked()
	names := m.cfg.mentionNames()
	if len(names) != 2 || names[0] != "甲" || names[1] != "乙" {
		t.Fatalf("配置未热重载，实际 %v", names)
	}
}

// TestParseFlow 钉住工具枚举到流程的映射。
func TestParseFlow(t *testing.T) {
	cases := map[string]Flow{
		"":              FlowStandard,
		"standard":      FlowStandard,
		"watch_comment": FlowWatchComment,
		"reply_only":    FlowReplyOnly,
	}
	for raw, want := range cases {
		got, ok := ParseFlow(raw)
		if !ok || got != want {
			t.Fatalf("ParseFlow(%q) = %q, %v; want %q", raw, got, ok, want)
		}
	}
	if _, ok := ParseFlow("share"); ok {
		t.Fatal("share 已并入 share 参数，不应再作为 flow")
	}
	if _, ok := ParseFlow("unknown"); ok {
		t.Fatal("未知流程应解析失败")
	}
}

// TestResultText 钉住工具回复：刷过视频给日记句柄，只回复评论时给条数。
func TestResultText(t *testing.T) {
	onlyReply := newRunner(Config{}, FlowReplyOnly, false, "")
	onlyReply.state.Replied = 3
	if got := onlyReply.resultText(); got != "[系统提示]已回复 3 条 B 站评论" {
		t.Fatalf("仅回复评论应返回条数，实际 %q", got)
	}

	noReply := newRunner(Config{}, FlowReplyOnly, false, "")
	if got := noReply.resultText(); got != "[系统提示]已回复 0 条 B 站评论" {
		t.Fatalf("没有未读时应返回 0 条，实际 %q", got)
	}

	// 防御：即使句柄字段残留，只要没刷视频就不应该展示日记句柄
	staleHandle := newRunner(Config{}, FlowReplyOnly, false, "")
	staleHandle.state.Replied = 2
	staleHandle.state.DiaryHandle = "@D.stale"
	staleHandle.state.DiaryStart = 3
	staleHandle.state.DiaryEnd = 8
	if got := staleHandle.resultText(); got != "[系统提示]已回复 2 条 B 站评论" {
		t.Fatalf("没刷视频时不应展示日记句柄，实际 %q", got)
	}

	watched := newRunner(Config{}, FlowWatchComment, false, "")
	watched.state.VideoIndex = 2
	watched.state.DiaryHandle = "@D.diary"
	watched.state.DiaryStart = 3
	watched.state.DiaryEnd = 20
	want := "[系统提示]你已经看完视频了, 可查询 @D.diary|L3-20 后向用户汇报内容"
	if got := watched.resultText(); got != want {
		t.Fatalf("刷过视频应返回日记句柄，实际 %q", got)
	}
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
