package viewer

// watch_branch_deps.go 看视频分支的外部依赖与生产实现。
//
// 默认实现直接落到引擎与智能体；单元测试替换为确定性桩，
// 不访问 B 站、不跑浏览器、不调真实模型。

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/agents/ViewerRemark"
	"LunarSubsystem/LunarHarness/agents/ViewerScreening"
	"LunarSubsystem/LunarHarness/engine/activity"
	module "LunarSubsystem/LunarHarness/engine/multimodal"
	"LunarSubsystem/LunarHarness/engine/viewerstore"
	"LunarSubsystem/LunarHarness/prompts"
)

// watchDeps 看视频分支的依赖集合。
type watchDeps struct {
	pause        func() bool
	now          func() time.Time
	nextSegment  func() (viewerstore.WatchSegment, bool, error)
	loadVideo    func(string) (viewerstore.WatchVideo, bool, error)
	loadSegments func(string) ([]viewerstore.WatchSegment, error)
	// understand 解码本地片段并做视觉理解，返回理解文本与降级说明。
	understand func(context.Context, viewerstore.WatchSegment, viewerstore.WatchVideo, int) (string, string, error)
	remark     func(context.Context, viewerstore.WatchVideo, string) (string, error)
	// postComment 把评论发到原视频评论区，返回（是否发布、是否 @ 了作者、错误）。
	postComment func(viewerstore.WatchVideo, string, bool) (bool, bool, error)
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
		pause:          pauseRequested,
		now:            time.Now,
		nextSegment:    viewerstore.NextUnwatchedSegment,
		loadVideo:      viewerstore.LoadVideo,
		loadSegments:   viewerstore.LoadSegments,
		understand:     understandClip,
		remark:         remarkOnVideo,
		postComment:    postVideoComment,
		markWatched:    viewerstore.MarkSegmentWatched,
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
			return fmt.Errorf("删除本地片段失败: %w", err)
		}
	}
	return viewerstore.ClearSegmentPath(seg.BV, seg.Index)
}

// understandClip 播放一个已下载片段：按片段自身时长抽帧，再交给多模态智能体。
// 下载时每个片段都是独立文件，因此时间区间固定从 0 开始。
func understandClip(ctx context.Context, seg viewerstore.WatchSegment, video viewerstore.WatchVideo, total int) (string, string, error) {
	path := strings.TrimSpace(seg.Path)
	if path == "" {
		return "", "", fmt.Errorf("片段没有本地文件")
	}
	duration, err := module.GetVideoDuration(path)
	if err != nil {
		return "", "", fmt.Errorf("读取本地片段时长失败: %w", err)
	}
	if duration <= 0 {
		return "", "", fmt.Errorf("本地片段时长为 0")
	}
	images, err := module.SegmentImages(path, module.MediaSegment{Start: 0, End: duration}, module.WatchFrameAuto)
	if err != nil {
		return "", "", fmt.Errorf("抽帧失败: %w", err)
	}
	persona, err := prompts.Read("lifeRole.md")
	if err != nil {
		return "", "", fmt.Errorf("读取人设提示词失败: %w", err)
	}
	scene := fmt.Sprintf("你正在看《%s》，UP主：%s。这是第 %d/%d 个片段。", video.Title, video.AuthorName, seg.Index+1, total)
	slice := module.WatchSlice{Index: seg.Index + 1, Start: 0, End: duration, Images: images}
	task := module.WatchTask(seg.Index+1, total, slice, scene)
	text, err := ViewerScreening.Slice(ctx, string(persona), task, modelPrimary, images)
	if err != nil {
		return "", "", err
	}
	return cleanMemoryText(text), "", nil
}

// remarkOnVideo 基于画面理解与视频印象写一条观感评论。
func remarkOnVideo(ctx context.Context, _ viewerstore.WatchVideo, material string) (string, error) {
	persona, err := prompts.Read("lifeRole.md")
	if err != nil {
		return "", fmt.Errorf("读取人设提示词失败: %w", err)
	}
	return ViewerRemark.Remark(ctx, ViewerRemark.RemarkInput{
		Persona: string(persona), Material: material, Model: modelPrimary,
	})
}

// postVideoComment 打开 B 站并发布视频评论；最后一段按需 @ 原作者。
func postVideoComment(video viewerstore.WatchVideo, message string, mentionAuthor bool) (bool, bool, error) {
	session, err := openBilibiliBrowser()
	if err != nil {
		return false, false, err
	}
	defer session.close()
	if err := session.ensureLogin(); err != nil {
		return false, false, err
	}
	info, err := fetchVideoInfo(video.BV)
	if err != nil {
		return false, false, err
	}
	if info.Aid <= 0 {
		return false, false, fmt.Errorf("视频 aid 缺失，无法发评论")
	}
	targets, mentioned := authorTargets(video, info, mentionAuthor)
	if err := session.postCommentOnObject(1, strconv.FormatInt(info.Aid, 10), message, targets); err != nil {
		return false, mentioned, err
	}
	return true, mentioned, nil
}

// authorTargets 解析 @ 原作者的目标；昵称或 UID 缺失时不加 @，避免无效字段。
func authorTargets(video viewerstore.WatchVideo, info videoInfo, mentionAuthor bool) ([]MentionTarget, bool) {
	if !mentionAuthor {
		return nil, false
	}
	name := strings.TrimSpace(video.AuthorName)
	uid, _ := strconv.ParseInt(strings.TrimSpace(video.AuthorUID), 10, 64)
	if name == "" {
		name = strings.TrimSpace(info.Owner)
	}
	if uid <= 0 {
		uid = info.OwnerMid
	}
	if name == "" || uid <= 0 {
		return nil, false
	}
	return []MentionTarget{{Name: name, UID: uid}}, true
}
