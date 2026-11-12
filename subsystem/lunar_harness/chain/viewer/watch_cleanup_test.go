package viewer

// watch_cleanup_test.go 看完片段后的本地清理：标记成功即删除本地片段，清理失败不阻断观看流程。

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestWatchRemoveFailureDoesNotBreakFlow(t *testing.T) {
	deps, log := newWatchTestDeps(t)
	log.segmentTotal = 1
	log.completedVideos = 1
	log.removeErr = errors.New("文件被占用")

	out, err := watchWithDeps(context.Background(), "", deps)
	if err != nil {
		t.Fatalf("清理失败不应让观看流程报错: %v", err)
	}
	if !out.CommentPosted || out.CompletedVideos != 1 {
		t.Fatalf("清理失败不应影响观看结果: %+v", out)
	}
	if log.removedIndex != 0 {
		t.Fatalf("应尝试删除第 1 段，实际 %d", log.removedIndex)
	}
	if !strings.Contains(out.Note, "清理本地视频失败") {
		t.Fatalf("清理失败应写入说明: %q", out.Note)
	}
	if log.milestoneCount != 1 {
		t.Fatalf("清理失败不应阻断阶段动态，实际 %d 次", log.milestoneCount)
	}
}

func TestWatchKeepsClipWhenMarkFails(t *testing.T) {
	deps, log := newWatchTestDeps(t)
	log.segmentTotal = 1
	log.markErr = errors.New("数据库忙")

	out, err := watchWithDeps(context.Background(), "", deps)
	if err != nil {
		t.Fatal(err)
	}
	if log.removedIndex != -1 {
		t.Fatalf("标记失败时不应删除文件，否则未看片段会指向已删文件: %d", log.removedIndex)
	}
	if !strings.Contains(out.Note, "标记视频已看失败") {
		t.Fatalf("标记失败应写入说明: %q", out.Note)
	}
}
