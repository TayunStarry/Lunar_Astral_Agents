package viewer

// watchlist_type.go 待看列表补齐管线的类型与依赖端口。
//
// 管线只做调度：浏览器、模型智能体、存储与下载全部通过接口注入，
// 生产由 defaultWatchlistDeps 绑定真实实现，测试换成假实现即可离线跑。

import (
	"context"
	"math/rand"
	"time"

	"LunarSubsystem/LunarHarness/engine/bilibili"
	"LunarSubsystem/LunarHarness/engine/viewerstore"
)

const (
	// watchlistWanted 触发条件满足时每轮筛出的候选视频数。
	watchlistWanted = 24
	// watchlistCommentLimit 每个候选抽取的评论条数。
	watchlistCommentLimit = 20
	// watchlistDownloadFailPrefix 片段下载失败写进 reject_reason 的前缀。
	// 带此前缀的 rejected 视频不会被当成永久历史，下一轮仍可作为候选重试。
	watchlistDownloadFailPrefix = "片段下载失败："
)

// WatchlistResult 一次补齐管线的对账结果。
type WatchlistResult struct {
	// Triggered 四项触发条件是否全部满足；false 表示本轮没有执行任何动作。
	Triggered bool `json:"triggered"`
	// Reason 未执行或跳过时的可读原因。
	Reason string `json:"reason,omitempty"`
	// Candidates 筛出的候选数（目标 watchlistWanted）。
	Candidates int `json:"candidates,omitempty"`
	// Impressions 成功抽取视频印象的数量。
	Impressions int `json:"impressions,omitempty"`
	// Approved / Rejected 审核通过数与未通过数。
	Approved int `json:"approved,omitempty"`
	Rejected int `json:"rejected,omitempty"`
	// Enqueued 新写入待看列表的行数（同 BV 已存在时不计）。
	Enqueued int `json:"enqueued,omitempty"`
	// Downloaded 成功切出片段并置为可播放的视频数。
	Downloaded int `json:"downloaded,omitempty"`
	// Failed 单条失败数（采集、审核或下载失败），不影响其他视频。
	Failed int `json:"failed,omitempty"`
	// Failures 失败原因摘要，便于日志定位。
	Failures []string `json:"failures,omitempty"`
	// BudgetExhausted 是否因整阶段时间预算耗尽而提前结束（已完成的成果仍然保留）。
	BudgetExhausted bool `json:"budget_exhausted,omitempty"`
	// Skipped 因预算耗尽而未完成的候选数（采集/审核/下载任一阶段放弃的都计入，且只计一次）。
	Skipped int `json:"skipped,omitempty"`
}

// watchItem 管线内一个候选视频的采集、印象、审核与切片结果。
type watchItem struct {
	record       *videoRecord
	approved     bool
	rejectReason string
	// slices 审核给出的切片数，下载阶段直接采用（钳制到 1~5）。
	slices int
}

// watchlistBudget 整阶段预算：总时长上限 + 单候选子预算。
//
// start 由 newWatchlistBudget 在阶段开始时设定；now 可注入，测试用它让预算耗尽时机可复现。
type watchlistBudget struct {
	total   time.Duration
	perItem time.Duration
	now     func() time.Time
	start   time.Time
}

// commentReader 评论读取能力（*webSession 实现）。
//
// 接口分页在 engine/bilibili；这里只保留页面 DOM 兜底读取。
type commentReader interface {
	bilibili.Fetcher
	readComments(max int) ([]commentItem, error)
}

// watchSession 抓取管线需要的浏览器能力，真实实现为 *webSession。
type watchSession interface {
	commentReader
	ensureLogin() error
	collectVideoLinks(max int) ([]videoLink, error)
	navigate(target string, timeout time.Duration) error
	wait(d time.Duration)
	close()
}

// watchDownloader 把一个视频的若干时间段下载成本地文件。
//
// 返回的片段必须带本地路径并标记 Downloaded；任何一段失败都返回错误，
// 由调用方把整个视频置为可重试状态，避免留下半套片段。
type watchDownloader interface {
	Download(ctx context.Context, info videoInfo, segments []viewerstore.SegmentInput) ([]viewerstore.SegmentInput, error)
}

// watchlistDeps 管线的全部外部依赖。
type watchlistDeps struct {
	// want 目标候选数（测试可调小）。
	want int
	// open 打开 Bilibili 页面。
	open            func() (watchSession, error)
	watchlistEmpty  func() (bool, error)
	loadVideo       func(bv string) (viewerstore.WatchVideo, bool, error)
	fetchInfo       func(bv string) (videoInfo, error)
	impress         func(ctx context.Context, video *videoRecord) videoImpression
	review          func(ctx context.Context, video *videoRecord) videoReview
	enqueue         func(items []viewerstore.WatchCandidate) (viewerstore.EnqueueReport, error)
	setReview       func(bv string, status viewerstore.WatchStatus, reason string) error
	replaceSegments func(bv string, segments []viewerstore.SegmentInput) error
	downloader      watchDownloader
	randInt         func(n int) int
	// budget 整阶段预算；total/perItem 非正数时由 newWatchlistBudget 取默认值。
	budget watchlistBudget
	// now 预算时钟（测试注入假时钟使耗尽时机可复现）。
	now func() time.Time
}

// defaultWatchlistDeps 绑定生产实现：浏览器、观影智能体、viewerstore 与 ffmpeg 下载。
func defaultWatchlistDeps() watchlistDeps {
	cfg := LoadConfig()
	return watchlistDeps{
		want: watchlistWanted,
		open: func() (watchSession, error) {
			session, err := openBilibiliBrowser()
			if err != nil {
				return nil, err
			}
			return session, nil
		},
		watchlistEmpty:  viewerstore.WatchlistEmpty,
		loadVideo:       viewerstore.LoadVideo,
		fetchInfo:       bilibili.FetchVideo,
		impress:         func(_ context.Context, video *videoRecord) videoImpression { return buildOneImpression(video) },
		review:          func(_ context.Context, video *videoRecord) videoReview { return reviewVideo(video) },
		enqueue:         viewerstore.EnqueueVideos,
		setReview:       viewerstore.SetVideoReview,
		replaceSegments: viewerstore.ReplaceSegments,
		downloader:      ffmpegWatchDownloader{},
		randInt:         rand.Intn,
		budget: watchlistBudget{
			total:   time.Duration(cfg.WatchlistBudgetSec) * time.Second,
			perItem: time.Duration(cfg.WatchlistCandidateBudgetSec) * time.Second,
		},
		now: time.Now,
	}
}
