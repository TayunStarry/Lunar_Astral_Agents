package viewer

// watch_branch_test.go 看视频分支：单切片理解的执行顺序、1-2 条评论、@ 人数取舍、
// 降级路径、状态更新与 8 视频里程碑去重。

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"LunarSubsystem/LunarHarness/agents/viewer/ViewerRemark"
	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/viewerstore"
)

// newWatchTestDeps 构造全部外设都走桩的依赖，记录调用顺序与参数。
func newWatchTestDeps(t *testing.T) (watchDeps, *watchTestLog) {
	t.Helper()
	log := &watchTestLog{
		removedIndex:   -1,
		impressionText: "《测试视频》的整体印象：轻松日常",
		comments:       []string{"橘猫那一下太可爱了"},
	}
	deps := watchDeps{
		pause: func() bool { return false },
		now:   func() time.Time { return time.Unix(1_800_000_000, 0) },
		nextSegment: func() (viewerstore.WatchSegment, bool, error) {
			log.order = append(log.order, "next")
			return viewerstore.WatchSegment{BV: "BV1test", Index: log.segmentIndex, Path: "clip.mp4"}, true, nil
		},
		loadVideo: func(string) (viewerstore.WatchVideo, bool, error) {
			return viewerstore.WatchVideo{
				BV: "BV1test", Title: "测试视频", AuthorName: "某UP", AuthorUID: "12345",
				Intro: "这是一个简介", Comments: []string{"好看"}, DurationSec: 600,
				Topics: []string{"测试"},
			}, true, nil
		},
		loadSegments: func(string) ([]viewerstore.WatchSegment, error) {
			out := make([]viewerstore.WatchSegment, 0, log.segmentTotal)
			for i := 0; i < log.segmentTotal; i++ {
				out = append(out, viewerstore.WatchSegment{BV: "BV1test", Index: i})
			}
			return out, nil
		},
		understand: func(_ context.Context, _ viewerstore.WatchSegment, _ viewerstore.WatchVideo) (string, string, error) {
			log.order = append(log.order, "understand")
			if log.understandErr != nil {
				return "", "", log.understandErr
			}
			return "画面里一只橘猫趴在窗台上", "", nil
		},
		media: func(_ viewerstore.WatchVideo) watchMedia {
			log.order = append(log.order, "media")
			return watchMedia{Aid: 100, Danmaku: []string{"这段好治愈"}}
		},
		impression: func(_ context.Context, _ viewerstore.WatchVideo, _ watchMedia, visual string) (string, string) {
			log.order = append(log.order, "impression")
			log.visual = visual
			if log.impressionEmpty {
				return "", "视频印象材料为空"
			}
			return log.impressionText, ""
		},
		remark: func(_ context.Context, _ viewerstore.WatchVideo, material string, candidates []ViewerRemark.MentionCandidate) (ViewerRemark.Remark, error) {
			log.order = append(log.order, "remark")
			log.material = material
			log.candidatesSeen = candidates
			if log.remarkErr != nil {
				return ViewerRemark.Remark{}, log.remarkErr
			}
			if log.plan != nil {
				return ViewerRemark.Remark{Comments: log.plan}, nil
			}
			comments := make([]ViewerRemark.RemarkComment, 0, len(log.comments))
			for _, text := range log.comments {
				comments = append(comments, ViewerRemark.RemarkComment{
					Text: text, Mention: log.mention, At: log.at,
				})
			}
			return ViewerRemark.Remark{Comments: comments}, nil
		},
		openMention: func() (mentionSession, error) {
			log.order = append(log.order, "open")
			if log.openMentionErr != nil {
				return nil, log.openMentionErr
			}
			return &fakeMentionSession{log: log}, nil
		},
		markWatched: func(_ string, index int, _ int64) (viewerstore.SegmentWatchResult, error) {
			log.order = append(log.order, "mark")
			log.markIndex = index
			if log.markErr != nil {
				return viewerstore.SegmentWatchResult{}, log.markErr
			}
			return viewerstore.SegmentWatchResult{VideoCompleted: true, CompletedVideos: log.completedVideos}, nil
		},
		removeClip: func(seg viewerstore.WatchSegment) error {
			log.order = append(log.order, "remove")
			log.removedIndex = seg.Index
			return log.removeErr
		},
		countCompleted: func() (int, error) { return log.completedVideos, nil },
		setActivity: func(text string) {
			log.order = append(log.order, "activity")
			log.activity = text
		},
		milestone: func(_ context.Context, count int) (bool, error) {
			log.milestoneCount = count
			return false, nil
		},
	}
	return deps, log
}

type watchTestLog struct {
	order           []string
	segmentIndex    int
	segmentTotal    int
	completedVideos int
	understandErr   error
	impressionText  string
	impressionEmpty bool
	visual          string
	material        string
	candidates      []MentionTarget
	candidatesSeen  []ViewerRemark.MentionCandidate
	comments        []string
	mention         int
	at              []string
	// plan 非空时直接作为模型输出，用于逐条给不同 @ 决策。
	plan           []ViewerRemark.RemarkComment
	remarkErr      error
	postedComments []mentionComment
	postedTargets  []MentionTarget
	postedAid      int64
	postFailed     bool
	postErr        error
	openMentionErr error
	markIndex      int
	markErr        error
	removedIndex   int
	removeErr      error
	activity       string
	milestoneCount int
}

// fakeMentionSession 假评论会话：候选来自日志，发布结果写回日志。
type fakeMentionSession struct {
	log    *watchTestLog
	closed bool
}

func (s *fakeMentionSession) Candidates() []MentionTarget { return s.log.candidates }

func (s *fakeMentionSession) Post(_ viewerstore.WatchVideo, comments []mentionComment, aid int64) (int, bool, error) {
	s.log.order = append(s.log.order, "post")
	s.log.postedComments = comments
	s.log.postedAid = aid
	targets := make([]MentionTarget, 0)
	mentioned := false
	for _, comment := range comments {
		targets = append(targets, comment.Targets...)
		if len(comment.Targets) > 0 {
			mentioned = true
		}
	}
	s.log.postedTargets = targets
	if s.log.postFailed {
		return 0, false, s.log.postErr
	}
	return len(comments), mentioned, nil
}

func (s *fakeMentionSession) Close() { s.closed = true }

// postedTexts 取出假会话收到的评论正文。
func postedTexts(comments []mentionComment) []string {
	out := make([]string, 0, len(comments))
	for _, comment := range comments {
		out = append(out, comment.Text)
	}
	return out
}

// TestWatchSegmentPublishesCommentsAndUpdatesActivity 单切片 → 一次理解 → 视频印象 → 1-2 条评论。
func TestWatchSegmentPublishesCommentsAndUpdatesActivity(t *testing.T) {
	deps, log := newWatchTestDeps(t)
	log.segmentTotal = 1
	log.comments = []string{"橘猫那一下太可爱了", "窗台的光影真好看"}

	out, err := watchWithDeps(context.Background(), "", deps)
	if err != nil {
		t.Fatal(err)
	}
	if !out.Triggered || out.SegmentIndex != 1 || out.SegmentTotal != 1 || !out.LastSegment {
		t.Fatalf("片段定位不符: %+v", out)
	}
	if out.VisualSource != "frames" || !strings.Contains(log.visual, "橘猫") {
		t.Fatalf("应把这一段的画面理解交给视频印象，实际来源=%s 画面=%q", out.VisualSource, log.visual)
	}
	if !out.CommentPosted || out.Mentioned || out.CommentCount != 2 {
		t.Fatalf("应发布 2 条评论且默认不 @：%+v", out)
	}
	if len(log.postedTargets) != 0 {
		t.Fatalf("模型给 0 时不应 @，实际 %+v", log.postedTargets)
	}
	if log.postedAid != 100 {
		t.Fatalf("发布应带上 aid，实际 %d", log.postedAid)
	}
	if strings.Join(postedTexts(log.postedComments), "|") != "橘猫那一下太可爱了|窗台的光影真好看" {
		t.Fatalf("评论正文未按顺序传达: %v", log.postedComments)
	}
	if !strings.Contains(log.material, "整体印象") {
		t.Fatalf("评论材料应来自视频印象，实际 %q", log.material)
	}
	if log.markIndex != 0 || log.removedIndex != 0 {
		t.Fatalf("应标记并清理同一个片段，mark=%d remove=%d", log.markIndex, log.removedIndex)
	}
	if !strings.Contains(log.activity, "刚看了哔哩哔哩上的《测试视频》") {
		t.Fatalf("看完应立即更新状态，实际 %q", log.activity)
	}
	if strings.Contains(log.activity, "一分钟") || strings.Contains(log.activity, "片段") {
		t.Fatalf("运行状态不应向人格声明只看了一小段，实际 %q", log.activity)
	}
	// 顺序：取片段 → 理解画面 → 补取播放信息 → 合成印象 → 更新状态 → 打开评论会话 → 生成评论 → 发布 → 标记已看 → 清理本地文件。
	want := []string{"next", "understand", "media", "impression", "activity", "open", "remark", "post", "mark", "remove"}
	if strings.Join(log.order, ",") != strings.Join(want, ",") {
		t.Fatalf("执行顺序不符：want %v，实际 %v", want, log.order)
	}
}

// TestWatchClampsCommentsAndMention 评论最多 2 条且去重；@ 人数钳制到 0-3。
func TestWatchClampsCommentsAndMention(t *testing.T) {
	deps, log := newWatchTestDeps(t)
	log.candidates = followPool(6)
	log.comments = []string{"一", "一", "二", "三"}
	log.mention = 9

	out, err := watchWithDeps(context.Background(), "", deps)
	if err != nil {
		t.Fatal(err)
	}
	if out.CommentCount != 2 {
		t.Fatalf("最多只应发布 2 条评论，实际 %d", out.CommentCount)
	}
	if strings.Join(postedTexts(log.postedComments), "|") != "一|二" {
		t.Fatalf("应去重后按序取前 2 条，实际 %v", log.postedComments)
	}
	if !out.Mentioned {
		t.Fatal("点名合法时应标记已 @")
	}
	for index, comment := range log.postedComments {
		if len(comment.Targets) != closingMentionMax {
			t.Fatalf("第 %d 条评论的 @ 人数应钳制到 %d，实际 %+v",
				index+1, closingMentionMax, comment.Targets)
		}
	}
}

// TestWatchMentionsPerComment 两条评论各自 @ 不同的人，互不影响。
func TestWatchMentionsPerComment(t *testing.T) {
	deps, log := newWatchTestDeps(t)
	pool := followPool(5)
	log.candidates = pool
	log.plan = []ViewerRemark.RemarkComment{
		{Text: "橘猫那一下太可爱了", Mention: 1, At: []string{"关注A"}},
		{Text: "窗台的光影真好看", Mention: 1, At: []string{"关注C"}},
	}

	out, err := watchWithDeps(context.Background(), "", deps)
	if err != nil {
		t.Fatal(err)
	}
	if len(log.postedComments) != 2 {
		t.Fatalf("应发布 2 条评论，实际 %+v", log.postedComments)
	}
	if len(log.postedComments[0].Targets) != 1 || log.postedComments[0].Targets[0].UID != pool[0].UID {
		t.Fatalf("第一条应 @ 关注A，实际 %+v", log.postedComments[0].Targets)
	}
	if len(log.postedComments[1].Targets) != 1 || log.postedComments[1].Targets[0].UID != pool[2].UID {
		t.Fatalf("第二条应 @ 关注C，实际 %+v", log.postedComments[1].Targets)
	}
	if !out.Mentioned {
		t.Fatal("任一条 @ 到人都应标记已 @")
	}
}

// TestWatchOneCommentMentionsOneNot 一条评论 @ 人、另一条不 @，互不影响。
func TestWatchOneCommentMentionsOneNot(t *testing.T) {
	deps, log := newWatchTestDeps(t)
	pool := followPool(5)
	log.candidates = pool
	log.plan = []ViewerRemark.RemarkComment{
		{Text: "橘猫那一下太可爱了", Mention: 0},
		{Text: "窗台的光影真好看", Mention: 2, At: []string{"关注B", "关注D"}},
	}

	out, err := watchWithDeps(context.Background(), "", deps)
	if err != nil {
		t.Fatal(err)
	}
	if len(log.postedComments) != 2 {
		t.Fatalf("应发布 2 条评论，实际 %+v", log.postedComments)
	}
	if len(log.postedComments[0].Targets) != 0 {
		t.Fatalf("第一条不 @ 时不应带目标: %+v", log.postedComments[0].Targets)
	}
	if len(log.postedComments[1].Targets) != 2 {
		t.Fatalf("第二条应 @ 2 人，实际 %+v", log.postedComments[1].Targets)
	}
	if !out.Mentioned {
		t.Fatal("第二条 @ 到人时应标记已 @")
	}
}

// TestWatchInvalidNameFallsBackOnlyThatComment 某条评论认不出名字时只回退该条，另一条不受影响。
func TestWatchInvalidNameFallsBackOnlyThatComment(t *testing.T) {
	deps, log := newWatchTestDeps(t)
	log.candidates = followPool(6)
	log.plan = []ViewerRemark.RemarkComment{
		{Text: "橘猫那一下太可爱了", Mention: 2, At: []string{"幽灵用户"}},
		{Text: "窗台的光影真好看", Mention: 1, At: []string{"关注E"}},
	}

	_, err := watchWithDeps(context.Background(), "", deps)
	if err != nil {
		t.Fatal(err)
	}
	if len(log.postedComments) != 2 {
		t.Fatalf("应发布 2 条评论，实际 %+v", log.postedComments)
	}
	if got := len(log.postedComments[0].Targets); got != 2 {
		t.Fatalf("认不出名字时应按该条人数随机挑 2 人，实际 %d 人", got)
	}
	for _, target := range log.postedComments[0].Targets {
		if target.Name == "幽灵用户" {
			t.Fatalf("不允许把模型编造的名字当成 @ 目标: %+v", log.postedComments[0].Targets)
		}
	}
	if len(log.postedComments[1].Targets) != 1 || log.postedComments[1].Targets[0].Name != "关注E" {
		t.Fatalf("另一条应保持模型点名，实际 %+v", log.postedComments[1].Targets)
	}
}

// TestWatchZeroMentionOverridesOwnNames 某条评论 mention=0 时，即使带名字也不 @。
func TestWatchZeroMentionOverridesOwnNames(t *testing.T) {
	deps, log := newWatchTestDeps(t)
	log.candidates = followPool(5)
	log.plan = []ViewerRemark.RemarkComment{
		{Text: "橘猫那一下太可爱了", Mention: 0, At: []string{"关注A"}},
		{Text: "窗台的光影真好看", Mention: 1, At: []string{"关注B"}},
	}

	out, err := watchWithDeps(context.Background(), "", deps)
	if err != nil {
		t.Fatal(err)
	}
	if len(log.postedComments[0].Targets) != 0 {
		t.Fatalf("mention=0 应压过名字，实际 %+v", log.postedComments[0].Targets)
	}
	if len(log.postedComments[1].Targets) != 1 || log.postedComments[1].Targets[0].Name != "关注B" {
		t.Fatalf("另一条仍应正常 @，实际 %+v", log.postedComments[1].Targets)
	}
	if !out.Mentioned {
		t.Fatal("第二条 @ 到人时应标记已 @")
	}
}

// TestWatchMentionsModelNamedUser 模型在候选清单里点名时，按名单原样 @ 该用户。
func TestWatchMentionsModelNamedUser(t *testing.T) {
	deps, log := newWatchTestDeps(t)
	pool := followPool(5)
	log.candidates = pool
	log.mention = 1
	log.at = []string{"关注C"}

	out, err := watchWithDeps(context.Background(), "", deps)
	if err != nil {
		t.Fatal(err)
	}
	if len(log.postedTargets) != 1 || log.postedTargets[0].UID != pool[2].UID {
		t.Fatalf("应 @ 模型点名的用户 %+v，实际 %+v", pool[2], log.postedTargets)
	}
	if !out.Mentioned {
		t.Fatal("点名合法时应标记已 @")
	}
	// 候选清单交给模型时只含序号与展示名，不含 UID。
	if len(log.candidatesSeen) != len(pool) || log.candidatesSeen[2].Name != "关注C" || log.candidatesSeen[2].Index != 3 {
		t.Fatalf("候选清单应以序号+展示名传给模型: %+v", log.candidatesSeen)
	}
}

// TestWatchNoMentionWhenModelSaysZero 模型给 0 时不 @ 特别关注。
func TestWatchNoMentionWhenModelSaysZero(t *testing.T) {
	deps, log := newWatchTestDeps(t)
	log.mention = 0

	out, err := watchWithDeps(context.Background(), "", deps)
	if err != nil {
		t.Fatal(err)
	}
	if out.Mentioned || len(log.postedTargets) != 0 {
		t.Fatalf("模型给 0 时不应 @：out=%+v targets=%+v", out, log.postedTargets)
	}
}

func TestWatchDegradesWhenClipUndecodable(t *testing.T) {
	deps, log := newWatchTestDeps(t)
	log.understandErr = errors.New("ffmpeg 解码失败")

	out, err := watchWithDeps(context.Background(), "对话者想看猫", deps)
	if err != nil {
		t.Fatal(err)
	}
	if out.VisualSource != "material" {
		t.Fatalf("解码失败时应降级为纯采集材料，实际 %q", out.VisualSource)
	}
	if !strings.Contains(out.Note, "无法解码") {
		t.Fatalf("降级需要在结果里说明，实际 %q", out.Note)
	}
	if !strings.Contains(log.material, "对话者想看猫") {
		t.Fatalf("降级材料仍应带上对话者要求，实际 %q", log.material)
	}
	if !out.CommentPosted || log.markIndex != 0 {
		t.Fatal("降级也必须走完评论与标记，不能中断")
	}
}

// TestWatchSkipsCommentWhenImpressionEmpty 视频印象为空时不硬编评论，但观看流程照常完成。
func TestWatchSkipsCommentWhenImpressionEmpty(t *testing.T) {
	deps, log := newWatchTestDeps(t)
	log.impressionEmpty = true

	out, err := watchWithDeps(context.Background(), "", deps)
	if err != nil {
		t.Fatal(err)
	}
	if out.CommentPosted || out.CommentCount != 0 || out.Comment != "" {
		t.Fatalf("印象为空不应生成或发布评论：%+v", out)
	}
	if !strings.Contains(out.Note, "视频印象为空") {
		t.Fatalf("跳过评论需要在结果里说明，实际 %q", out.Note)
	}
	if log.markIndex != 0 || log.removedIndex != 0 {
		t.Fatalf("跳过评论仍应标记并清理片段，mark=%d remove=%d", log.markIndex, log.removedIndex)
	}
	for _, step := range log.order {
		if step == "remark" || step == "post" {
			t.Fatalf("印象为空不应调用评论生成或发布：%v", log.order)
		}
	}
}

// TestWatchCommentFailureStillCompletes 评论生成失败只记录说明，不阻断标记与清理。
func TestWatchCommentFailureStillCompletes(t *testing.T) {
	deps, log := newWatchTestDeps(t)
	log.remarkErr = errors.New("模型超时")

	out, err := watchWithDeps(context.Background(), "", deps)
	if err != nil {
		t.Fatal(err)
	}
	if out.CommentPosted || !strings.Contains(out.Note, "视频评论生成失败") {
		t.Fatalf("评论失败应记录说明：%+v", out)
	}
	if log.markIndex != 0 || log.removedIndex != 0 {
		t.Fatalf("评论失败也应完成观看，mark=%d remove=%d", log.markIndex, log.removedIndex)
	}
}

func TestWatchStopsWhenNothingToWatch(t *testing.T) {
	deps, _ := newWatchTestDeps(t)
	deps.nextSegment = func() (viewerstore.WatchSegment, bool, error) {
		return viewerstore.WatchSegment{}, false, nil
	}

	out, err := watchWithDeps(context.Background(), "", deps)
	if err != nil {
		t.Fatal(err)
	}
	if out.Triggered || out.Reason == "" {
		t.Fatalf("没有片段时应明确说明原因: %+v", out)
	}
}

// setupMilestoneDir 隔离本地目录，模拟「进程重启」。
func setupMilestoneDir(t *testing.T) {
	t.Helper()
	oldDir := config.LocalDir
	config.LocalDir = t.TempDir()
	t.Cleanup(func() { config.LocalDir = oldDir })
}

func TestMilestonePostsOncePerBatch(t *testing.T) {
	setupMilestoneDir(t)

	if _, _, ok := milestoneGate(8); !ok {
		t.Fatal("累计满 8 个完整视频时应允许发阶段动态")
	}
	// 模拟重启：重新从磁盘读里程碑，仍在发布中，按已发处理，不重复发。
	if _, _, ok := milestoneGate(8); ok {
		t.Fatal("重启后不应重复发布同一个里程碑")
	}
	if _, _, ok := milestoneGate(15); ok {
		t.Fatal("未满下一个 8 个不应发布")
	}
	if err := saveMilestone(milestoneState{Milestone: 1, Status: milestoneDone}); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := milestoneGate(16); !ok {
		t.Fatal("满 16 个完整视频应发布下一条阶段动态")
	}
	if _, _, ok := milestoneGate(23); ok {
		t.Fatal("刚写过 posting 的里程碑不应立刻重发")
	}
}

func TestMilestoneRollbackAllowsRetry(t *testing.T) {
	setupMilestoneDir(t)
	if err := saveMilestone(milestoneState{Milestone: 1, Status: milestoneDone}); err != nil {
		t.Fatal(err)
	}
	target, prev, ok := milestoneGate(16)
	if !ok || target != 2 || prev != 1 {
		t.Fatalf("应准备发布第 2 个里程碑: target=%d prev=%d ok=%v", target, prev, ok)
	}
	rollbackMilestone(prev)
	if _, _, ok := milestoneGate(16); !ok {
		t.Fatal("发布失败回滚后应允许下次重试")
	}
}

func TestMilestoneCountsWholeVideosNotSegments(t *testing.T) {
	deps, log := newWatchTestDeps(t)
	log.segmentTotal = 1
	log.completedVideos = 7

	out, err := watchWithDeps(context.Background(), "", deps)
	if err != nil {
		t.Fatal(err)
	}
	if out.CompletedVideos != 7 {
		t.Fatalf("里程碑计数应取完整视频累计数，实际 %d", out.CompletedVideos)
	}
	if log.milestoneCount != 7 {
		t.Fatalf("应把完整视频累计数交给里程碑判定，实际 %d", log.milestoneCount)
	}
}
