package viewer

// watch_branch_deps.go 看视频分支的外部依赖与生产实现。
//
// 默认实现直接落到引擎与智能体；单元测试替换为确定性桩，
// 不访问 B 站、不跑浏览器、不调真实模型。

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/agents/Vision"
	"LunarSubsystem/LunarHarness/agents/viewer/ViewerRemark"
	"LunarSubsystem/LunarHarness/agents/viewer/ViewerScreening"
	"LunarSubsystem/LunarHarness/engine/activity"
	"LunarSubsystem/LunarHarness/engine/bilibili"
	module "LunarSubsystem/LunarHarness/engine/multimodal"
	"LunarSubsystem/LunarHarness/engine/viewerstore"
)

// watchDeps 看视频分支的依赖集合。
type watchDeps struct {
	pause        func() bool
	now          func() time.Time
	nextSegment  func() (viewerstore.WatchSegment, bool, error)
	loadVideo    func(string) (viewerstore.WatchVideo, bool, error)
	loadSegments func(string) ([]viewerstore.WatchSegment, error)
	// understand 解码本地片段并做一次视觉理解，返回理解文本与降级说明。
	understand func(context.Context, viewerstore.WatchSegment, viewerstore.WatchVideo) (string, string, error)
	// media 补取播放信息（aid / 时长 / 实时弹幕）；失败通过 Note 降级，不阻断观看。
	media func(viewerstore.WatchVideo) watchMedia
	// impression 结合画面理解与采集材料合成「视频印象」，返回印象文本与降级说明。
	impression func(context.Context, viewerstore.WatchVideo, watchMedia, string) (string, string)
	// remark 基于视频印象生成 1-2 条评论，每条各自带 @ 人数与可选名字/序号。
	remark func(context.Context, viewerstore.WatchVideo, string, []ViewerRemark.MentionCandidate) (ViewerRemark.Remark, error)
	// openMention 打开一次评论会话：先读特别关注候选，再用同一会话发布评论。
	openMention func() (mentionSession, error)
	markWatched func(string, int, int64) (viewerstore.SegmentWatchResult, error)
	// removeClip 删除已看完片段的本地文件并清空存储路径；失败只影响清理，不影响观看流程。
	removeClip     func(viewerstore.WatchSegment) error
	countCompleted func() (int, error)
	setActivity    func(string)
	milestone      func(context.Context, int) (bool, error)
}

// defaultWatchDeps 生产依赖。
func defaultWatchDeps() watchDeps {
	return watchDeps{
		pause:        pauseRequested,
		now:          time.Now,
		nextSegment:  viewerstore.NextUnwatchedSegment,
		loadVideo:    viewerstore.LoadVideo,
		loadSegments: viewerstore.LoadSegments,
		understand:   understandClip,
		media:        loadWatchMedia,
		impression:   buildWatchImpression,
		remark:       remarkOnVideo,
		openMention:  openMentionSession,
		markWatched: func(bv string, index int, at int64) (viewerstore.SegmentWatchResult, error) {
			// 完成回调在存储侧的标记事务内执行，偏好策略（watch_topic.go）与完成状态同事务落盘。
			return viewerstore.MarkSegmentWatched(bv, index, at, bumpWatchedTopics)
		},
		removeClip:     discardWatchedClip,
		countCompleted: viewerstore.CountCompletedVideos,
		setActivity:    activity.Set,
		milestone:      postStageDynamic,
	}
}

// discardWatchedClip 删除已看完片段的本地文件，并清空存储里的路径。
//
// 先删文件再清路径：删除失败时记录仍指向存在的文件，不会留下「记录指向已删文件」的脏路径。
func discardWatchedClip(seg viewerstore.WatchSegment) error {
	if path := strings.TrimSpace(seg.Path); path != "" {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("删除本地视频文件失败: %w", err)
		}
	}
	return viewerstore.ClearSegmentPath(seg.BV, seg.Index)
}

// understandClip 理解一个已下载片段：按片段自身时长抽帧，再交给多模态智能体。
// 一个视频只抽一个随机位置的 1 分钟片段，因此时间区间固定从 0 开始。
func understandClip(ctx context.Context, seg viewerstore.WatchSegment, video viewerstore.WatchVideo) (string, string, error) {
	path := strings.TrimSpace(seg.Path)
	if path == "" {
		return "", "", fmt.Errorf("视频没有本地文件")
	}
	duration, err := module.GetVideoDuration(path)
	if err != nil {
		return "", "", fmt.Errorf("读取本地视频时长失败: %w", err)
	}
	if duration <= 0 {
		return "", "", fmt.Errorf("本地视频时长为 0")
	}
	images, err := module.SegmentImages(path, module.MediaSegment{Start: 0, End: duration}, module.WatchFrameAuto)
	if err != nil {
		return "", "", fmt.Errorf("抽帧失败: %w", err)
	}
	scene := fmt.Sprintf("你正在看《%s》，UP主：%s。这是从视频里随机抽取的一分钟片段。", video.Title, video.AuthorName)
	slice := module.WatchSlice{Index: 1, Start: 0, End: duration, Images: images}
	task := Vision.WatchTask(1, 1, slice, scene)
	text, err := ViewerScreening.Slice(ctx, task, modelPrimary, images)
	if err != nil {
		return "", "", err
	}
	return cleanMemoryText(text), "", nil
}

// loadWatchMedia 读取评论发布所需的 aid 与这一分钟的实时弹幕。
// 读取失败只降级为无弹幕，评论发布时会再尝试一次。
func loadWatchMedia(video viewerstore.WatchVideo) watchMedia {
	info, err := bilibili.FetchVideo(video.BV)
	if err != nil {
		return watchMedia{Note: fmt.Sprintf("读取视频播放信息失败，本次没有弹幕：%v", err)}
	}
	media := watchMedia{Aid: info.Aid}
	if info.Cid > 0 {
		media.Danmaku = bilibili.FetchDanmaku(info.Cid, watchImpressionDanmakuRows)
	}
	return media
}

// buildWatchImpression 把采集材料与这一分钟的画面理解排版后，交给阅片智能体合成视频印象。
// 印象模型不可用时退回排版材料并说明，保证评论仍可基于材料生成。
func buildWatchImpression(ctx context.Context, video viewerstore.WatchVideo, media watchMedia, visual string) (string, string) {
	record := watchRecord(video, media.Danmaku)
	material := impressionMaterial(record, visual)
	if strings.TrimSpace(material) == "" {
		return "", "视频印象材料为空"
	}
	impression, err := ViewerScreening.Impress(ctx, material, modelLight)
	if err != nil {
		return material, "视频印象合成失败，改用整理材料：" + err.Error()
	}
	text := material
	if summary := strings.TrimSpace(impression.Summary); summary != "" {
		text += "\n\n整体印象：" + summary
	}
	if tone := strings.TrimSpace(impression.Tone); tone != "" {
		text += "\n氛围：" + tone
	}
	return text, ""
}

// watchRecord 把存储里的视频信息还原成印象材料所需的采集结构。
func watchRecord(video viewerstore.WatchVideo, danmaku []string) *videoRecord {
	comments := make([]commentItem, 0, len(video.Comments))
	for _, text := range video.Comments {
		if text = strings.TrimSpace(text); text != "" {
			comments = append(comments, commentItem{Content: text})
		}
	}
	return &videoRecord{
		Info: videoInfo{
			Bvid:     video.BV,
			Title:    video.Title,
			Desc:     video.Intro,
			Owner:    video.AuthorName,
			Duration: video.DurationSec,
		},
		Tags:     video.Topics,
		Comments: comments,
		Danmaku:  danmaku,
	}
}

// remarkOnVideo 基于视频印象生成 1-2 条评论，并把特别关注候选交给模型（每条评论各自决定 @）。
func remarkOnVideo(ctx context.Context, _ viewerstore.WatchVideo, material string, candidates []ViewerRemark.MentionCandidate) (ViewerRemark.Remark, error) {
	return ViewerRemark.Generate(ctx, ViewerRemark.RemarkInput{
		Material: material, Candidates: candidates, Model: modelPrimary,
	})
}

// toMentionCandidates 把候选转成「序号 + 展示名」，内部 UID 不进入提示词。
func toMentionCandidates(pool []MentionTarget) []ViewerRemark.MentionCandidate {
	out := make([]ViewerRemark.MentionCandidate, 0, len(pool))
	for index, target := range pool {
		out = append(out, ViewerRemark.MentionCandidate{Index: index + 1, Name: target.Name})
	}
	return out
}
