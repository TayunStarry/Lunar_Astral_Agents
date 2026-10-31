package resource

// store.go 统一句柄库（SQLite handles.db）—— 单库承载 URL 主表、六类句柄映射、段落与缓存
//
// 与旧实现的关键差异：
//   - 不再拆 archive.db / media.db / web.db，URL、解析文本、句柄、缓存全部在一库；
//   - 句柄表只存 (seq, handle, url) 三列纯映射，元数据与解析文本在 url_master；
//   - url_master.bound_handle 的偏唯一索引保证「一条 URL 最多被一个句柄指向」；
//   - 序号按类型表 MAX(seq) 重建，永不复用。

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"

	_ "github.com/mattn/go-sqlite3"
)

// Init 初始化统一句柄库，幂等
func Init(dbPath string) error {
	archiveInitMu.Lock()
	defer archiveInitMu.Unlock()
	if archiveDB != nil && archiveDB.initialized {
		return nil
	}
	db := &store{}
	if err := db.init(dbPath); err != nil {
		return err
	}
	archiveDB = db
	logger.Info("Persistence", "统一句柄库初始化完成: %s", dbPath)
	// 内容仓库根目录（documents / audios / multimedia / unknown）随句柄库一起就绪：
	// unknown 专门存放不可读二进制文件（zip/7z/bin/exe/dll/gguf…），同样纳入扫描与 GC。
	for _, root := range StorageRoots {
		if err := os.MkdirAll(filepath.Join(config.LocalDir, root), 0o755); err != nil {
			logger.Warn("Persistence", "创建内容仓库目录失败(%s): %v", root, err)
		}
	}
	// persistence 的落盘观察者：三棵内容仓库树内的写 / 移 / 删 / 读全部同步句柄
	persistenceObserverOnce.Do(func() {
		registerObserver()
	})
	// 启动同步：把 documents / audios / multimedia 内全部实体纳入句柄映射
	if err := SyncStorageHandles(); err != nil {
		logger.Warn("Persistence", "内容仓库句柄同步失败: %v", err)
	}
	// 启动 GC：孤儿句柄入池、可清理媒体建档、按上一轮调用情况结算积分
	if err := RunStartupGC(); err != nil {
		logger.Warn("Persistence", "启动 GC 失败: %v", err)
	}
	return nil
}

// EnsureReady 确保句柄库可用，必要时按默认路径初始化
func EnsureReady() error {
	if archiveDB != nil && archiveDB.initialized && archiveDB.db != nil {
		return nil
	}
	return Init(config.ArchiveDBPath)
}

func (d *store) init(dbPath string) error {
	dir := filepath.Dir(dbPath)
	if dir != "." && dir != "/" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("创建句柄库目录失败: %v", err)
		}
	}
	db, err := sql.Open("sqlite3", dbPath+"?_busy_timeout=10000&_journal_mode=WAL")
	if err != nil {
		return fmt.Errorf("连接句柄库失败: %v", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(5 * time.Minute)
	if err := db.Ping(); err != nil {
		db.Close()
		return fmt.Errorf("测试句柄库连接失败: %v", err)
	}
	for _, pragma := range []string{
		"PRAGMA synchronous = NORMAL",
		"PRAGMA foreign_keys = OFF",
		"PRAGMA cache_size = 10000",
	} {
		if _, err := db.Exec(pragma); err != nil {
			logger.Error("Persistence", "句柄库 PRAGMA 失败(%s): %v", pragma, err)
		}
	}
	d.db = db
	if err := d.ensureSchema(); err != nil {
		db.Close()
		d.db = nil
		return err
	}
	d.initialized = true
	return nil
}

// ensureSchema 建表（幂等）
func (d *store) ensureSchema() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS url_master (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			url           TEXT NOT NULL,
			parse_text    TEXT NOT NULL DEFAULT '',
			parse_state   TEXT NOT NULL DEFAULT 'none',
			bound_handle  TEXT NOT NULL DEFAULT '',
			kind          TEXT NOT NULL DEFAULT '',
			name          TEXT NOT NULL DEFAULT '',
			ext           TEXT NOT NULL DEFAULT '',
			mime          TEXT NOT NULL DEFAULT '',
			content_hash  TEXT NOT NULL DEFAULT '',
			blob_path     TEXT NOT NULL DEFAULT '',
			size_bytes    INTEGER NOT NULL DEFAULT 0,
			width         INTEGER NOT NULL DEFAULT 0,
			height        INTEGER NOT NULL DEFAULT 0,
			duration_ms   INTEGER NOT NULL DEFAULT 0,
			title         TEXT NOT NULL DEFAULT '',
			tags          TEXT NOT NULL DEFAULT '[]',
			origin        TEXT NOT NULL DEFAULT '',
			retain        TEXT NOT NULL DEFAULT 'normal',
			state         TEXT NOT NULL DEFAULT 'active',
			section_count INTEGER NOT NULL DEFAULT 0,
			created_at    INTEGER NOT NULL DEFAULT 0,
			used_at       INTEGER NOT NULL DEFAULT 0,
			parsed_at     INTEGER NOT NULL DEFAULT 0,
			blob_owned    INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uk_url_master_url ON url_master(url)`,
		// 一条 URL 只能被一个句柄占用：偏唯一索引即数据库级唯一绑定
		`CREATE UNIQUE INDEX IF NOT EXISTS uk_url_master_bound ON url_master(bound_handle) WHERE bound_handle <> ''`,
		`CREATE INDEX IF NOT EXISTS idx_url_master_hash ON url_master(kind, content_hash) WHERE content_hash <> ''`,
		`CREATE INDEX IF NOT EXISTS idx_url_master_kind ON url_master(kind, id DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_url_master_retain ON url_master(retain, used_at)`,

		`CREATE TABLE IF NOT EXISTS handle_file (
			seq    INTEGER PRIMARY KEY,
			handle TEXT NOT NULL,
			url    TEXT NOT NULL,
			UNIQUE(handle), UNIQUE(url)
		)`,
		`CREATE TABLE IF NOT EXISTS handle_image (
			seq    INTEGER PRIMARY KEY,
			handle TEXT NOT NULL,
			url    TEXT NOT NULL,
			UNIQUE(handle), UNIQUE(url)
		)`,
		`CREATE TABLE IF NOT EXISTS handle_audio (
			seq    INTEGER PRIMARY KEY,
			handle TEXT NOT NULL,
			url    TEXT NOT NULL,
			UNIQUE(handle), UNIQUE(url)
		)`,
		`CREATE TABLE IF NOT EXISTS handle_video (
			seq    INTEGER PRIMARY KEY,
			handle TEXT NOT NULL,
			url    TEXT NOT NULL,
			UNIQUE(handle), UNIQUE(url)
		)`,
		`CREATE TABLE IF NOT EXISTS handle_animated (
			seq    INTEGER PRIMARY KEY,
			handle TEXT NOT NULL,
			url    TEXT NOT NULL,
			UNIQUE(handle), UNIQUE(url)
		)`,
		`CREATE TABLE IF NOT EXISTS handle_web (
			seq    INTEGER PRIMARY KEY,
			handle TEXT NOT NULL,
			url    TEXT NOT NULL,
			UNIQUE(handle), UNIQUE(url)
		)`,

		`CREATE TABLE IF NOT EXISTS doc_sections (
			url        TEXT NOT NULL,
			ordinal    INTEGER NOT NULL,
			level      INTEGER NOT NULL DEFAULT 0,
			title      TEXT NOT NULL DEFAULT '',
			path       TEXT NOT NULL DEFAULT '',
			line_start INTEGER NOT NULL DEFAULT 0,
			line_end   INTEGER NOT NULL DEFAULT 0,
			chars      INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY(url, ordinal)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_doc_sections_title ON doc_sections(title)`,

		`CREATE TABLE IF NOT EXISTS cache (
			kind       TEXT NOT NULL,
			source_url TEXT NOT NULL,
			variant    TEXT NOT NULL,
			path       TEXT NOT NULL DEFAULT '',
			bytes      INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL DEFAULT 0,
			used_at    INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY(kind, source_url, variant)
		)`,

		// GC 专用表 1：源文件不存在/URL 不可达且没有任何文本描述的句柄。
		// 启动时把这些句柄从主表移除；行本身保留为「空闲句柄池」，新资源优先复用。
		`CREATE TABLE IF NOT EXISTS gc_orphan_handle (
			handle     TEXT PRIMARY KEY,
			kind       TEXT NOT NULL,
			seq        INTEGER NOT NULL,
			url        TEXT NOT NULL DEFAULT '',
			blob_path  TEXT NOT NULL DEFAULT '',
			reason     TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_gc_orphan_kind_seq ON gc_orphan_handle(kind, seq)`,

		// GC 专用表 2：可清理的根目录媒体（multimedia 根图片/视频、audios 根哈希音频）。
		// call_points 为调用积分（-20..20），current_call 为本轮是否被查询（0/1）。
		`CREATE TABLE IF NOT EXISTS gc_media_score (
			handle       TEXT PRIMARY KEY,
			kind         TEXT NOT NULL,
			blob_path    TEXT NOT NULL,
			call_points  INTEGER NOT NULL DEFAULT 0,
			current_call INTEGER NOT NULL DEFAULT 0,
			created_at   INTEGER NOT NULL DEFAULT 0,
			updated_at   INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_gc_media_score_path ON gc_media_score(blob_path)`,
	}
	for _, stmt := range stmts {
		if _, err := d.db.Exec(stmt); err != nil {
			return fmt.Errorf("句柄库建表失败: %v", err)
		}
	}
	// 序号缓存一律从各句柄表重建
	archiveSeqLoaded = false
	archiveSeqCache = map[string]int64{}
	return nil
}

// loadSeqCache 从六张句柄表重建各类型码的下一可用序号（序号永不复用）
func (d *store) loadSeqCache() error {
	cache := map[string]int64{}
	for _, kind := range []string{KindDocument, KindImage, KindAudio, KindVideo, KindAnimated, KindURL} {
		var maxSeq sql.NullInt64
		if err := d.db.QueryRow(`SELECT MAX(seq) FROM ` + handleTable(kind)).Scan(&maxSeq); err != nil {
			return err
		}
		cache[kind] = maxSeq.Int64
	}
	archiveSeqCache = cache
	archiveSeqLoaded = true
	return nil
}

// nextSeqLocked 分配下一个序号（调用方须持有 archiveMu）
func (d *store) nextSeqLocked(kind string) (int64, error) {
	if !archiveSeqLoaded {
		if err := d.loadSeqCache(); err != nil {
			return 0, err
		}
	}
	next := archiveSeqCache[kind] + 1
	archiveSeqCache[kind] = next
	return next, nil
}

// close 关闭句柄库连接
func (d *store) close() error {
	if d == nil || d.db == nil {
		return nil
	}
	err := d.db.Close()
	d.db = nil
	d.initialized = false
	return err
}

// Close 关闭全局句柄库实例（幂等）
func Close() error {
	if archiveDB == nil {
		return nil
	}
	err := archiveDB.close()
	archiveDB = nil
	return err
}
