package viewer

// watchlist_download.go 过审视频的入队与切片下载。
//
// 入队先于下载：审核通过的视频先以 pending 落库，只有片段真正下载成功并写入
// watch_segment 后才置 approved。任何一步失败都会把该行置为 rejected 并写明原因，
// 既不会让待看列表被「审核通过但没切出片段」卡死，也保留了下一轮重试的依据。

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"LunarSubsystem/LunarHarness/engine/bilibili"
	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/viewerstore"
)

const (
	// watchlistSegmentMinSec / MaxSec 单个随机片段的时长范围（秒）。
	watchlistSegmentMinSec = 30
	watchlistSegmentMaxSec = 120
	// watchlistCacheDirName 片段落盘目录（相对 local_data/cache）。
	watchlistCacheDirName = "watchlist"
)

// persistWatchItems 把审核通过的视频以 pending 状态批量入队（BV 幂等）。
func persistWatchItems(items []*watchItem, deps watchlistDeps) (int, error) {
	candidates := make([]viewerstore.WatchCandidate, 0, len(items))
	for _, item := range items {
		if item == nil || !item.approved || item.record == nil {
			continue
		}
		record := item.record
		candidates = append(candidates, viewerstore.WatchCandidate{
			BV:          record.Info.Bvid,
			Title:       record.Info.Title,
			Intro:       record.Info.Desc,
			AuthorUID:   authorUID(record.Info),
			AuthorName:  record.Info.Owner,
			Comments:    commentTexts(record.Comments),
			DurationSec: record.Info.Duration,
			Topics:      record.Impression.Topics,
		})
	}
	if len(candidates) == 0 {
		return 0, nil
	}
	report, err := deps.enqueue(candidates)
	if err != nil {
		return 0, err
	}
	logger.Info("BilibiliViewer", "待看列表：过审入队 %d 条（已存在跳过 %d 条）", report.Inserted, report.Skipped)
	return report.Inserted, nil
}

// downloadWatchItems 逐个视频切片段并下载；单条失败只记录，不影响其他视频。
func downloadWatchItems(ctx context.Context, items []*watchItem, deps watchlistDeps, result *WatchlistResult, budget watchlistBudget) {
	for _, item := range items {
		if item == nil || !item.approved || item.record == nil {
			continue
		}
		if err := ctx.Err(); err != nil {
			return
		}
		bv := item.record.Info.Bvid
		if budget.expired() {
			deferWatchByBudget(bv, deps, result)
			continue
		}
		segments := planWatchSegments(item.record.Info.Duration, watchSegmentCount(item, deps), deps.randInt)
		if len(segments) == 0 {
			markWatchDownloadFailed(bv, "视频时长未知，无法切片", deps, result)
			continue
		}
		downloadCtx, cancel := budget.itemContext(ctx)
		downloaded, err := deps.downloader.Download(downloadCtx, item.record.Info, segments)
		cancel()
		if err != nil {
			markWatchDownloadFailed(bv, err.Error(), deps, result)
			continue
		}
		if !downloadComplete(downloaded) {
			markWatchDownloadFailed(bv, "下载结果缺少本地文件", deps, result)
			continue
		}
		if err := deps.replaceSegments(bv, downloaded); err != nil {
			markWatchDownloadFailed(bv, "片段记录写入失败: "+err.Error(), deps, result)
			continue
		}
		if err := deps.setReview(bv, viewerstore.WatchStatusApproved, ""); err != nil {
			// 片段已落库但状态没写成：回退为 rejected + 原因，让下一轮重新下载覆盖；
			// 万一这次也写失败，该行仍是 pending，同样会被 retryableWatchVideo 救回。
			markWatchDownloadFailed(bv, "片段已下载但置为可播放失败: "+err.Error(), deps, result)
			continue
		}
		result.Downloaded++
		logger.Info("BilibiliViewer", "待看列表：《%s》切出 %d 个片段并置为可播放",
			item.record.Info.Title, len(downloaded))
	}
}

// watchSegmentCount 以审核给出的切片数为准（钳制到 1~5，且不超过视频可用片段数）；
// 审核没给数时才退回随机 1~5。
func watchSegmentCount(item *watchItem, deps watchlistDeps) int {
	count := item.slices
	if count <= 0 {
		count = 1 + deps.randInt(watchSliceMax)
	}
	return clampReviewSlices(item.record.Info.Duration, count)
}

// deferWatchByBudget 预算耗尽时把已入队但还没下载的视频置为可重试的 rejected 并写明原因，
// 既不留「pending 且无片段」的半条记录，也不让待看列表被卡住。
func deferWatchByBudget(bv string, deps watchlistDeps, result *WatchlistResult) {
	result.BudgetExhausted = true
	result.Skipped++
	reason := watchlistDownloadFailPrefix + "本轮时间预算耗尽，未下载"
	if err := deps.setReview(bv, viewerstore.WatchStatusRejected, reason); err != nil {
		logger.Warn("BilibiliViewer", "待看列表：%s 预算顺延原因写入失败: %v", bv, err)
	}
	logger.Warn("BilibiliViewer", "待看列表：%s 因时间预算耗尽顺延到下一轮", bv)
}

// markWatchDownloadFailed 保留记录并写明原因，使下一轮可以重试。
func markWatchDownloadFailed(bv, reason string, deps watchlistDeps, result *WatchlistResult) {
	text := watchlistDownloadFailPrefix + strings.TrimSpace(reason)
	if err := deps.setReview(bv, viewerstore.WatchStatusRejected, text); err != nil {
		logger.Warn("BilibiliViewer", "待看列表：%s 下载失败原因写入失败: %v", bv, err)
	}
	logger.Warn("BilibiliViewer", "待看列表：%s 切片下载失败，保留记录待下轮重试: %s", bv, text)
	result.Failed++
	result.Failures = append(result.Failures, bv+" "+text)
}

// downloadComplete 校验下载器是否真的产出了可播放文件。
func downloadComplete(segments []viewerstore.SegmentInput) bool {
	if len(segments) == 0 {
		return false
	}
	for _, segment := range segments {
		if !segment.Downloaded || strings.TrimSpace(segment.Path) == "" {
			return false
		}
	}
	return true
}

// planWatchSegments 按总时长随机生成 count 个互不重叠的片段。
//
// 做法：把时间轴均分为 count 个带，每带内随机放一个 30~120 秒的窗口。
// 带宽天然大于等于窗口长度，因此片段按构造就不会重叠，也不会越过视频结尾。
func planWatchSegments(duration, count int, randInt func(int) int) []viewerstore.SegmentInput {
	if duration <= 0 || count <= 0 {
		return nil
	}
	if randInt == nil {
		randInt = rand.Intn
	}
	if maxByMin := duration / watchlistSegmentMinSec; count > maxByMin {
		count = maxByMin
	}
	if count > viewerstore.WatchSegmentMax {
		count = viewerstore.WatchSegmentMax
	}
	if count < 1 {
		count = 1
	}
	band := duration / count
	if band <= 0 {
		return nil
	}
	length := band
	if length > watchlistSegmentMaxSec {
		span := watchlistSegmentMaxSec - watchlistSegmentMinSec + 1
		length = watchlistSegmentMinSec + randInt(span)
	}
	if length > band {
		length = band
	}
	out := make([]viewerstore.SegmentInput, 0, count)
	for index := 0; index < count; index++ {
		start := index * band
		if slack := band - length; slack > 0 {
			start += randInt(slack + 1)
		}
		out = append(out, viewerstore.SegmentInput{StartSec: start, EndSec: start + length})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].StartSec < out[j].StartSec })
	return out
}

// authorUID 作者 UID 转字符串（缺失时留空，由存储层容忍）。
func authorUID(info videoInfo) string {
	if info.OwnerMid <= 0 {
		return ""
	}
	return strconv.FormatInt(info.OwnerMid, 10)
}

// commentTexts 评论转成「用户：内容」的入库格式。
func commentTexts(comments []commentItem) []string {
	out := make([]string, 0, len(comments))
	for _, comment := range comments {
		text := strings.TrimSpace(comment.User + "：" + comment.Content)
		if text != "" {
			out = append(out, text)
		}
	}
	return out
}

// ffmpegWatchDownloader 生产下载器：取播放地址后用 ffmpeg 按时间段切片下载。
type ffmpegWatchDownloader struct{}

// Download 依次下载每个片段；任一段失败会清理本次已生成的文件并返回错误。
func (ffmpegWatchDownloader) Download(ctx context.Context, info videoInfo, segments []viewerstore.SegmentInput) ([]viewerstore.SegmentInput, error) {
	if strings.TrimSpace(info.Bvid) == "" || info.Cid <= 0 {
		return nil, fmt.Errorf("缺少 BV 号或 Cid，无法获取播放地址")
	}
	playURL, err := bilibili.PlayURL(info.Bvid, info.Cid, 32)
	if err != nil {
		return nil, fmt.Errorf("获取播放地址失败: %w", err)
	}
	dir := filepath.Join(config.LocalDir, "cache", watchlistCacheDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("创建片段目录失败: %w", err)
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, fmt.Errorf("找不到 ffmpeg: %w", err)
	}
	out := make([]viewerstore.SegmentInput, 0, len(segments))
	created := make([]string, 0, len(segments))
	for index, segment := range segments {
		length := segment.EndSec - segment.StartSec
		if length <= 0 {
			return nil, fmt.Errorf("片段 %d 时长不合法", index+1)
		}
		path := filepath.Join(dir, fmt.Sprintf("bili_watch_%s_%02d_%d_%d.mp4",
			info.Bvid, index, segment.StartSec, segment.EndSec))
		args := []string{
			"-y", "-hide_banner", "-loglevel", "error",
			"-ss", strconv.Itoa(segment.StartSec),
			"-headers", "Referer: " + bilibili.Referer + "\r\nUser-Agent: " + bilibili.UserAgent + "\r\n",
			"-i", playURL,
			"-t", strconv.Itoa(length),
			"-c", "copy", "-avoid_negative_ts", "make_zero", path,
		}
		if data, runErr := exec.CommandContext(ctx, ffmpeg, args...).CombinedOutput(); runErr != nil {
			for _, stale := range created {
				_ = os.Remove(stale)
			}
			return nil, fmt.Errorf("片段 %d 下载失败: %v %s", index+1, runErr, truncate(string(data), 512))
		}
		if stat, statErr := os.Stat(path); statErr != nil || stat.Size() == 0 {
			for _, stale := range created {
				_ = os.Remove(stale)
			}
			_ = os.Remove(path)
			return nil, fmt.Errorf("片段 %d 下载文件为空", index+1)
		}
		created = append(created, path)
		out = append(out, viewerstore.SegmentInput{
			StartSec: segment.StartSec, EndSec: segment.EndSec, Path: path, Downloaded: true,
		})
	}
	return out, nil
}
