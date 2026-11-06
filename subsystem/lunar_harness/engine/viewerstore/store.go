package viewerstore

// store.go 观影记忆库（local_data/database/bilibili/viewer.db）的连接与建表。
//
// 连接与建表沿用仓库现有的 sqlite 约定（engine/resource/store.go）：
// 打开时带 busy_timeout + WAL、单连接、建表全部 IF NOT EXISTS。
//
// 表的职责：
//   - viewer_observation：近期观感，BV 号主键，同视频重复观看覆盖更新，最多 ObservationMax 行；
//   - viewer_profile / viewer_stance：选片口味 / 评论倾向单行表（固定 key）；
//   - viewer_round：观影轮次追溯，最多 RoundMax 行；
//   - viewer_archive：淘汰前先写入的归档（先归档再删，可回退）；
//   - viewer_meta：迁移完成标记等内部状态。

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/dbopen"
)

var (
	storeMu sync.Mutex
	storeDB *sql.DB
)

// DataDir 返回观影记忆落盘目录（local_data/database/bilibili）。
func DataDir() string {
	root := strings.TrimSpace(config.LocalDir)
	if root == "" {
		root = "local_data"
	}
	return filepath.Join(root, "database", "bilibili")
}

// DatabasePath 观影记忆库路径。
func DatabasePath() string {
	return filepath.Join(DataDir(), dbName)
}

// Init 初始化观影记忆库（幂等）。
func Init() error {
	_, err := openStore()
	return err
}

// openStore 返回已初始化的连接；首次调用时建目录、开库并建表。
func openStore() (*sql.DB, error) {
	storeMu.Lock()
	defer storeMu.Unlock()
	if storeDB != nil {
		return storeDB, nil
	}
	path := DatabasePath()
	db, err := dbopen.Open(path, dbopen.Options{
		CreateDir:    true,
		MaxOpenConns: maxOpenConns,
		MaxIdleConns: maxIdleConns,
	})
	if err != nil {
		if cause, isDir := dbopen.DirCause(err); isDir {
			return nil, fmt.Errorf("创建观影记忆库目录失败: %w", cause)
		}
		return nil, fmt.Errorf("连接观影记忆库失败: %w", err)
	}
	if err := createTables(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	storeDB = db
	return storeDB, nil
}

// Close 关闭连接（测试隔离用）。
func Close() {
	storeMu.Lock()
	defer storeMu.Unlock()
	if storeDB != nil {
		_ = storeDB.Close()
		storeDB = nil
	}
}

// createTables 幂等建表。
func createTables(db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS viewer_observation (
			bv          TEXT PRIMARY KEY,
			title       TEXT NOT NULL DEFAULT '',
			url         TEXT NOT NULL DEFAULT '',
			topics      TEXT NOT NULL DEFAULT '',
			tone        TEXT NOT NULL DEFAULT '',
			conclusion  TEXT NOT NULL DEFAULT '',
			updated_at  INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS viewer_profile (
			slot        TEXT PRIMARY KEY,
			value       TEXT NOT NULL DEFAULT '',
			updated_at  INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS viewer_stance (
			slot        TEXT PRIMARY KEY,
			value       TEXT NOT NULL DEFAULT '',
			updated_at  INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS viewer_round (
			round_id    TEXT PRIMARY KEY,
			bvids       TEXT NOT NULL DEFAULT '',
			conclusion  TEXT NOT NULL DEFAULT '',
			created_at  INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS viewer_archive (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			kind         TEXT NOT NULL,
			payload      TEXT NOT NULL,
			reason       TEXT NOT NULL,
			archived_at  INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS viewer_meta (
			key    TEXT PRIMARY KEY,
			value  TEXT NOT NULL DEFAULT ''
		)`,
		// 待看视频列表：bv 主键保证入队幂等，queue_seq 固定入队顺序（不回收）；
		// 时间字段一律 Unix 秒，archived 只做逻辑归档，统计仍包含历史行。
		`CREATE TABLE IF NOT EXISTS watch_video (
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
		)`,
		`CREATE INDEX IF NOT EXISTS idx_watch_video_queue
			ON watch_video(status, archived, completed, queue_seq)`,
		// 片段表：(bv, seg_index) 唯一，重复切片段时整组覆盖；
		// 播放顺序依赖联表后的 queue_seq + seg_index，索引覆盖「未看片段」筛选。
		`CREATE TABLE IF NOT EXISTS watch_segment (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			bv          TEXT NOT NULL,
			seg_index   INTEGER NOT NULL,
			start_sec   INTEGER NOT NULL DEFAULT 0,
			end_sec     INTEGER NOT NULL DEFAULT 0,
			path        TEXT NOT NULL DEFAULT '',
			downloaded  INTEGER NOT NULL DEFAULT 0,
			watched     INTEGER NOT NULL DEFAULT 0,
			watched_at  INTEGER NOT NULL DEFAULT 0,
			UNIQUE(bv, seg_index)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_watch_segment_pending
			ON watch_segment(watched, bv, seg_index)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			return fmt.Errorf("建表失败: %w", err)
		}
	}
	return nil
}
