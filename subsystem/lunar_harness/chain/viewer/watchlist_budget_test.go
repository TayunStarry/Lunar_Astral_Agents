package viewer

// watchlist_budget_test.go 整阶段时间预算：到点提前结束、已完成成果保留、未处理候选不留记录。

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"LunarSubsystem/LunarHarness/engine/viewerstore"
)

// fakeBudgetClock 可手动推进的时钟，让预算耗尽时机可复现（不依赖真实等待）。
type fakeBudgetClock struct {
	mu  sync.Mutex
	now time.Time
}

// newFakeBudgetClock 返回从固定时刻起步的假时钟。
func newFakeBudgetClock() *fakeBudgetClock {
	return &fakeBudgetClock{now: time.Unix(1_700_000_000, 0)}
}

// Now 当前（假）时间。
func (c *fakeBudgetClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Advance 把时间向前推进 d。
func (c *fakeBudgetClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

// newBudgetTestDeps 构造「采集 20s / 审核 5s / 下载 5s」的预算用例依赖，总预算 90s。
//
// 采集阶段份额为 90s 的 2/3（60s），所以只会采集 3 个候选；
// 审核与下载用剩余的 30s，正好能全部完成。
func newBudgetTestDeps(links []videoLink, clock *fakeBudgetClock, downloadCost time.Duration) (watchlistDeps, *fakeWatchDownloader) {
	downloader := &fakeWatchDownloader{onCall: func() { clock.Advance(downloadCost) }}
	deps := newWatchlistTestDeps(&fakeWatchSession{links: links}, downloader)
	deps.want = len(links)
	deps.now = clock.Now
	deps.budget = watchlistBudget{total: 90 * time.Second, perItem: 60 * time.Second}
	deps.impress = func(_ context.Context, video *videoRecord) videoImpression {
		clock.Advance(20 * time.Second)
		return videoImpression{Summary: "摘要 " + video.Info.Title}
	}
	deps.review = func(_ context.Context, _ *videoRecord) videoReview {
		clock.Advance(5 * time.Second)
		return videoReview{Pass: true, Slices: 2}
	}
	return deps, downloader
}

// TestEnsureWatchlistBudgetStopsCollectKeepsProgress 采集阶段用尽份额即停，
// 已采集的候选仍完成审核、入队与下载；未采集的候选不留任何记录。
func TestEnsureWatchlistBudgetStopsCollectKeepsProgress(t *testing.T) {
	isolateViewerDB(t)
	links := watchTestLinks(6)
	deps, _ := newBudgetTestDeps(links, newFakeBudgetClock(), 5*time.Second)

	result, err := ensureWatchlist(context.Background(), true, deps)
	if err != nil {
		t.Fatalf("管线执行失败: %v", err)
	}
	if !result.BudgetExhausted {
		t.Fatalf("应标记预算耗尽: %+v", result)
	}
	if result.Candidates != 6 || result.Impressions != 3 || result.Skipped != 3 {
		t.Fatalf("应只采集 3 个并放弃其余 3 个: %+v", result)
	}
	if result.Approved != 3 || result.Enqueued != 3 || result.Downloaded != 3 || result.Failed != 0 {
		t.Fatalf("已采集的候选应全部完成: %+v", result)
	}
	count, err := viewerstore.CountRows("watch_video")
	if err != nil || count != 3 {
		t.Fatalf("未采集的候选不应留下记录: count=%d err=%v", count, err)
	}
	for index := 0; index < 3; index++ {
		bv := bvidOf(links[index].Href)
		approved, ok, _ := viewerstore.LoadVideo(bv)
		if !ok || approved.Status != viewerstore.WatchStatusApproved {
			t.Fatalf("已完成的候选应可播放: bv=%s %+v", bv, approved)
		}
		assertWatchSegments(t, bv, 2, 2)
	}
}

// TestEnsureWatchlistBudgetDefersRemainingDownloads 预算在下载阶段耗尽时，
// 已下载的保留为 approved，未下载的置为可重试 rejected 并写明原因。
func TestEnsureWatchlistBudgetDefersRemainingDownloads(t *testing.T) {
	isolateViewerDB(t)
	links := watchTestLinks(6)
	deps, _ := newBudgetTestDeps(links, newFakeBudgetClock(), 20*time.Second)

	result, err := ensureWatchlist(context.Background(), true, deps)
	if err != nil {
		t.Fatalf("管线执行失败: %v", err)
	}
	if !result.BudgetExhausted || result.Downloaded != 1 || result.Enqueued != 3 || result.Skipped != 5 {
		t.Fatalf("应下载 1 个、顺延 2 个、采集放弃 3 个: %+v", result)
	}
	approved, _, _ := viewerstore.LoadVideo(bvidOf(links[0].Href))
	if approved.Status != viewerstore.WatchStatusApproved {
		t.Fatalf("已下载的应保留为 approved: %+v", approved)
	}
	assertWatchSegments(t, bvidOf(links[0].Href), 2, 2)
	for index := 1; index < 3; index++ {
		bv := bvidOf(links[index].Href)
		deferred, ok, _ := viewerstore.LoadVideo(bv)
		if !ok || deferred.Status != viewerstore.WatchStatusRejected ||
			!strings.HasPrefix(deferred.RejectReason, watchlistDownloadFailPrefix) {
			t.Fatalf("未下载的应顺延为可重试 rejected: bv=%s %+v", bv, deferred)
		}
		if segments, _ := viewerstore.LoadSegments(bv); len(segments) != 0 {
			t.Fatalf("顺延的候选不应留下片段: bv=%s len=%d", bv, len(segments))
		}
	}
}
