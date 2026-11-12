package viewerstore

// store.go 观影记忆库（local_data/database/bilibili/viewer.db）的连接与建表。
//
// 连接与建表沿用仓库现有的 sqlite 约定（engine/resource/store.go）：
// 打开时带 busy_timeout + WAL、单连接、建表全部 IF NOT EXISTS。
//
// 表的职责：
//   - watch_video：待看视频列表；
//   - watch_segment：待看视频的切片与观看进度。

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
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
	// 旧版 JSON 只在首次打开时导入一次；失败不阻断库使用，下次打开会重试。
	if err := importLegacyJSON(db); err != nil {
		logger.Warn("BilibiliViewer", "历史 JSON 迁移失败（本次跳过，下次打开重试）: %v", err)
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
			archived_at   INTEGER NOT NULL DEFAULT 0,
			topics        TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_watch_video_queue
			ON watch_video(status, archived, completed, queue_seq)`,
		// 主题词偏好池：看过视频的印象主题词按时间衰减后的权重累积在这里。
		// 选片只读这张小表，不回扫 watch_video 历史行。
		`CREATE TABLE IF NOT EXISTS watch_topic (
			topic      TEXT PRIMARY KEY,
			weight     REAL NOT NULL DEFAULT 0,
			updated_at INTEGER NOT NULL DEFAULT 0
		)`,
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
		// 消息中心去重键：键本身就是业务唯一键（msg:/source:/历史 reply:），
		// seen_at 用于 30 天 TTL 与条数截断。
		`CREATE TABLE IF NOT EXISTS inbox_seen (
			key      TEXT PRIMARY KEY,
			seen_at  INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_inbox_seen_time ON inbox_seen(seen_at)`,
		// 已看视频：一个 BV 一条，watched_at 是最近一次观看时间（不是复合键）。
		`CREATE TABLE IF NOT EXISTS seen_video (
			bv          TEXT PRIMARY KEY,
			watched_at  INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_seen_video_time ON seen_video(watched_at)`,
		// 迁移标记：保证旧版 JSON 只导一次，避免 TTL 清空后把陈年数据重新灌回来。
		`CREATE TABLE IF NOT EXISTS viewer_meta (
			key    TEXT PRIMARY KEY,
			value  TEXT NOT NULL DEFAULT ''
		)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			return fmt.Errorf("建表失败: %w", err)
		}
	}
	return ensureWatchVideoColumns(db)
}

// ensureWatchVideoColumns 给旧库补齐后加的列（列已存在时跳过）。
func ensureWatchVideoColumns(db *sql.DB) error {
	rows, err := db.Query(`PRAGMA table_info(watch_video)`)
	if err != nil {
		return fmt.Errorf("读取待看视频表结构失败: %w", err)
	}
	existing := make(map[string]bool)
	for rows.Next() {
		var (
			cid       int
			name      string
			columnTyp string
			notNull   int
			defaultV  sql.NullString
			primary   int
		)
		if err := rows.Scan(&cid, &name, &columnTyp, &notNull, &defaultV, &primary); err != nil {
			rows.Close()
			return fmt.Errorf("解析待看视频表结构失败: %w", err)
		}
		existing[name] = true
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("读取待看视频表结构失败: %w", err)
	}
	// 单连接策略下必须先关掉结果集，否则后续 ALTER 会一直等这条连接。
	rows.Close()
	for _, add := range watchVideoColumnAdds {
		if existing[add.Column] {
			continue
		}
		if _, err := db.Exec(add.SQL); err != nil {
			return fmt.Errorf("补齐 %s 列失败: %w", add.Column, err)
		}
	}
	return nil
}

// CountRows 统计表行数（测试与对账用）。
func CountRows(table string) (int, error) {
	db, err := openStore()
	if err != nil {
		return 0, err
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}
