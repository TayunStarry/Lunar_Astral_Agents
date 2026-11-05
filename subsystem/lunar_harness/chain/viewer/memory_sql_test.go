package viewer

import (
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/viewerstore"
)

// isolateViewerDB 为每个用例准备独立的观影记忆库。
func isolateViewerDB(t *testing.T) {
	t.Helper()
	oldDir := config.LocalDir
	config.LocalDir = t.TempDir()
	viewerstore.Close()
	if err := viewerstore.Init(); err != nil {
		t.Fatalf("初始化观影记忆库失败: %v", err)
	}
	t.Cleanup(func() {
		viewerstore.Close()
		config.LocalDir = oldDir
	})
}

// TestBuildObservationKeepsOnlyConclusion 观感只保留首句结论。
func TestBuildObservationKeepsOnlyConclusion(t *testing.T) {
	video := &videoRecord{
		Info: videoInfo{
			Bvid:  "BV1xx411c7mD",
			Title: "测试视频",
			URL:   "https://www.bilibili.com/video/BV1xx411c7mD",
		},
		Impression:   videoImpression{Topics: []string{"游戏", "解说"}, Tone: "轻松"},
		WatchSummary: "开头铺垫有点长。中段节奏起来了，后面收尾仓促。整体还算好看。",
	}
	obs := buildObservation(video)
	if obs.BV != "BV1xx411c7mD" || obs.Title != "测试视频" {
		t.Fatalf("观感字段不完整: %+v", obs)
	}
	if obs.Topics != "游戏、解说" || obs.Tone != "轻松" {
		t.Fatalf("主题/氛围不符: %+v", obs)
	}
	if obs.Conclusion != "开头铺垫有点长。" {
		t.Fatalf("应只保留首句结论，实际 %q", obs.Conclusion)
	}
	if strings.Contains(obs.Conclusion, "中段节奏起来了") {
		t.Fatalf("结论不应包含完整摘要: %q", obs.Conclusion)
	}
}

// TestBuildSelectionProfileFromRecentObservations 选片口味由近期观感的主题聚合而来。
func TestBuildSelectionProfileFromRecentObservations(t *testing.T) {
	observations := []viewerstore.Observation{
		{Topics: "游戏、解说"},
		{Topics: "游戏、紧凑"},
	}
	got := buildSelectionProfile(observations)
	if !strings.HasPrefix(got, "最近常看") || !strings.Contains(got, "游戏") {
		t.Fatalf("选片口味 = %q", got)
	}
	if got := buildSelectionProfile(nil); got == "" {
		t.Fatal("没有观感时也应给出兜底口味")
	}
}

// TestMemoryConclusionFallsBackToHardLimit 无句号时按硬上限截断。
func TestMemoryConclusionFallsBackToHardLimit(t *testing.T) {
	long := strings.Repeat("没有句号的长句", 200)
	got := viewerstore.Conclusion(long)
	if len([]rune(got)) > viewerstore.ConclusionMaxRunes {
		t.Fatalf("conclusion too long: %d", len([]rune(got)))
	}
	if got == "" {
		t.Fatal("conclusion should not be empty")
	}
}

// countingLegacySource 只统计扫描次数，用于核验读取链路不触碰记忆库。
type countingLegacySource struct{ scans *int }

// ListText 记录被扫描的次数（读取链路不应触发）。
func (c countingLegacySource) ListText(int, int) ([]viewerstore.LegacyMemoryEntry, int, error) {
	*c.scans++
	return nil, 0, nil
}

// DeleteEntry 计数场景下不会发生删除。
func (countingLegacySource) DeleteEntry(string) error { return nil }

// TestReadChainDoesNotTouchMemoryLibrary 评论/选片读取链路只读 SQL，不触碰统一记忆库。
func TestReadChainDoesNotTouchMemoryLibrary(t *testing.T) {
	isolateViewerDB(t)
	if err := viewerstore.SaveObservation(viewerstore.Observation{
		BV: "BV1aa411c7mD", Title: "A", URL: "https://www.bilibili.com/video/BV1aa411c7mD",
		Topics: "游戏", Conclusion: "开头不错。", UpdatedAt: 100,
	}); err != nil {
		t.Fatal(err)
	}
	if err := viewerstore.SaveProfile("最近常看游戏。"); err != nil {
		t.Fatal(err)
	}
	if err := viewerstore.SaveStance("挺喜欢。"); err != nil {
		t.Fatal(err)
	}

	scans := 0
	oldSource := viewerMemorySource
	viewerMemorySource = countingLegacySource{scans: &scans}
	t.Cleanup(func() { viewerMemorySource = oldSource })

	material := memoryMaterial(12)
	for _, want := range []string{"开头不错。", "选片口味：", "评论倾向："} {
		if !strings.Contains(material, want) {
			t.Fatalf("记忆材料缺少 %q: %q", want, material)
		}
	}
	if text := recentMemoryText(12); !strings.Contains(text, "BV1aa411c7mD") {
		t.Fatalf("近期观感应从 SQL 读取: %q", text)
	}
	refreshSelectionProfile()
	if scans != 0 {
		t.Fatalf("读取链路不应访问统一记忆库，实际调用 %d 次", scans)
	}
}
