package viewerstore

// watchsegment_test.go 片段取用顺序、完整视频计数与重新切片（全部使用 t.TempDir）。

import "testing"

// TestNextUnwatchedSegmentOrder 取片顺序按视频入队顺序，再看片段序号；未下载片段跳过。
func TestNextUnwatchedSegmentOrder(t *testing.T) {
	isolateViewerDB(t)
	late := watchCandidate("BV1zzz", segments(2)...)
	early := watchCandidate("BV1aaa", segments(2)...)
	late.Segments[1].Downloaded = false // 第二段还没下载，应被跳过
	if _, err := EnqueueVideos([]WatchCandidate{late, early}); err != nil {
		t.Fatal(err)
	}
	for _, bv := range []string{"BV1zzz", "BV1aaa"} {
		if err := SetVideoReview(bv, WatchStatusApproved, ""); err != nil {
			t.Fatal(err)
		}
	}
	want := []struct {
		bv    string
		index int
	}{{"BV1zzz", 0}, {"BV1aaa", 0}, {"BV1aaa", 1}}
	for step, expect := range want {
		segment, ok, err := NextUnwatchedSegment()
		if err != nil || !ok {
			t.Fatalf("第 %d 步取片失败 ok=%v err=%v", step, ok, err)
		}
		if segment.BV != expect.bv || segment.Index != expect.index {
			t.Fatalf("第 %d 步应取 %s#%d，实际 %s#%d",
				step, expect.bv, expect.index, segment.BV, segment.Index)
		}
		if _, err := MarkSegmentWatched(segment.BV, segment.Index, 0, nil); err != nil {
			t.Fatal(err)
		}
	}
	if _, ok, err := NextUnwatchedSegment(); err != nil || ok {
		t.Fatalf("全部看完后应无片段 ok=%v err=%v", ok, err)
	}
}

// TestClearSegmentPathDropsLocalPath 删除文件后清空路径：记录不再指向已删文件，其它片段不受影响。
func TestClearSegmentPathDropsLocalPath(t *testing.T) {
	isolateViewerDB(t)
	if _, err := EnqueueVideos([]WatchCandidate{watchCandidate("BV1clr", segments(2)...)}); err != nil {
		t.Fatal(err)
	}
	before, err := LoadSegments("BV1clr")
	if err != nil || len(before) != 2 {
		t.Fatalf("准备片段失败 len=%d err=%v", len(before), err)
	}
	if before[0].Path == "" {
		t.Fatal("前置条件：片段应有本地路径")
	}
	if err := ClearSegmentPath("BV1clr", 0); err != nil {
		t.Fatal(err)
	}
	after, err := LoadSegments("BV1clr")
	if err != nil {
		t.Fatal(err)
	}
	if after[0].Path != "" {
		t.Fatalf("目标片段路径应清空，实际 %q", after[0].Path)
	}
	if after[1].Path != before[1].Path {
		t.Fatalf("不应影响其它片段：before=%q after=%q", before[1].Path, after[1].Path)
	}
	if err := ClearSegmentPath("", 0); err == nil {
		t.Fatal("缺少 BV 号时应报错")
	}
}

// TestCompletedVideoCountIsVideoLevel 一个视频 3 片段：看 2 个不算完成，看完才算 1 个完整视频。
func TestCompletedVideoCountIsVideoLevel(t *testing.T) {
	isolateViewerDB(t)
	if _, err := EnqueueVideos([]WatchCandidate{watchCandidate("BV1cnt", segments(3)...)}); err != nil {
		t.Fatal(err)
	}
	if err := SetVideoReview("BV1cnt", WatchStatusApproved, ""); err != nil {
		t.Fatal(err)
	}
	if empty, err := WatchlistEmpty(); err != nil || empty {
		t.Fatalf("有未看完视频时待看列表应非空 empty=%v err=%v", empty, err)
	}
	for index := 0; index < 2; index++ {
		result, err := MarkSegmentWatched("BV1cnt", index, int64(100+index), nil)
		if err != nil {
			t.Fatal(err)
		}
		if result.VideoCompleted || result.CompletedVideos != 0 {
			t.Fatalf("看 2/3 段不应算完整视频: %+v", result)
		}
	}
	result, err := MarkSegmentWatched("BV1cnt", 2, 200, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !result.VideoCompleted || result.CompletedVideos != 1 {
		t.Fatalf("看完全部片段应计 1 个完整视频: %+v", result)
	}
	if count, err := CountCompletedVideos(); err != nil || count != 1 {
		t.Fatalf("完整视频数应为 1，实际 %d err=%v", count, err)
	}
	if archived, err := ArchiveCompletedVideos(); err != nil || archived != 1 {
		t.Fatalf("应归档 1 个已完成视频，实际 %d err=%v", archived, err)
	}
	if count, err := CountCompletedVideos(); err != nil || count != 1 {
		t.Fatalf("归档后完整视频数仍应为 1，实际 %d err=%v", count, err)
	}
	if empty, err := WatchlistEmpty(); err != nil || !empty {
		t.Fatalf("全部看完并归档后待看列表应为空 empty=%v err=%v", empty, err)
	}
}

// TestReplaceSegmentsAfterEnqueue 入队时没有片段，下载阶段补写后即可续跑；重新切片会重置完成状态。
func TestReplaceSegmentsAfterEnqueue(t *testing.T) {
	isolateViewerDB(t)
	if _, err := EnqueueVideos([]WatchCandidate{watchCandidate("BV1late")}); err != nil {
		t.Fatal(err)
	}
	if err := SetVideoReview("BV1late", WatchStatusApproved, ""); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := NextUnwatchedSegment(); err != nil || ok {
		t.Fatalf("尚未切片时不应取到片段 ok=%v err=%v", ok, err)
	}
	if err := ReplaceSegments("BV1late", segments(2)); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 2; index++ {
		if _, err := MarkSegmentWatched("BV1late", index, 400+int64(index), nil); err != nil {
			t.Fatal(err)
		}
	}
	if count, err := CountCompletedVideos(); err != nil || count != 1 {
		t.Fatalf("重新切片前应算 1 个完整视频，实际 %d err=%v", count, err)
	}
	if err := ReplaceSegments("BV1late", segments(3)); err != nil {
		t.Fatal(err)
	}
	if count, err := CountCompletedVideos(); err != nil || count != 0 {
		t.Fatalf("重新切片后完成状态应重置为 0，实际 %d err=%v", count, err)
	}
	segment, ok, err := NextUnwatchedSegment()
	if err != nil || !ok || segment.Index != 0 {
		t.Fatalf("重新切片后应从第 1 段开始: ok=%v segment=%+v err=%v", ok, segment, err)
	}
}
