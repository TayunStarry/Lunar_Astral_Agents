package viewer

// test_helper_test.go 单元测试共用的隔离环境。

import (
	"testing"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/viewerstore"
)

// isolateViewerDB 为每个用例准备独立的观影记忆库。
func isolateViewerDB(t *testing.T) {
	t.Helper()
	oldDir := config.LocalDir
	config.LocalDir = t.TempDir()
	viewerstore.Close()
	if err := viewerstore.Init(); err != nil {
		t.Fatalf("初始化观影记忆库失败: %v", err)
	}
	t.Cleanup(func() {
		viewerstore.Close()
		config.LocalDir = oldDir
	})
}

// writeSeenRecord 写入一条历史已看记录，供选片去重用例构造前置状态。
func writeSeenRecord(t *testing.T, bvid string) {
	t.Helper()
	if err := viewerstore.RecordSeenVideos(map[string]int64{bvid: time.Now().Unix()}); err != nil {
		t.Fatalf("写入已看记录失败: %v", err)
	}
}
