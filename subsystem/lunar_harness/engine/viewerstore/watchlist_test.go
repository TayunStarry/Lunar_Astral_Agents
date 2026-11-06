package viewerstore

// watchlist_test.go 待看视频列表与片段存储的验收用例（全部使用 t.TempDir）。

import (
	"path/filepath"
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/dbopen"
)

// watchCandidate 构造一条带片段的入队候选。
func watchCandidate(bv string, segments ...SegmentInput) WatchCandidate {
	return WatchCandidate{
		BV:          bv,
		Title:       "标题 " + bv,
		Intro:       "简介",
		AuthorUID:   "1001",
		AuthorName:  "作者",
		Comments:    []string{"评论一", "评论二"},
		DurationSec: 600,
		Segments:    segments,
	}
}

// segments 生成 count 个互不重叠的片段（默认都已下载）。
func segments(count int) []SegmentInput {
	out := make([]SegmentInput, 0, count)
	for index := 0; index < count; index++ {
		out = append(out, SegmentInput{
			StartSec:   index * 30,
			EndSec:     index*30 + 20,
			Path:       "segments/" + string(rune('a'+index)) + ".mp4",
			Downloaded: true,
		})
	}
	return out
}

// TestEnqueueIdempotentByBV 同 BV 重复入队不产生第二条，也不覆盖已有印象。
func TestEnqueueIdempotentByBV(t *testing.T) {
	isolateViewerDB(t)
	first, err := EnqueueVideos([]WatchCandidate{
		watchCandidate("BV1aaa", segments(2)...),
		watchCandidate("BV1bbb", segments(1)...),
		watchCandidate("BV1aaa", segments(3)...), // 同批次内重复，直接丢弃
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.Inserted != 2 || first.Skipped != 0 {
		t.Fatalf("首次入队应写入 2 条，实际 %+v", first)
	}
	duplicate := watchCandidate("BV1aaa", segments(3)...)
	duplicate.Title = "新标题"
	second, err := EnqueueVideos([]WatchCandidate{duplicate})
	if err != nil {
		t.Fatal(err)
	}
	if second.Inserted != 0 || second.Skipped != 1 {
		t.Fatalf("重复入队应跳过，实际 %+v", second)
	}
	if count, err := CountRows("watch_video"); err != nil || count != 2 {
		t.Fatalf("待看视频应恒为 2 条，实际 %d err=%v", count, err)
	}
	video, ok, err := LoadVideo("BV1aaa")
	if err != nil || !ok {
		t.Fatalf("读取视频失败 ok=%v err=%v", ok, err)
	}
	if video.Title != "标题 BV1aaa" {
		t.Fatalf("重复入队不应覆盖原有印象，实际标题 %q", video.Title)
	}
	if len(video.Comments) != 2 || len(segmentsOf(t, "BV1aaa")) != 2 {
		t.Fatalf("评论/片段应与首次入队一致: %+v", video)
	}
}

// TestReviewStateTransition 审核状态流转：待审 → 通过 → 未通过（带原因）→ 通过。
func TestReviewStateTransition(t *testing.T) {
	isolateViewerDB(t)
	if _, err := EnqueueVideos([]WatchCandidate{watchCandidate("BV1rev", segments(1)...)}); err != nil {
		t.Fatal(err)
	}
	approved, err := LoadApprovedVideos()
	if err != nil {
		t.Fatal(err)
	}
	if len(approved) != 0 {
		t.Fatalf("待审视频不应出现在通过列表，实际 %d", len(approved))
	}
	if err := SetVideoReview("BV1rev", WatchStatusApproved, ""); err != nil {
		t.Fatal(err)
	}
	if approved, err = LoadApprovedVideos(); err != nil || len(approved) != 1 {
		t.Fatalf("通过后应出现在列表，实际 %d err=%v", len(approved), err)
	}
	if err := SetVideoReview("BV1rev", WatchStatusRejected, "内容不合适"); err != nil {
		t.Fatal(err)
	}
	video, ok, err := LoadVideo("BV1rev")
	if err != nil || !ok {
		t.Fatalf("读取视频失败 ok=%v err=%v", ok, err)
	}
	if video.Status != WatchStatusRejected || video.RejectReason != "内容不合适" {
		t.Fatalf("未通过原因未落库: %+v", video)
	}
	if err := SetVideoReview("BV1rev", WatchStatusApproved, "应被清空"); err != nil {
		t.Fatal(err)
	}
	if video, _, _ = LoadVideo("BV1rev"); video.RejectReason != "" {
		t.Fatalf("通过后原因应清空，实际 %q", video.RejectReason)
	}
	if err := SetVideoReview("BV1rev", "unknown", ""); err == nil {
		t.Fatal("非法状态应报错")
	}
	if err := SetVideoReview("BV1missing", WatchStatusApproved, ""); err == nil {
		t.Fatal("不存在的视频应报错")
	}
}

// TestWatchlistEmptyLifecycle 空库为空；仅待审视频也算空；通过后非空。
func TestWatchlistEmptyLifecycle(t *testing.T) {
	isolateViewerDB(t)
	if empty, err := WatchlistEmpty(); err != nil || !empty {
		t.Fatalf("新库待看列表应为空 empty=%v err=%v", empty, err)
	}
	if _, err := EnqueueVideos([]WatchCandidate{watchCandidate("BV1empty", segments(1)...)}); err != nil {
		t.Fatal(err)
	}
	if empty, err := WatchlistEmpty(); err != nil || !empty {
		t.Fatalf("待审视频不应算待看 empty=%v err=%v", empty, err)
	}
	if err := SetVideoReview("BV1empty", WatchStatusApproved, ""); err != nil {
		t.Fatal(err)
	}
	if empty, err := WatchlistEmpty(); err != nil || empty {
		t.Fatalf("审核通过后待看列表应非空 empty=%v err=%v", empty, err)
	}
}

// TestWatchStateSurvivesReopen 关闭连接再打开，进度与计数读回一致。
func TestWatchStateSurvivesReopen(t *testing.T) {
	isolateViewerDB(t)
	if _, err := EnqueueVideos([]WatchCandidate{
		watchCandidate("BV1persist", segments(2)...),
	}); err != nil {
		t.Fatal(err)
	}
	if err := SetVideoReview("BV1persist", WatchStatusApproved, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := MarkSegmentWatched("BV1persist", 0, 300); err != nil {
		t.Fatal(err)
	}
	Close()
	if err := Init(); err != nil {
		t.Fatalf("重开数据库失败: %v", err)
	}
	video, ok, err := LoadVideo("BV1persist")
	if err != nil || !ok || video.Status != WatchStatusApproved || video.Completed {
		t.Fatalf("重开后视频状态不一致: ok=%v video=%+v err=%v", ok, video, err)
	}
	segment, ok, err := NextUnwatchedSegment()
	if err != nil || !ok || segment.BV != "BV1persist" || segment.Index != 1 {
		t.Fatalf("重开后应从第 2 段续跑: ok=%v segment=%+v err=%v", ok, segment, err)
	}
	if count, err := CountCompletedVideos(); err != nil || count != 0 {
		t.Fatalf("重开后完整视频数应为 0，实际 %d err=%v", count, err)
	}
	if _, err := MarkSegmentWatched(segment.BV, segment.Index, 301); err != nil {
		t.Fatal(err)
	}
	if count, err := CountCompletedVideos(); err != nil || count != 1 {
		t.Fatalf("读回进度后看完应计 1，实际 %d err=%v", count, err)
	}
}

// TestOpenLegacyDBAddsWatchTables 旧库（只有 viewer_observation）打开后自动补表，旧数据不变。
func TestOpenLegacyDBAddsWatchTables(t *testing.T) {
	dir := t.TempDir()
	oldDir := config.LocalDir
	config.LocalDir = dir
	Close()
	t.Cleanup(func() {
		Close()
		config.LocalDir = oldDir
	})

	path := filepath.Join(dir, "database", "bilibili", "viewer.db")
	db, err := dbopen.Open(path, dbopen.Options{CreateDir: true})
	if err != nil {
		t.Fatalf("预置旧库失败: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE viewer_observation (
		bv TEXT PRIMARY KEY, title TEXT NOT NULL DEFAULT '', url TEXT NOT NULL DEFAULT '',
		topics TEXT NOT NULL DEFAULT '', tone TEXT NOT NULL DEFAULT '',
		conclusion TEXT NOT NULL DEFAULT '', updated_at INTEGER NOT NULL DEFAULT 0)`); err != nil {
		t.Fatalf("建旧表失败: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO viewer_observation (bv, title, updated_at)
		VALUES ('BV1legacy', '旧观感', 1)`); err != nil {
		t.Fatalf("写入旧数据失败: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("关闭旧库失败: %v", err)
	}

	if err := Init(); err != nil {
		t.Fatalf("旧库打开应自动补表: %v", err)
	}
	if count, err := CountRows("watch_video"); err != nil || count != 0 {
		t.Fatalf("补表后 watch_video 应为空表，实际 %d err=%v", count, err)
	}
	if count, err := CountRows("watch_segment"); err != nil || count != 0 {
		t.Fatalf("补表后 watch_segment 应为空表，实际 %d err=%v", count, err)
	}
	if observations, err := LoadObservations(); err != nil || len(observations) != 1 {
		t.Fatalf("旧数据应保持可读，实际 %d err=%v", len(observations), err)
	}
}

// TestSegmentValidationRejectsOverlap 片段重叠或超量时拒绝入队，不写半条数据。
func TestSegmentValidationRejectsOverlap(t *testing.T) {
	isolateViewerDB(t)
	overlap := []SegmentInput{
		{StartSec: 0, EndSec: 20, Path: "a.mp4"},
		{StartSec: 10, EndSec: 30, Path: "b.mp4"},
	}
	if _, err := EnqueueVideos([]WatchCandidate{watchCandidate("BV1bad", overlap...)}); err == nil {
		t.Fatal("重叠片段应报错")
	}
	if count, err := CountRows("watch_video"); err != nil || count != 0 {
		t.Fatalf("校验失败不应写入任何视频，实际 %d err=%v", count, err)
	}
	many := make([]SegmentInput, WatchSegmentMax+1)
	for index := range many {
		many[index] = SegmentInput{StartSec: index * 10, EndSec: index*10 + 5, Path: "x.mp4"}
	}
	if _, err := EnqueueVideos([]WatchCandidate{watchCandidate("BV1many", many...)}); err == nil {
		t.Fatal("片段超量应报错")
	}
}

// segmentsOf 读取一个视频的片段（测试辅助）。
func segmentsOf(t *testing.T, bv string) []WatchSegment {
	t.Helper()
	items, err := LoadSegments(bv)
	if err != nil {
		t.Fatalf("读取片段失败: %v", err)
	}
	return items
}
