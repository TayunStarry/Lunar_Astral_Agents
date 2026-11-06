package viewer

// watch_branch_test.go 看视频分支：取片段顺序、最后一段 @、降级、状态更新、8 视频里程碑去重。

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/viewerstore"
)

// newWatchTestDeps 构造全部外设都走桩的依赖，记录调用顺序与参数。
func newWatchTestDeps(t *testing.T) (watchDeps, *watchTestLog) {
	t.Helper()
	log := &watchTestLog{removedIndex: -1}
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
				Intro: "这是一个简介", Comments: []string{"好看"},
			}, true, nil
		},
		loadSegments: func(string) ([]viewerstore.WatchSegment, error) {
			out := make([]viewerstore.WatchSegment, 0, log.segmentTotal)
			for i := 0; i < log.segmentTotal; i++ {
				out = append(out, viewerstore.WatchSegment{BV: "BV1test", Index: i})
			}
			return out, nil
		},
		understand: func(_ context.Context, _ viewerstore.WatchSegment, _ viewerstore.WatchVideo, _ int) (string, string, error) {
			log.order = append(log.order, "understand")
			if log.understandErr != nil {
				return "", "", log.understandErr
			}
			return "画面里一只橘猫趴在窗台上", "", nil
		},
		remark: func(_ context.Context, _ viewerstore.WatchVideo, material string) (string, error) {
			log.order = append(log.order, "remark")
			log.material = material
			return "橘猫那一下太可爱了", nil
		},
		postComment: func(_ viewerstore.WatchVideo, message string, mentionAuthor bool) (bool, bool, error) {
			log.order = append(log.order, "post")
			log.posted, log.mentionAsked = message, mentionAuthor
			if log.postFailed {
				return false, false, nil
			}
			return true, mentionAuthor, nil
		},
		markWatched: func(_ string, index int, _ int64) (viewerstore.SegmentWatchResult, error) {
			log.order = append(log.order, "mark")
			log.markIndex = index
			if log.markErr != nil {
				return viewerstore.SegmentWatchResult{}, log.markErr
			}
			return viewerstore.SegmentWatchResult{VideoCompleted: index+1 == log.segmentTotal, CompletedVideos: log.completedVideos}, nil
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
	material        string
	posted          string
	mentionAsked    bool
	postFailed      bool
	markIndex       int
	markErr         error
	removedIndex    int
	removeErr       error
	activity        string
	milestoneCount  int
}

func TestWatchSegmentPublishesCommentAndUpdatesActivity(t *testing.T) {
	deps, log := newWatchTestDeps(t)
	log.segmentIndex, log.segmentTotal = 0, 3

	out, err := watchWithDeps(context.Background(), "", deps)
	if err != nil {
		t.Fatal(err)
	}
	if !out.Triggered || out.SegmentIndex != 1 || out.SegmentTotal != 3 || out.LastSegment {
		t.Fatalf("片段定位不符: %+v", out)
	}
	if !out.CommentPosted || out.Mentioned {
		t.Fatalf("非最后一段应发评论且不 @：%+v", out)
	}
	if log.mentionAsked {
		t.Fatal("非最后一段不应请求 @ 作者")
	}
	if !strings.Contains(log.material, "橘猫") || !strings.Contains(log.material, "测试视频") {
		t.Fatalf("评论材料应同时包含画面理解与视频印象，实际 %q", log.material)
	}
	if log.posted != "橘猫那一下太可爱了" {
		t.Fatalf("评论正文未传达到发布环节: %q", log.posted)
	}
	if log.markIndex != 0 {
		t.Fatalf("应标记同一个片段已看，实际 %d", log.markIndex)
	}
	if !strings.Contains(log.activity, "第 1/3 段") {
		t.Fatalf("看完片段应立即更新状态，实际 %q", log.activity)
	}
	// 顺序：取片段 → 理解画面 → 立刻更新状态 → 生成评论 → 发布 → 标记已看 → 清理本地文件。
	want := []string{"next", "understand", "activity", "remark", "post", "mark", "remove"}
	if strings.Join(log.order, ",") != strings.Join(want, ",") {
		t.Fatalf("执行顺序不符：want %v，实际 %v", want, log.order)
	}
}

func TestWatchLastSegmentMentionsAuthor(t *testing.T) {
	deps, log := newWatchTestDeps(t)
	log.segmentIndex, log.segmentTotal = 2, 3

	out, err := watchWithDeps(context.Background(), "", deps)
	if err != nil {
		t.Fatal(err)
	}
	if !out.LastSegment {
		t.Fatal("最后一个片段应被识别")
	}
	if !log.mentionAsked || !out.Mentioned {
		t.Fatalf("最后一段的评论应带 @ 作者，实际 asked=%v mentioned=%v", log.mentionAsked, out.Mentioned)
	}
	if !strings.Contains(log.activity, "最后一个片段") {
		t.Fatalf("最后一段的状态文案不符: %q", log.activity)
	}
}

func TestWatchDegradesWhenClipUndecodable(t *testing.T) {
	deps, log := newWatchTestDeps(t)
	log.segmentIndex, log.segmentTotal = 0, 2
	log.understandErr = errors.New("ffmpeg 解码失败")

	out, err := watchWithDeps(context.Background(), "对话者想看猫", deps)
	if err != nil {
		t.Fatal(err)
	}
	if out.VisualSource != "impression" {
		t.Fatalf("解码失败时应降级为视频印象，实际 %q", out.VisualSource)
	}
	if !strings.Contains(out.Note, "无法解码") {
		t.Fatalf("降级需要在结果里说明，实际 %q", out.Note)
	}
	if !strings.Contains(log.material, "拿不到画面") || !strings.Contains(log.material, "对话者想看猫") {
		t.Fatalf("降级材料应说明拿不到画面并带上对话者要求，实际 %q", log.material)
	}
	if !out.CommentPosted || log.markIndex != 0 {
		t.Fatal("降级也必须走完评论与标记，不能中断")
	}
}

func TestWatchStopsWhenNothingToWatch(t *testing.T) {
	deps, _ := newWatchTestDeps(t)
	deps.nextSegment = func() (viewerstore.WatchSegment, bool, error) { return viewerstore.WatchSegment{}, false, nil }

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
	log.segmentIndex, log.segmentTotal = 0, 2
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
