package viewer

// watchlist_test.go 待看列表管线的触发条件、候选筛选、印象结构与审核时序测试。
//
// 所有用例使用 t.TempDir() 隔离 viewer.db，浏览器/模型/下载全部用假实现，不访问 B 站。

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"LunarSubsystem/LunarHarness/engine/viewerstore"
)

// fakeWatchSession 假浏览器：只返回预设视频卡片与固定评论。
type fakeWatchSession struct {
	links    []videoLink
	loginErr error
	opened   bool
	closed   bool
}

func (f *fakeWatchSession) ensureLogin() error { return f.loginErr }

func (f *fakeWatchSession) collectVideoLinks(max int) ([]videoLink, error) {
	out := f.links
	if max > 0 && len(out) > max {
		out = out[:max]
	}
	return out, nil
}

func (f *fakeWatchSession) navigate(string, time.Duration) error { return nil }
func (f *fakeWatchSession) wait(time.Duration)                   {}

func (f *fakeWatchSession) FetchText(string, string, string, string, time.Duration) (string, error) {
	return `{"code":0,"data":{"replies":[{"member":{"uname":"观众甲"},"content":{"message":"这个讲得挺清楚"},"like":8}]}}`, nil
}

func (f *fakeWatchSession) FetchForm(string, map[string]string, time.Duration) (string, error) {
	return `{"code":0}`, nil
}

func (f *fakeWatchSession) readComments(int) ([]commentItem, error) { return nil, nil }
func (f *fakeWatchSession) close()                                  { f.closed = true }

// fakeWatchDownloader 假下载器：按 BV 模拟成功或失败。
type fakeWatchDownloader struct {
	fail  map[string]bool
	calls int
	// onCall 每次下载前的钩子（预算用例用来推进假时钟）。
	onCall func()
}

func (f *fakeWatchDownloader) Download(_ context.Context, info videoInfo, segments []viewerstore.SegmentInput) ([]viewerstore.SegmentInput, error) {
	f.calls++
	if f.onCall != nil {
		f.onCall()
	}
	if f.fail[info.Bvid] {
		return nil, errors.New("模拟下载失败")
	}
	out := make([]viewerstore.SegmentInput, 0, len(segments))
	for _, segment := range segments {
		segment.Path = fmt.Sprintf("fake/%s_%d.mp4", info.Bvid, segment.StartSec)
		segment.Downloaded = true
		out = append(out, segment)
	}
	return out, nil
}

// newWatchlistTestDeps 只替换外部副作用端口，存储仍是真实 viewerstore（指向临时库）。
func newWatchlistTestDeps(session watchSession, downloader watchDownloader) watchlistDeps {
	deps := defaultWatchlistDeps()
	deps.want = watchlistWanted
	deps.open = func() (watchSession, error) { return session, nil }
	deps.fetchInfo = func(bv string) (videoInfo, error) {
		return videoInfo{
			Bvid: bv, Aid: 100, Cid: 200, Title: "视频" + bv, Desc: "简介" + bv,
			Owner: "UP主" + bv, OwnerMid: 42, Duration: 600,
		}, nil
	}
	deps.impress = func(_ context.Context, video *videoRecord) videoImpression {
		return videoImpression{Summary: "摘要 " + video.Info.Title, Topics: []string{"测试"}}
	}
	deps.review = func(_ context.Context, _ *videoRecord) videoReview {
		return videoReview{Pass: true}
	}
	deps.downloader = downloader
	deps.randInt = func(n int) int {
		if n <= 1 {
			return 0
		}
		return n / 2
	}
	// 测试固定用机制默认预算，不受本机 local_data/config/bilibili.json 影响。
	deps.budget = watchlistBudget{total: watchlistBudgetDefault, perItem: watchlistCandidateBudgetDefault}
	deps.now = time.Now
	return deps
}

// watchTestLinks 生成 count 条合法 BV 的候选链接。
func watchTestLinks(count int) []videoLink {
	links := make([]videoLink, 0, count)
	for index := 0; index < count; index++ {
		bv := fmt.Sprintf("BV1%09d", index)
		links = append(links, videoLink{
			Href:  "https://www.bilibili.com/video/" + bv,
			Title: "AI 测试视频 " + bv,
		})
	}
	return links
}

// TestRetryableWatchVideo 钉住重试判定：下载失败前缀与 pending 可重试，其余不重试。
func TestRetryableWatchVideo(t *testing.T) {
	cases := []struct {
		name  string
		video viewerstore.WatchVideo
		want  bool
	}{
		{"pending 未写成可播放", viewerstore.WatchVideo{Status: viewerstore.WatchStatusPending}, true},
		{"下载失败可重试", viewerstore.WatchVideo{
			Status: viewerstore.WatchStatusRejected, RejectReason: watchlistDownloadFailPrefix + "网络错误",
		}, true},
		{"审核拒绝不重试", viewerstore.WatchVideo{
			Status: viewerstore.WatchStatusRejected, RejectReason: "内容不适",
		}, false},
		{"已可播放不重试", viewerstore.WatchVideo{Status: viewerstore.WatchStatusApproved}, false},
	}
	for _, testCase := range cases {
		if got := retryableWatchVideo(testCase.video); got != testCase.want {
			t.Fatalf("%s: got %v want %v", testCase.name, got, testCase.want)
		}
	}
}

// TestEnsureWatchlistTriggerConditions 四个触发条件各自不满足时都不执行。
func TestEnsureWatchlistTriggerConditions(t *testing.T) {
	t.Run("生活链未触发", func(t *testing.T) {
		isolateViewerDB(t)
		opened := false
		deps := newWatchlistTestDeps(&fakeWatchSession{}, &fakeWatchDownloader{})
		deps.open = func() (watchSession, error) {
			opened = true
			return &fakeWatchSession{}, nil
		}
		result, err := ensureWatchlist(context.Background(), false, deps)
		if err != nil {
			t.Fatalf("不应返回错误: %v", err)
		}
		if result.Triggered || result.Reason != "生活链未触发" || opened {
			t.Fatalf("不应执行: %+v opened=%v", result, opened)
		}
	})

	t.Run("待看列表非空", func(t *testing.T) {
		isolateViewerDB(t)
		seedWatchVideo(t, "BV1000000001", viewerstore.WatchStatusApproved)
		opened := false
		deps := newWatchlistTestDeps(&fakeWatchSession{}, &fakeWatchDownloader{})
		deps.open = func() (watchSession, error) {
			opened = true
			return &fakeWatchSession{}, nil
		}
		result, err := ensureWatchlist(context.Background(), true, deps)
		if err != nil {
			t.Fatalf("不应返回错误: %v", err)
		}
		if result.Triggered || result.Reason != "待看列表非空" || opened {
			t.Fatalf("不应执行: %+v opened=%v", result, opened)
		}
	})

	t.Run("页面开启失败", func(t *testing.T) {
		isolateViewerDB(t)
		deps := newWatchlistTestDeps(&fakeWatchSession{}, &fakeWatchDownloader{})
		deps.open = func() (watchSession, error) { return nil, errors.New("窗口打不开") }
		result, err := ensureWatchlist(context.Background(), true, deps)
		if err != nil {
			t.Fatalf("不应返回错误: %v", err)
		}
		if result.Triggered || result.Reason != "Bilibili 页面开启失败" {
			t.Fatalf("不应执行: %+v", result)
		}
	})

	t.Run("账号未登录", func(t *testing.T) {
		isolateViewerDB(t)
		session := &fakeWatchSession{links: watchTestLinks(30), loginErr: errors.New("未登录")}
		impressions := 0
		deps := newWatchlistTestDeps(session, &fakeWatchDownloader{})
		deps.impress = func(_ context.Context, video *videoRecord) videoImpression {
			impressions++
			return videoImpression{Summary: video.Info.Title}
		}
		result, err := ensureWatchlist(context.Background(), true, deps)
		if err != nil {
			t.Fatalf("不应返回错误: %v", err)
		}
		if result.Triggered || result.Reason != "B站账号未登录" || impressions != 0 || !session.closed {
			t.Fatalf("不应采集: %+v impressions=%d closed=%v", result, impressions, session.closed)
		}
	})
}

// TestEnsureWatchlistCollectsAndDownloads 24 候选筛选、印象结构、审核时序与切片下载。
func TestEnsureWatchlistCollectsAndDownloads(t *testing.T) {
	isolateViewerDB(t)
	links := watchTestLinks(32)
	seenBV := bvidOf(links[0].Href)
	writeSeenRecord(t, seenBV)
	pendingBV := bvidOf(links[1].Href)
	if _, err := viewerstore.EnqueueVideos([]viewerstore.WatchCandidate{
		{BV: pendingBV, Title: "已在待看列表", DurationSec: 600},
	}); err != nil {
		t.Fatalf("准备待看记录失败: %v", err)
	}
	baseline, err := viewerstore.CountRows("watch_video")
	if err != nil {
		t.Fatalf("统计基线失败: %v", err)
	}

	session := &fakeWatchSession{links: links}
	downloader := &fakeWatchDownloader{}
	deps := newWatchlistTestDeps(session, downloader)

	var impressions []*videoRecord
	reviewed := make(map[string]bool)
	deps.impress = func(_ context.Context, video *videoRecord) videoImpression {
		impressions = append(impressions, video)
		return videoImpression{Summary: "摘要 " + video.Info.Title}
	}
	deps.review = func(_ context.Context, video *videoRecord) videoReview {
		if count, countErr := viewerstore.CountRows("watch_video"); countErr != nil || count != baseline {
			t.Fatalf("审核未全部结束就写库: count=%d err=%v", count, countErr)
		}
		reviewed[video.Info.Bvid] = true
		return videoReview{Pass: true}
	}

	result, err := ensureWatchlist(context.Background(), true, deps)
	if err != nil {
		t.Fatalf("管线执行失败: %v", err)
	}
	if !result.Triggered || result.Candidates != 24 || result.Impressions != 24 {
		t.Fatalf("候选/印象数量不符: %+v", result)
	}
	if result.Approved != 24 || result.Downloaded != 24 || result.Failed != 0 {
		t.Fatalf("审核与下载结果不符: %+v", result)
	}
	if result.BudgetExhausted || result.Skipped != 0 {
		t.Fatalf("默认预算内不应提前结束: %+v", result)
	}
	if reviewed[seenBV] {
		t.Fatalf("已看过的视频不应进入审核")
	}
	// pending 表示「已入队但没写成可播放」：必须视为可重试，否则该行会永久静默丢失。
	if !reviewed[pendingBV] {
		t.Fatalf("pending 记录应重新进入审核并被推到可播放")
	}
	if len(impressions) != 24 {
		t.Fatalf("抽取印象数量=%d", len(impressions))
	}
	first := impressions[0]
	if first.Info.Title == "" || first.Info.Desc == "" || first.Info.Owner == "" ||
		first.Info.OwnerMid <= 0 || len(first.Comments) == 0 {
		t.Fatalf("视频印象结构不完整: %+v comments=%d", first.Info, len(first.Comments))
	}
	for bv := range reviewed {
		assertWatchSegments(t, bv, 1, viewerstore.WatchSegmentMax)
	}
}

// TestEnsureWatchlistPersistsImpressionTopics 印象主题词随入队写入 watch_video；未看完不入偏好池。
func TestEnsureWatchlistPersistsImpressionTopics(t *testing.T) {
	isolateViewerDB(t)
	session := &fakeWatchSession{links: watchTestLinks(30)}
	deps := newWatchlistTestDeps(session, &fakeWatchDownloader{})
	deps.want = 1
	deps.impress = func(_ context.Context, _ *videoRecord) videoImpression {
		return videoImpression{Summary: "摘要", Topics: []string{"主题甲", "主题乙"}}
	}
	result, err := ensureWatchlist(context.Background(), true, deps)
	if err != nil {
		t.Fatalf("管线执行失败: %v", err)
	}
	if result.Enqueued != 1 {
		t.Fatalf("应入队 1 条，实际 %+v", result)
	}
	approved, err := viewerstore.LoadApprovedVideos()
	if err != nil || len(approved) != 1 {
		t.Fatalf("读取过审视频失败 len=%d err=%v", len(approved), err)
	}
	if strings.Join(approved[0].Topics, ",") != "主题甲,主题乙" {
		t.Fatalf("印象主题词未随入队落库: %v", approved[0].Topics)
	}
	// 入队 ≠ 看过：偏好看板在看完之前保持为空。
	if topics, err := activeWatchedTopics(watchDynamicKeywordLimit, time.Now().Unix()); err != nil || len(topics) != 0 {
		t.Fatalf("未看完不应产生动态偏好，实际 %v err=%v", topics, err)
	}
}

// seedWatchVideo 在待看列表里预置一条记录。
func seedWatchVideo(t *testing.T, bv string, status viewerstore.WatchStatus) {
	t.Helper()
	if _, err := viewerstore.EnqueueVideos([]viewerstore.WatchCandidate{{BV: bv, Title: "种子", DurationSec: 600}}); err != nil {
		t.Fatalf("准备待看记录失败: %v", err)
	}
	if err := viewerstore.SetVideoReview(bv, status, ""); err != nil {
		t.Fatalf("设置审核状态失败: %v", err)
	}
}

// assertWatchSegments 校验片段数量、非重叠与下载状态。
func assertWatchSegments(t *testing.T, bv string, minCount, maxCount int) {
	t.Helper()
	segments, err := viewerstore.LoadSegments(bv)
	if err != nil {
		t.Fatalf("读取片段失败 %s: %v", bv, err)
	}
	if len(segments) < minCount || len(segments) > maxCount {
		t.Fatalf("%s 片段数量=%d，期望 %d~%d", bv, len(segments), minCount, maxCount)
	}
	for index, segment := range segments {
		if !segment.Downloaded || segment.Path == "" {
			t.Fatalf("%s 片段 %d 未下载: %+v", bv, index, segment)
		}
		if segment.EndSec <= segment.StartSec {
			t.Fatalf("%s 片段 %d 区间非法: %+v", bv, index, segment)
		}
		if index > 0 && segment.StartSec < segments[index-1].EndSec {
			t.Fatalf("%s 片段 %d 与前一段重叠", bv, index)
		}
	}
}
