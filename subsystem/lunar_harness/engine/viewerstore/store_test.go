package viewerstore

import (
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
)

// isolateViewerDB 为每个用例准备独立的观影记忆库。
func isolateViewerDB(t *testing.T) {
	t.Helper()
	oldDir := config.LocalDir
	config.LocalDir = t.TempDir()
	Close()
	if err := Init(); err != nil {
		t.Fatalf("初始化观影记忆库失败: %v", err)
	}
	t.Cleanup(func() {
		Close()
		config.LocalDir = oldDir
	})
}

// TestOpenStoreKeepsOpenerParams 建库后的连接参数与读写行为正常。
func TestOpenStoreKeepsOpenerParams(t *testing.T) {
	isolateViewerDB(t)
	db, err := openStore()
	if err != nil {
		t.Fatalf("取连接失败: %v", err)
	}
	var busy int64
	if err := db.QueryRow("PRAGMA busy_timeout").Scan(&busy); err != nil {
		t.Fatalf("读取 busy_timeout 失败: %v", err)
	}
	if busy != 10000 {
		t.Fatalf("busy_timeout 应为 10000，实际 %d", busy)
	}
	var mode string
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("读取 journal_mode 失败: %v", err)
	}
	if mode != "wal" {
		t.Fatalf("journal_mode 应为 wal，实际 %q", mode)
	}
	if count, err := CountRows("watch_video"); err != nil || count != 0 {
		t.Fatalf("新建库待看列表应为空，实际 %d err=%v", count, err)
	}
	if count, err := CountRows("watch_segment"); err != nil || count != 0 {
		t.Fatalf("新建库切片表应为空，实际 %d err=%v", count, err)
	}
}
