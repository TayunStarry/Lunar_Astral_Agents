package life

import (
	"os"
	"path/filepath"
	"strings"
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

func TestArchiveMemoryBatchWritesJSONL(t *testing.T) {
	oldDir := config.LocalDir
	config.LocalDir = t.TempDir()
	t.Cleanup(func() { config.LocalDir = oldDir })

	batch := memoryBatch{Items: []memoryItem{
		{ID: "old-1", Content: "旧记忆一", Role: "user", Timestamp: 100},
		{ID: "old-2", Content: "旧记忆二", Role: "assistant", Timestamp: 200},
	}}
	if err := archiveMemoryBatch("mem-001", batch); err != nil {
		t.Fatalf("归档失败: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(config.LocalDir, filepath.FromSlash(memoryArchiveFile)))
	if err != nil {
		t.Fatalf("读取归档失败: %v", err)
	}
	text := string(raw)
	for _, want := range []string{"old-1", "旧记忆一", "old-2", "旧记忆二"} {
		if !strings.Contains(text, want) {
			t.Fatalf("归档缺少 %q: %s", want, text)
		}
	}
}

func TestMemoryConsolidationHasSingleTriggerEntry(t *testing.T) {
	oldDir := config.LocalDir
	oldAuto := config.MemoryAutoConsolidate
	oldOnce := config.MemoryConsolidateOnce
	config.LocalDir = t.TempDir()
	config.MemoryAutoConsolidate = false
	config.MemoryConsolidateOnce = false
	t.Cleanup(func() {
		config.LocalDir = oldDir
		config.MemoryAutoConsolidate = oldAuto
		config.MemoryConsolidateOnce = oldOnce
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
	if memoryEvents != 1 {
		t.Fatalf("整理入口应唯一，实际注册了 %d 个 memory 事件", memoryEvents)
	}
	if event == nil || event.Weight() != 0 {
		t.Fatal("自动整理默认关闭时 memory 事件权重应为 0")
	}
	// 手动触发与未来重新开启共用同一个事件入口
	if !RequestMemoryConsolidation("") {
		t.Fatal("手动整理应登记到 memory 事件")
	}
	mu.Lock()
	pending := state.PendingEvent
	mu.Unlock()
	if pending != "memory" {
		t.Fatalf("手动整理应走 memory 入口，实际 PendingEvent=%q", pending)
	}
	if !RequestEvent("memory", "整理一下") {
		t.Fatal("显式请求 memory 事件应成功")
	}
	mu.Lock()
	pending = state.PendingEvent
	mu.Unlock()
	if pending != "memory" {
		t.Fatalf("显式请求也应落到同一个 memory 入口，实际 PendingEvent=%q", pending)
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
