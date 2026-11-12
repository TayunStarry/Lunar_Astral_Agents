package life

import (
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/memory"
)

func TestSameTextSetDetectsNoMergeContent(t *testing.T) {
	oldTexts := []string{"今天天气不错", "  今天天气很好  "}
	if !sameTextSet(oldTexts, []string{"今天天气很好", "今天天气不错"}) {
		t.Fatal("仅顺序/空白差异应视为没有可合并内容")
	}
	if sameTextSet(oldTexts, []string{"今天天气整体不错"}) {
		t.Fatal("内容发生合并后不应判定为原样")
	}
}

func TestViewerSourceOnlyBySourceField(t *testing.T) {
	if isViewerSource("") {
		t.Fatal("无 Source 的旧数据不能仅凭正文前缀当成观影记忆")
	}
	if !isViewerSource(" bilibili_viewer ") {
		t.Fatal("Source 为 bilibili_viewer 时应排除")
	}
}

// TestViewerPrefixCleanupRemoved 观影正文清洗已移除：只按 Source 排除观影条目，
// 正文前缀原样保留，生活链不再改写观影链的历史记忆。
func TestViewerPrefixCleanupRemoved(t *testing.T) {
	entries := []memory.Entry{
		{ID: "viewer-1", Content: "[B站视频] 某视频", Source: viewerMemorySource},
		{ID: "life-1", Content: "[B站视频] 其实是生活记录"},
	}
	items := filterLifeMemories(entries)
	if len(items) != 1 || items[0].ID != "life-1" {
		t.Fatalf("应只排除 Source=%s 的条目，实得 %+v", viewerMemorySource, items)
	}
	if items[0].Content != "[B站视频] 其实是生活记录" {
		t.Fatalf("无 Source 的正文前缀不应被清洗: %q", items[0].Content)
	}
}

func TestMemoryIsFixedStepNotWeightEvent(t *testing.T) {
	oldDir := config.LocalDir
	config.LocalDir = t.TempDir()
	t.Cleanup(func() {
		config.LocalDir = oldDir
		mu.Lock()
		registry = nil
		state = State{Version: 1, Used: map[string]int{}}
		mu.Unlock()
	})
	Init()

	mu.Lock()
	memoryEvents := 0
	for _, event := range registry {
		if event.ID() == "memory" {
			memoryEvents++
		}
	}
	event := eventByIDLocked("memory")
	mu.Unlock()
	if memoryEvents != 0 || event != nil {
		t.Fatalf("记忆整理不应注册为事件，实际注册 %d 个", memoryEvents)
	}
	if _, ok := defaultEvents["memory"]; ok {
		t.Fatal("记忆整理不应出现在权重事件表中")
	}
	if RequestEvent("memory", "整理一下", nil) {
		t.Fatal("记忆整理不能再通过事件队列触发")
	}

	oldRand := pickRandHook
	t.Cleanup(func() { pickRandHook = oldRand })
	for r := 0; r < 100; r++ {
		pickRandHook = func(int) int { return r }
		if picked := pickLocked(); picked != nil && picked.ID() == "memory" {
			t.Fatalf("事件挑选不应命中 memory（r=%d）", r)
		}
	}
}

func TestPickFallsBackToDaze(t *testing.T) {
	oldDir := config.LocalDir
	config.LocalDir = t.TempDir()
	t.Cleanup(func() {
		config.LocalDir = oldDir
		mu.Lock()
		registry = nil
		state = State{Version: 1, Used: map[string]int{}}
		mu.Unlock()
	})
	Init()

	mu.Lock()
	for _, event := range registry {
		if event.ID() != "daze" {
			state.Used[event.ID()] = event.DailyQuota()
		}
	}
	picked := pickLocked()
	mu.Unlock()
	if picked == nil || picked.ID() != "daze" {
		t.Fatalf("额度耗尽后应发呆，实际 %v", picked)
	}
}

func TestDefaultDazeWeight(t *testing.T) {
	if got := defaultEvents["daze"].Weight; got != 40 {
		t.Fatalf("发呆权重应为 40（权重总和 100 的兜底占比），实际 %d", got)
	}
}
