package viewerstore

// watch_migration_test.go 旧库（无 topics 列）的自动迁移与向后兼容。

import (
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/dbopen"
)

// TestLegacyWatchVideoGainsTopicsColumn 旧版 watch_video 启动后自动补列，历史行原样保留。
func TestLegacyWatchVideoGainsTopicsColumn(t *testing.T) {
	oldDir := config.LocalDir
	config.LocalDir = t.TempDir()
	Close()
	t.Cleanup(func() {
		Close()
		config.LocalDir = oldDir
	})

	db, err := dbopen.Open(DatabasePath(), dbopen.Options{CreateDir: true})
	if err != nil {
		t.Fatalf("准备旧版数据库失败: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE watch_video (
		bv            TEXT PRIMARY KEY,
		title         TEXT NOT NULL DEFAULT '',
		intro         TEXT NOT NULL DEFAULT '',
		author_uid    TEXT NOT NULL DEFAULT '',
		author_name   TEXT NOT NULL DEFAULT '',
		comments      TEXT NOT NULL DEFAULT '[]',
		duration_sec  INTEGER NOT NULL DEFAULT 0,
		status        TEXT NOT NULL DEFAULT 'pending',
		reject_reason TEXT NOT NULL DEFAULT '',
		queue_seq     INTEGER NOT NULL DEFAULT 0,
		queued_at     INTEGER NOT NULL DEFAULT 0,
		reviewed_at   INTEGER NOT NULL DEFAULT 0,
		completed     INTEGER NOT NULL DEFAULT 0,
		completed_at  INTEGER NOT NULL DEFAULT 0,
		archived      INTEGER NOT NULL DEFAULT 0,
		archived_at   INTEGER NOT NULL DEFAULT 0
	)`); err != nil {
		db.Close()
		t.Fatalf("创建旧版表失败: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO watch_video (bv, title, status, completed) VALUES (?, ?, ?, 1)`,
		"BV1legacy", "旧标题", string(WatchStatusApproved)); err != nil {
		db.Close()
		t.Fatalf("写入历史行失败: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("关闭旧版数据库失败: %v", err)
	}

	if err := Init(); err != nil {
		t.Fatalf("启动时自动迁移失败: %v", err)
	}
	video, ok, err := LoadVideo("BV1legacy")
	if err != nil || !ok {
		t.Fatalf("历史行丢失: ok=%v err=%v", ok, err)
	}
	if video.Title != "旧标题" || video.Status != WatchStatusApproved || !video.Completed {
		t.Fatalf("历史行字段被破坏: %+v", video)
	}
	if len(video.Topics) != 0 {
		t.Fatalf("历史行没有主题词时应为空列表，实际 %v", video.Topics)
	}

	// 迁移后新行可以正常写入主题词。
	if _, err := EnqueueVideos([]WatchCandidate{{
		BV: "BV1fresh", Title: "新标题", Topics: []string{"人工智能"},
	}}); err != nil {
		t.Fatalf("迁移后入队失败: %v", err)
	}
	fresh, ok, err := LoadVideo("BV1fresh")
	if err != nil || !ok || len(fresh.Topics) != 1 || fresh.Topics[0] != "人工智能" {
		t.Fatalf("迁移后主题词未落库: ok=%v video=%+v err=%v", ok, fresh, err)
	}
}

// TestEnsureWatchVideoColumnsIsIdempotent 重复执行迁移不报错，也不改动既有数据。
func TestEnsureWatchVideoColumnsIsIdempotent(t *testing.T) {
	isolateViewerDB(t)
	db, err := openStore()
	if err != nil {
		t.Fatalf("取连接失败: %v", err)
	}
	for round := 0; round < 2; round++ {
		if err := ensureWatchVideoColumns(db); err != nil {
			t.Fatalf("第 %d 次迁移失败: %v", round+1, err)
		}
	}
}
