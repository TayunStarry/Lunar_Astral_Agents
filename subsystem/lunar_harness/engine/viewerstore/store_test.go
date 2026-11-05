package viewerstore

import (
	"fmt"
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
)

// isolateViewerDB 为每个用例准备独立的观影记忆库。
func isolateViewerDB(t *testing.T) {
	t.Helper()
	oldDir := config.LocalDir
	config.LocalDir = t.TempDir()
	Close()
	if err := Init(); err != nil {
		t.Fatalf("初始化观影记忆库失败: %v", err)
	}
	t.Cleanup(func() {
		Close()
		config.LocalDir = oldDir
	})
}

// TestObservationUpsertCoversSameBV 同一 BV 重复观看覆盖更新，只留一行。
func TestObservationUpsertCoversSameBV(t *testing.T) {
	isolateViewerDB(t)
	bv := "BV1aa411c7mD"
	first := Observation{BV: bv, Title: "旧标题", URL: "https://www.bilibili.com/video/" + bv, Conclusion: "旧结论。", UpdatedAt: 100}
	if err := SaveObservation(first); err != nil {
		t.Fatal(err)
	}
	second := Observation{BV: bv, Title: "新标题", URL: first.URL, Topics: "游戏、解说", Conclusion: "新结论。", UpdatedAt: 200}
	if err := SaveObservation(second); err != nil {
		t.Fatal(err)
	}
	observations, err := LoadObservations()
	if err != nil {
		t.Fatal(err)
	}
	if len(observations) != 1 {
		t.Fatalf("同 BV 应只留 1 行，实际 %d", len(observations))
	}
	if observations[0].Title != "新标题" || observations[0].Conclusion != "新结论。" {
		t.Fatalf("覆盖更新失败: %+v", observations[0])
	}
}

// TestObservationFIFOArchivesOldest 观感超过 6 条时按最旧淘汰，淘汰前先归档。
func TestObservationFIFOArchivesOldest(t *testing.T) {
	isolateViewerDB(t)
	for index := 1; index <= ObservationMax+1; index++ {
		obs := Observation{
			BV:         fmt.Sprintf("BV1aa411c7m%d", index),
			Title:      "视频",
			Conclusion: "结论。",
			UpdatedAt:  int64(index),
		}
		if err := SaveObservation(obs); err != nil {
			t.Fatalf("第 %d 条写入失败: %v", index, err)
		}
	}
	observations, err := LoadObservations()
	if err != nil {
		t.Fatal(err)
	}
	if len(observations) != ObservationMax {
		t.Fatalf("观感应维持 %d 条，实际 %d", ObservationMax, len(observations))
	}
	if observations[0].UpdatedAt != 2 {
		t.Fatalf("最旧一条应被淘汰，实际首条时间 %d", observations[0].UpdatedAt)
	}
	archived, err := CountRows("viewer_archive")
	if err != nil {
		t.Fatal(err)
	}
	if archived != 1 {
		t.Fatalf("淘汰应先归档 1 条，实际 %d", archived)
	}
}

// TestProfileAndStanceAreSingletons 两个单行槽无论写多少次都只有 1 行。
func TestProfileAndStanceAreSingletons(t *testing.T) {
	isolateViewerDB(t)
	for _, value := range []string{"游戏。", "音乐。", "游戏、解说。"} {
		if err := SaveProfile(value); err != nil {
			t.Fatal(err)
		}
	}
	for _, value := range []string{"还行。", "挺喜欢。"} {
		if err := SaveStance(value); err != nil {
			t.Fatal(err)
		}
	}
	if count, err := CountRows("viewer_profile"); err != nil || count != 1 {
		t.Fatalf("选片口味应恒为 1 行，实际 %d err=%v", count, err)
	}
	if count, err := CountRows("viewer_stance"); err != nil || count != 1 {
		t.Fatalf("评论倾向应恒为 1 行，实际 %d err=%v", count, err)
	}
	if value, _, ok, _ := LoadProfile(); !ok || value != "游戏、解说。" {
		t.Fatalf("选片口味应是最新值，实际 %q", value)
	}
	if value, _, ok, _ := LoadStance(); !ok || value != "挺喜欢。" {
		t.Fatalf("评论倾向应是最新值，实际 %q", value)
	}
}

// TestRoundFIFOArchivesOldest 观影轮次超过上限时按最旧淘汰并归档。
func TestRoundFIFOArchivesOldest(t *testing.T) {
	isolateViewerDB(t)
	for index := 1; index <= RoundMax+1; index++ {
		round := Round{
			ID:         fmt.Sprintf("round-%d", index),
			BVIDs:      "BV1aa411c7mD",
			Conclusion: "结论。",
			CreatedAt:  int64(index),
		}
		if err := SaveRound(round); err != nil {
			t.Fatal(err)
		}
	}
	count, err := CountRows("viewer_round")
	if err != nil {
		t.Fatal(err)
	}
	if count != RoundMax {
		t.Fatalf("轮次应维持 %d 条，实际 %d", RoundMax, count)
	}
	if archived, err := CountRows("viewer_archive"); err != nil || archived != 1 {
		t.Fatalf("轮次淘汰应先归档 1 条，实际 %d err=%v", archived, err)
	}
}

// TestOpenStoreKeepsOpenerParams 替换为统一 opener 后连接参数与读写行为不变。
func TestOpenStoreKeepsOpenerParams(t *testing.T) {
	isolateViewerDB(t)
	db, err := openStore()
	if err != nil {
		t.Fatalf("取连接失败: %v", err)
	}
	var busy int64
	if err := db.QueryRow("PRAGMA busy_timeout").Scan(&busy); err != nil {
		t.Fatalf("读取 busy_timeout 失败: %v", err)
	}
	if busy != 10000 {
		t.Fatalf("busy_timeout 应为 10000，实际 %d", busy)
	}
	var mode string
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("读取 journal_mode 失败: %v", err)
	}
	if mode != "wal" {
		t.Fatalf("journal_mode 应为 wal，实际 %q", mode)
	}
	if err := SaveObservation(Observation{BV: "BV1regress1", Title: "回归", Conclusion: "结论。", UpdatedAt: 1}); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	observations, err := LoadObservations()
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if len(observations) != 1 || observations[0].BV != "BV1regress1" {
		t.Fatalf("读回结果异常: %+v", observations)
	}
}
