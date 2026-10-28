package life

import (
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
)

func TestBuildMemoryBatchesKeepsEntriesWhole(t *testing.T) {
	items := []memoryItem{
		{ID: "a", Content: strings.Repeat("甲", 6000)},
		{ID: "b", Content: strings.Repeat("乙", 6000)},
		{ID: "c", Content: strings.Repeat("丙", 12000)},
	}
	batches := buildMemoryBatches(items)
	if len(batches) != 3 {
		t.Fatalf("应分成 3 批，实际 %d", len(batches))
	}
	for _, batch := range batches {
		for _, item := range batch.Items {
			if item.Content == "" || len([]rune(item.Content)) != map[string]int{"a": 6000, "b": 6000, "c": 12000}[item.ID] {
				t.Fatalf("条目 %s 被截断", item.ID)
			}
		}
	}
}

func TestBuildMemoryBatchesGreedy(t *testing.T) {
	items := []memoryItem{
		{ID: "a", Content: strings.Repeat("甲", 4000)},
		{ID: "b", Content: strings.Repeat("乙", 4000)},
		{ID: "c", Content: strings.Repeat("丙", 4000)},
	}
	batches := buildMemoryBatches(items)
	if len(batches) != 2 {
		t.Fatalf("应为 2 批，实际 %d", len(batches))
	}
	if len(batches[0].Items) != 2 || len(batches[1].Items) != 1 {
		t.Fatalf("分批结果不符合预期: %d / %d", len(batches[0].Items), len(batches[1].Items))
	}
}

func TestParseMemoryArray(t *testing.T) {
	text := "```json\n[\"第一条\", \"第二条\", \"第一条\"]\n```"
	values, err := parseMemoryArray(text)
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 2 || values[0] != "第一条" || values[1] != "第二条" {
		t.Fatalf("解析结果异常: %#v", values)
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
	if got := defaultEvents["daze"].Weight; got != 50 {
		t.Fatalf("发呆权重应为 50，实际 %d", got)
	}
}
