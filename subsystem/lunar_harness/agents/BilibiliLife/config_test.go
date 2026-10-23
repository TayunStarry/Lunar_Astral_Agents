package BilibiliLife

import (
	"os"
	"path/filepath"
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
)

func TestFilterAndScoreBlocksPoliticsAndConfrontation(t *testing.T) {
	cfg := defaultConfig()
	links := []videoLink{
		{Href: "https://www.bilibili.com/video/BV1", Title: "中美关系最新进展"},
		{Href: "https://www.bilibili.com/video/BV2", Title: "男女对立又上热搜了"},
		{Href: "https://www.bilibili.com/video/BV3", Title: "AI 绘画入门：大肥鱼模型实测"},
		{Href: "https://www.bilibili.com/video/BV4", Title: "直播回放：昨天"},
	}
	out, blockedCount := filterAndScore(links, cfg)
	if blockedCount != 3 {
		t.Fatalf("应过滤 3 条涉政/对立/录播，实际 %d：%#v", blockedCount, out)
	}
	if len(out) != 1 || out[0].Href != "https://www.bilibili.com/video/BV3" {
		t.Fatalf("应只留下 AI 视频，实际 %#v", out)
	}
	if out[0].Score <= 0 {
		t.Fatalf("AI 参考词应加分，实际 score=%d", out[0].Score)
	}
}

// TestMentionNamesAllTargets 钉住分享时 @ 全部目标（最多 5 人、去重、保持配置顺序）。
func TestMentionNamesAllTargets(t *testing.T) {
	cfg := Config{MentionTargets: []MentionTarget{{Name: "甲"}, {Name: "乙"}, {Name: "甲"}}}
	got := cfg.mentionNames()
	if len(got) != 2 || got[0] != "甲" || got[1] != "乙" {
		t.Fatalf("应按配置顺序去重后返回全部目标，实际 %v", got)
	}

	// 超过上限时按配置顺序截断
	over := Config{}
	for _, name := range []string{"1", "2", "3", "4", "5", "6", "7"} {
		over.MentionTargets = append(over.MentionTargets, MentionTarget{Name: name})
	}
	if got := over.mentionNames(); len(got) != mentionMaxCount || got[mentionMaxCount-1] != "5" {
		t.Fatalf("应截断为前 %d 人，实际 %v", mentionMaxCount, got)
	}

	// 未配置时返回空：调用方据此跳过 @ 分享，绝不内置默认 @ 对象
	if got := (Config{}).mentionNames(); len(got) != 0 {
		t.Fatalf("未配置 mention_targets 时不应产生任何 @ 目标，实际 %v", got)
	}
}

// TestOnlyCallUserInDynamicDefault 钉住新配置项默认开启：
// 未配置时不再挑视频发 @分享评论，只在动态末尾呼叫用户。
func TestOnlyCallUserInDynamicDefault(t *testing.T) {
	if !defaultConfig().onlyCallUserInDynamic() {
		t.Fatal("「仅在动态中呼叫用户」默认应为开启")
	}
	if !(Config{}).onlyCallUserInDynamic() {
		t.Fatal("零值配置也应按开启处理")
	}
	off := false
	if (Config{OnlyCallUserInDynamic: &off}).onlyCallUserInDynamic() {
		t.Fatal("显式关闭时应返回 false")
	}
}

// TestLoadConfigOnlyCallUserInDynamic 钉住配置项能正常读写切换（含热重载读取）。
func TestLoadConfigOnlyCallUserInDynamic(t *testing.T) {
	tmp := t.TempDir()
	orig := *config.LocalDir
	*config.LocalDir = tmp
	t.Cleanup(func() { *config.LocalDir = orig })

	path := filepath.Join(tmp, "bilibili_life.json")
	if cfg := LoadConfig(); !cfg.onlyCallUserInDynamic() {
		t.Fatal("配置文件缺失时应默认开启")
	}
	if err := os.WriteFile(path, []byte(`{"only_call_user_in_dynamic": false}`), 0o644); err != nil {
		t.Fatalf("写入配置失败: %v", err)
	}
	if cfg := LoadConfig(); cfg.onlyCallUserInDynamic() {
		t.Fatal("配置为 false 时应关闭")
	}
	if err := os.WriteFile(path, []byte(`{"only_call_user_in_dynamic": true}`), 0o644); err != nil {
		t.Fatalf("写入配置失败: %v", err)
	}
	if cfg := LoadConfig(); !cfg.onlyCallUserInDynamic() {
		t.Fatal("配置为 true 时应开启")
	}
}

func TestBlockedKeywordsAreNonEmpty(t *testing.T) {
	cfg := defaultConfig()
	for _, word := range []string{"政治", "男女对立", "引战"} {
		if _, ok := blockedBy("测试"+word+"内容", cfg.BlockedKeywords); !ok {
			t.Fatalf("内置安全过滤缺少 %q", word)
		}
	}
}

// TestDefaultLifeMechanism 钉住一轮生活的机制默认值：
// 3 个视频 / 每个 10 个切片 / 5 次回应额度；评论数与读评论条数是固定机制，不再出现在配置里。
func TestDefaultLifeMechanism(t *testing.T) {
	cfg := defaultConfig()
	if cfg.VideosPerRound != 3 {
		t.Fatalf("默认应刷 3 个视频，实际 %d", cfg.VideosPerRound)
	}
	if cfg.SlicesPerVideo != defaultSlicesPerVideo {
		t.Fatalf("默认切片数应为 %d，实际 %d", defaultSlicesPerVideo, cfg.SlicesPerVideo)
	}
	if cfg.RepliesPerRound != defaultRepliesPerRound {
		t.Fatalf("默认回应额度应为 %d，实际 %d", defaultRepliesPerRound, cfg.RepliesPerRound)
	}
	if commentsPerVideo != 1 || commentsReadLimit != 15 {
		t.Fatalf("评论/热评的固定机制被改动: %d/%d", commentsPerVideo, commentsReadLimit)
	}
}

// TestClampVideosPerRound 钉住视频数量的硬边界：min 1、max 5。
func TestClampVideosPerRound(t *testing.T) {
	cases := []struct{ in, want int }{
		{0, 1}, {1, 1}, {3, 3}, {5, 5}, {8, 5}, {-2, 1},
	}
	for _, c := range cases {
		if got := clampInt(c.in, videoMinCount, videoMaxCount); got != c.want {
			t.Fatalf("clampInt(%d) 应为 %d，实际 %d", c.in, c.want, got)
		}
	}
}

// TestNewRunnerClampsConfig 钉住 runner 侧的兜底夹紧（配置文件写越界也不会真的刷 10 个）。
func TestNewRunnerClampsConfig(t *testing.T) {
	r := newRunner(Config{VideosPerRound: 10, SlicesPerVideo: 0, RepliesPerRound: 0}, true)
	if r.cfg.VideosPerRound != videoMaxCount {
		t.Fatalf("越界视频数应夹到 %d，实际 %d", videoMaxCount, r.cfg.VideosPerRound)
	}
	if r.cfg.SlicesPerVideo != defaultSlicesPerVideo || r.cfg.RepliesPerRound != defaultRepliesPerRound {
		t.Fatalf("零值应回落到默认值，实际 %d/%d", r.cfg.SlicesPerVideo, r.cfg.RepliesPerRound)
	}
}

// TestPickFeaturedVideo 钉住「一轮只选一个主角视频」：取评论区收集最多的一条，平票取先看的。
func TestPickFeaturedVideo(t *testing.T) {
	first := &videoRecord{Info: videoInfo{Title: "先看的"}}
	second := &videoRecord{Info: videoInfo{Title: "讨论最多的"}}
	first.Comments = []commentItem{{Content: "a"}}
	second.Comments = []commentItem{{Content: "a"}, {Content: "b"}, {Content: "c"}}

	if got := pickFeaturedVideo([]*videoRecord{nil, first, second}); got != second {
		t.Fatalf("应选评论区最多的视频，实际 %v", got)
	}
	// 平票时保持先看的那条，保证断点续跑选择稳定
	tie := &videoRecord{Info: videoInfo{Title: "平票"}}
	if got := pickFeaturedVideo([]*videoRecord{first, tie}); got != first {
		t.Fatalf("平票应选先看的，实际 %v", got)
	}
	if pickFeaturedVideo(nil) != nil || pickFeaturedVideo([]*videoRecord{nil}) != nil {
		t.Fatal("没有视频时应返回 nil")
	}
}
