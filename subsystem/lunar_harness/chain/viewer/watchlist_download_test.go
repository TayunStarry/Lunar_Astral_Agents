package viewer

// watchlist_download_test.go 切片规划、下载失败自愈与入队幂等测试。

import (
	"context"
	"errors"
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/engine/viewerstore"
)

// TestEnsureWatchlistSingleFailureDoesNotBlockOthers 单条下载失败不影响其他视频。
func TestEnsureWatchlistSingleFailureDoesNotBlockOthers(t *testing.T) {
	isolateViewerDB(t)
	links := watchTestLinks(2)
	failBV := bvidOf(links[0].Href)
	okBV := bvidOf(links[1].Href)
	session := &fakeWatchSession{links: links}
	downloader := &fakeWatchDownloader{fail: map[string]bool{failBV: true}}
	deps := newWatchlistTestDeps(session, downloader)
	deps.want = 2

	result, err := ensureWatchlist(context.Background(), true, deps)
	if err != nil {
		t.Fatalf("管线执行失败: %v", err)
	}
	if result.Downloaded != 1 || result.Failed != 1 || result.Approved != 2 {
		t.Fatalf("失败隔离结果不符: %+v", result)
	}
	failed, ok, err := viewerstore.LoadVideo(failBV)
	if err != nil || !ok {
		t.Fatalf("失败视频记录应保留: ok=%v err=%v", ok, err)
	}
	if failed.Status != viewerstore.WatchStatusRejected ||
		!strings.HasPrefix(failed.RejectReason, watchlistDownloadFailPrefix) {
		t.Fatalf("失败视频状态不符: %+v", failed)
	}
	if segments, _ := viewerstore.LoadSegments(failBV); len(segments) != 0 {
		t.Fatalf("失败视频不应留下片段: %d", len(segments))
	}
	assertWatchSegments(t, okBV, 1, viewerstore.WatchSegmentMax)
	approved, _, _ := viewerstore.LoadVideo(okBV)
	if approved.Status != viewerstore.WatchStatusApproved {
		t.Fatalf("成功视频应为 approved: %+v", approved)
	}
}

// TestEnsureWatchlistDownloadFailureRetriesNextRound 下载失败后待看列表仍为空，下一轮可重试并自愈。
func TestEnsureWatchlistDownloadFailureRetriesNextRound(t *testing.T) {
	isolateViewerDB(t)
	links := watchTestLinks(1)
	bv := bvidOf(links[0].Href)
	session := &fakeWatchSession{links: links}
	downloader := &fakeWatchDownloader{fail: map[string]bool{bv: true}}
	deps := newWatchlistTestDeps(session, downloader)
	deps.want = 1

	if _, err := ensureWatchlist(context.Background(), true, deps); err != nil {
		t.Fatalf("首轮执行失败: %v", err)
	}
	empty, err := viewerstore.WatchlistEmpty()
	if err != nil || !empty {
		t.Fatalf("下载失败后待看列表应视为空，避免卡死: empty=%v err=%v", empty, err)
	}
	failed, _, _ := viewerstore.LoadVideo(bv)
	if failed.Status != viewerstore.WatchStatusRejected {
		t.Fatalf("首轮应保留可重试的 rejected 记录: %+v", failed)
	}

	downloader.fail = nil
	second, err := ensureWatchlist(context.Background(), true, deps)
	if err != nil {
		t.Fatalf("重试轮执行失败: %v", err)
	}
	if !second.Triggered || second.Downloaded != 1 || second.Failed != 0 {
		t.Fatalf("重试轮未自愈: %+v", second)
	}
	approved, _, _ := viewerstore.LoadVideo(bv)
	if approved.Status != viewerstore.WatchStatusApproved {
		t.Fatalf("重试成功后应置为 approved: %+v", approved)
	}
	assertWatchSegments(t, bv, 1, viewerstore.WatchSegmentMax)
}

// TestEnsureWatchlistApprovedWriteFailureSelfHeals 片段已落盘但置 approved 失败时，
// 回退为 rejected+原因，下一轮重新下载并推到 approved，视频不会静默丢失。
func TestEnsureWatchlistApprovedWriteFailureSelfHeals(t *testing.T) {
	isolateViewerDB(t)
	links := watchTestLinks(1)
	bv := bvidOf(links[0].Href)
	deps := newWatchlistTestDeps(&fakeWatchSession{links: links}, &fakeWatchDownloader{})
	deps.want = 1
	realSet := deps.setReview
	failed := false
	deps.setReview = func(target string, status viewerstore.WatchStatus, reason string) error {
		if status == viewerstore.WatchStatusApproved && !failed {
			failed = true
			return errors.New("模拟置为可播放写入失败")
		}
		return realSet(target, status, reason)
	}

	first, err := ensureWatchlist(context.Background(), true, deps)
	if err != nil {
		t.Fatalf("首轮执行失败: %v", err)
	}
	if first.Failed != 1 || first.Downloaded != 0 {
		t.Fatalf("首轮应记录失败且不可播放: %+v", first)
	}
	video, ok, err := viewerstore.LoadVideo(bv)
	if err != nil || !ok {
		t.Fatalf("记录应保留: ok=%v err=%v", ok, err)
	}
	if video.Status != viewerstore.WatchStatusRejected ||
		!strings.HasPrefix(video.RejectReason, watchlistDownloadFailPrefix) {
		t.Fatalf("应回退为可重试的 rejected 并写明原因: %+v", video)
	}
	// 片段已落盘：库里的片段状态必须与磁盘一致（已下载），不能出现「文件在但库里没有」。
	segments, err := viewerstore.LoadSegments(bv)
	if err != nil || len(segments) == 0 {
		t.Fatalf("片段记录应保留: len=%d err=%v", len(segments), err)
	}
	for _, segment := range segments {
		if !segment.Downloaded || segment.Path == "" {
			t.Fatalf("片段应标记为已下载: %+v", segment)
		}
	}
	empty, err := viewerstore.WatchlistEmpty()
	if err != nil || !empty {
		t.Fatalf("rejected 不应阻塞后续抓取: empty=%v err=%v", empty, err)
	}

	second, err := ensureWatchlist(context.Background(), true, deps)
	if err != nil {
		t.Fatalf("重试轮执行失败: %v", err)
	}
	if second.Downloaded != 1 || second.Failed != 0 {
		t.Fatalf("重试轮未自愈: %+v", second)
	}
	approved, _, _ := viewerstore.LoadVideo(bv)
	if approved.Status != viewerstore.WatchStatusApproved {
		t.Fatalf("重试成功后应置为 approved: %+v", approved)
	}
	assertWatchSegments(t, bv, 1, watchSliceMax)
}

// TestEnsureWatchlistPendingWithSegmentsRetries 置 approved 与回退写 rejected 都失败时，行停在 pending；
// pending 一律视为可重试，否则这行永远不可播也不会被救回。
func TestEnsureWatchlistPendingWithSegmentsRetries(t *testing.T) {
	isolateViewerDB(t)
	links := watchTestLinks(1)
	bv := bvidOf(links[0].Href)
	deps := newWatchlistTestDeps(&fakeWatchSession{links: links}, &fakeWatchDownloader{})
	deps.want = 1
	realSet := deps.setReview
	broken := true
	deps.setReview = func(target string, status viewerstore.WatchStatus, reason string) error {
		if broken {
			return errors.New("模拟数据库写入失败")
		}
		return realSet(target, status, reason)
	}

	if _, err := ensureWatchlist(context.Background(), true, deps); err != nil {
		t.Fatalf("首轮执行失败: %v", err)
	}
	video, _, _ := viewerstore.LoadVideo(bv)
	if video.Status != viewerstore.WatchStatusPending {
		t.Fatalf("两次写入都失败时应停在 pending: %+v", video)
	}
	segments, err := viewerstore.LoadSegments(bv)
	if err != nil || len(segments) == 0 {
		t.Fatalf("片段记录应保留: len=%d err=%v", len(segments), err)
	}

	broken = false
	second, err := ensureWatchlist(context.Background(), true, deps)
	if err != nil {
		t.Fatalf("重试轮执行失败: %v", err)
	}
	if second.Downloaded != 1 {
		t.Fatalf("pending 且有片段的记录应被重试救回: %+v", second)
	}
	approved, _, _ := viewerstore.LoadVideo(bv)
	if approved.Status != viewerstore.WatchStatusApproved {
		t.Fatalf("重试成功后应置为 approved: %+v", approved)
	}
}

// TestEnsureWatchlistUsesReviewSliceCount 审核给出的切片数必须生效；越界时钳制到 1~5。
func TestEnsureWatchlistUsesReviewSliceCount(t *testing.T) {
	isolateViewerDB(t)
	links := watchTestLinks(2)
	normalBV := bvidOf(links[0].Href)
	overBV := bvidOf(links[1].Href)
	deps := newWatchlistTestDeps(&fakeWatchSession{links: links}, &fakeWatchDownloader{})
	deps.want = 2
	deps.review = func(_ context.Context, video *videoRecord) videoReview {
		if video.Info.Bvid == overBV {
			return videoReview{Pass: true, Slices: watchSliceMax + 4}
		}
		return videoReview{Pass: true, Slices: 3}
	}

	result, err := ensureWatchlist(context.Background(), true, deps)
	if err != nil {
		t.Fatalf("管线执行失败: %v", err)
	}
	if result.Downloaded != 2 {
		t.Fatalf("两个视频都应可播放: %+v", result)
	}
	assertWatchSegments(t, normalBV, 3, 3)
	assertWatchSegments(t, overBV, watchSliceMax, watchSliceMax)
}

// TestPersistWatchItemsIdempotent 过审入库按 BV 幂等，重复执行不新增行。
func TestPersistWatchItemsIdempotent(t *testing.T) {
	isolateViewerDB(t)
	deps := newWatchlistTestDeps(&fakeWatchSession{}, &fakeWatchDownloader{})
	items := []*watchItem{
		{record: &videoRecord{Info: videoInfo{Bvid: "BV1000000001", Title: "A", Duration: 600}}, approved: true},
		{record: &videoRecord{Info: videoInfo{Bvid: "BV1000000002", Title: "B", Duration: 600}}, approved: true},
	}
	first, err := persistWatchItems(items, deps)
	if err != nil || first != 2 {
		t.Fatalf("首次入队结果不符: inserted=%d err=%v", first, err)
	}
	second, err := persistWatchItems(items, deps)
	if err != nil || second != 0 {
		t.Fatalf("重复入队应全部跳过: inserted=%d err=%v", second, err)
	}
	count, err := viewerstore.CountRows("watch_video")
	if err != nil || count != 2 {
		t.Fatalf("重复入队不应新增行: count=%d err=%v", count, err)
	}
}

// TestPlanWatchSegments 片段数量 1~5、互不重叠且不越界。
func TestPlanWatchSegments(t *testing.T) {
	for _, duration := range []int{600, 300, 40, 5} {
		for count := 1; count <= 5; count++ {
			segments := planWatchSegments(duration, count, func(int) int { return 0 })
			if len(segments) < 1 || len(segments) > 5 {
				t.Fatalf("时长 %d 请求 %d 段，实际 %d 段", duration, count, len(segments))
			}
			for index, segment := range segments {
				if segment.StartSec < 0 || segment.EndSec <= segment.StartSec || segment.EndSec > duration {
					t.Fatalf("时长 %d 片段越界: %+v", duration, segment)
				}
				if index > 0 && segment.StartSec < segments[index-1].EndSec {
					t.Fatalf("时长 %d 片段重叠: %+v", duration, segments)
				}
			}
		}
	}
}
