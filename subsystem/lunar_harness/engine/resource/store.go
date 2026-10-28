package resource

// store.go 资源目录（SQLite archive.db）—— 三层存储的第一层
//
// 职责：句柄分配与解析、资源元数据、段落结构表、跨资源搜索。
// 明确不存：正文内容（在内容仓库里）、向量（在记忆库里）。
//
// 序号分配规则（ADR「双身份」）：序号是持久化主键，永不复用。
// 进程启动时从 resources 表重建计数器（MAX(seq)），
// 因此 @I.007 在重启后仍指向同一资源。

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/knowledge"
	persistence "LunarSubsystem/LunarHarness/engine/persistence"

	_ "github.com/mattn/go-sqlite3"
)

// Init 初始化资源目录（SQLite），幂等
//
// 初始化成功后注册两件事：
//   - 知识库系统的 archive 域库（/knowledge/ 的 ExecuteOn("archive", ...) 由此取连接）；
//   - persistence 的落盘观察者：三棵内容仓库树内的写 / 移 / 删 / 读全部自动同步句柄。
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
	logger.Info("Persistence", "资源目录初始化完成: %s", dbPath)
	// 把资源名录登记为知识库系统的域库：knowledge 不反向依赖本包（依赖方向 resource → knowledge）
	knowledge.RegisterDomain("archive", func() (*sql.DB, error) {
		if err := EnsureReady(); err != nil {
			return nil, err
		}
		return archiveDB.db, nil
	})
	// 落盘观察者：persistence 的写 / 移 / 删 / 读原语完成后来这里登记或清理句柄
	persistence.SetResourceObserver(observer{})
	// 启动同步：把 documents / audios / multimedia 内全部实体纳入句柄映射，
	// 同时清理源文件已丢失或损坏的失效句柄
	if err := SyncStorageHandles(); err != nil {
		logger.Warn("Persistence", "内容仓库句柄同步失败: %v", err)
	}
	return nil
}

// EnsureReady 确保资源目录可用，必要时按默认路径初始化
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
			return fmt.Errorf("创建资源目录数据库目录失败: %v", err)
		}
	}
	db, err := sql.Open("sqlite3", dbPath+"?_busy_timeout=10000&_journal_mode=WAL")
	if err != nil {
		return fmt.Errorf("连接资源目录失败: %v", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(5 * time.Minute)
	if err := db.Ping(); err != nil {
		db.Close()
		return fmt.Errorf("测试资源目录连接失败: %v", err)
	}
	for _, pragma := range []string{
		"PRAGMA synchronous = NORMAL",
		"PRAGMA foreign_keys = ON",
		"PRAGMA cache_size = 10000",
	} {
		if _, err := db.Exec(pragma); err != nil {
			logger.Error("FileManager", "资源目录 PRAGMA 失败(%s): %v", pragma, err)
		}
	}
	d.db = db
	if err := d.ensureSchema(); err != nil {
		db.Close()
		d.db = nil
		return err
	}
	d.initialized = true
	// 历史版本把视觉副本簿记放在 archive.db：搬到 media.db（备份失败则保留旧表待下次重试）
	d.migrateLegacyVisionCache(dbPath)
	return nil
}

// ensureSchema 建表（幂等）并初始化序号元数据
func (d *store) ensureSchema() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS resources (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			kind          TEXT NOT NULL,
			seq           INTEGER NOT NULL,
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
			summary       TEXT NOT NULL DEFAULT '',
			tags          TEXT NOT NULL DEFAULT '[]',
			origin        TEXT NOT NULL DEFAULT '',
			retain        TEXT NOT NULL DEFAULT 'normal',
			state         TEXT NOT NULL DEFAULT 'active',
			section_count INTEGER NOT NULL DEFAULT 0,
			created_at    INTEGER NOT NULL DEFAULT 0,
			used_at       INTEGER NOT NULL DEFAULT 0,
			-- blob_owned 实体是否为「归档私有副本」（1 = 归档可回收；0 = 前端/引擎所有，回收器不碰）
			blob_owned    INTEGER NOT NULL DEFAULT 0,
			-- url 链接地址（仅 @U 使用；不落盘，只记录地址与缓存摘要）
			url           TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_resources_handle ON resources(kind, seq)`,
		`CREATE INDEX IF NOT EXISTS idx_resources_hash ON resources(content_hash)`,
		// 注意：idx_resources_url 必须等下面的 ALTER 补完 url 列再建。
		// 既有库没有 url 列时先建索引会直接报错，导致整个资源目录初始化失败。
		`CREATE INDEX IF NOT EXISTS idx_resources_kind_created ON resources(kind, id DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_resources_retain ON resources(retain, used_at)`,
		`CREATE TABLE IF NOT EXISTS resource_sections (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			resource_id INTEGER NOT NULL,
			ordinal     INTEGER NOT NULL,
			level       INTEGER NOT NULL DEFAULT 0,
			title       TEXT NOT NULL DEFAULT '',
			path        TEXT NOT NULL DEFAULT '',
			line_start  INTEGER NOT NULL DEFAULT 0,
			line_end    INTEGER NOT NULL DEFAULT 0,
			chars       INTEGER NOT NULL DEFAULT 0,
			FOREIGN KEY(resource_id) REFERENCES resources(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sections_resource ON resource_sections(resource_id, ordinal)`,
		// 注意：视觉副本缓存（vision_cache）已迁到 media.db（见 engine/knowledge）。
		// 本库只保留资源名录自身的表：句柄、元数据、段落结构。
	}
	for _, stmt := range stmts {
		if _, err := d.db.Exec(stmt); err != nil {
			return fmt.Errorf("资源目录建表失败: %v", err)
		}
	}
	// 清理历史遗留的 _meta 空表：序号缓存实际从 resources.MAX(seq) 重建，该表已无用途
	if _, err := d.db.Exec(`DROP TABLE IF EXISTS _meta`); err != nil {
		logger.Warn("Persistence", "清理历史 _meta 表失败: %v", err)
	}
	// 兼容既有库：老版本没有 blob_owned 列，缺失时补上（ALTER 失败仅代表列已存在）
	// 必须先补齐列，后面的历史去重查询会读取 archiveResourceColumns。
	if _, err := d.db.Exec(`ALTER TABLE resources ADD COLUMN blob_owned INTEGER NOT NULL DEFAULT 0`); err != nil {
		// 列已存在是正常情况，不视为错误
		_ = err
	}
	// 兼容既有库：@U 链接资源需要 url 列（ALTER 失败仅代表列已存在）
	if _, err := d.db.Exec(`ALTER TABLE resources ADD COLUMN url TEXT NOT NULL DEFAULT ''`); err != nil {
		_ = err
	}
	if _, err := d.db.Exec(`CREATE INDEX IF NOT EXISTS idx_resources_url ON resources(url) WHERE url <> ''`); err != nil {
		logger.Warn("Persistence", "创建链接索引失败: %v", err)
	}
	// 兼容既有库：老版本 vision_cache 没有 used_at 列（LRU 淘汰依据），缺失时补上
	if _, err := d.db.Exec(`ALTER TABLE vision_cache ADD COLUMN used_at INTEGER NOT NULL DEFAULT 0`); err != nil {
		_ = err
	}
	// 文本类不记录内容哈希（历史库可能遗留）：先清空，避免把同名内容不同的文档误判为重复
	if _, err := d.db.Exec(`UPDATE resources SET content_hash = '' WHERE kind IN (?, ?)`, KindDocument, KindTable); err != nil {
		logger.Warn("Persistence", "清理历史文本哈希失败: %v", err)
	}
	// 内容哈希唯一：先收敛历史重复（保留生成目录/集合子目录，舍弃根目录重复件），
	// 再建立唯一索引，确保任何写入路径都无法制造第二份同哈希实体。
	if _, err := d.DedupeDuplicateHashes(); err != nil {
		logger.Warn("Persistence", "历史内容哈希去重失败: %v", err)
	}
	if _, err := d.db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_resources_kind_hash
		ON resources(kind, content_hash) WHERE content_hash <> ''`); err != nil {
		return fmt.Errorf("创建内容哈希唯一索引失败: %v", err)
	}
	return nil
}

// loadSeqCache 从库中重建各类型码的下一可用序号（序号永不复用）
func (d *store) loadSeqCache() error {
	rows, err := d.db.Query(`SELECT kind, MAX(seq) FROM resources GROUP BY kind`)
	if err != nil {
		return err
	}
	defer rows.Close()
	cache := map[string]int64{}
	for rows.Next() {
		var kind string
		var maxSeq sql.NullInt64
		if err := rows.Scan(&kind, &maxSeq); err != nil {
			return err
		}
		cache[kind] = maxSeq.Int64
	}
	if err := rows.Err(); err != nil {
		return err
	}
	archiveSeqCache = cache
	archiveSeqLoaded = true
	return nil
}

// close 关闭资源目录连接（进程退出与测试清理使用）
func (d *store) close() error {
	if d == nil || d.db == nil {
		return nil
	}
	err := d.db.Close()
	d.db = nil
	d.initialized = false
	return err
}

// Close 关闭全局资源目录实例（幂等）
func Close() error {
	if archiveDB == nil {
		return nil
	}
	err := archiveDB.close()
	archiveDB = nil
	return err
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

// InsertResource 写入一条资源目录记录并分配句柄，返回完整记录
func (d *store) InsertResource(res *Resource) error {
	archiveMu.Lock()
	defer archiveMu.Unlock()

	seq, err := d.nextSeqLocked(res.Kind)
	if err != nil {
		return err
	}
	res.Seq = seq
	res.Handle = FormatHandle(res.Kind, seq)
	if res.CreatedAt == 0 {
		res.CreatedAt = time.Now().Unix()
	}
	if res.UsedAt == 0 {
		res.UsedAt = res.CreatedAt
	}
	if res.Retain == "" {
		res.Retain = DefaultRetain(res.Origin)
	}
	if res.State == "" {
		res.State = StateActive
	}
	tagsJSON, _ := json.Marshal(res.Tags)
	if res.Tags == nil {
		tagsJSON = []byte("[]")
	}
	result, err := d.db.Exec(`INSERT INTO resources
		(kind, seq, name, ext, mime, content_hash, blob_path, size_bytes, width, height,
		 duration_ms, title, summary, tags, origin, retain, state, section_count, created_at, used_at, blob_owned, url)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		res.Kind, res.Seq, res.Name, res.Ext, res.MIME, res.ContentHash, res.BlobPath,
		res.SizeBytes, res.Width, res.Height, res.DurationMS, res.Title, res.Summary,
		string(tagsJSON), res.Origin, res.Retain, res.State, res.SectionCount,
		res.CreatedAt, res.UsedAt, boolToInt(res.BlobOwned), res.URL)
	if err != nil {
		// 回滚序号缓存，避免因写入失败造成序号空洞
		archiveSeqCache[res.Kind] = seq - 1
		// 内容哈希唯一索引冲突：并发下另一条写入已登记同一哈希，直接复用现有目录行
		if res.ContentHash != "" && strings.Contains(err.Error(), "UNIQUE constraint failed") {
			if existing, ok := d.FindResourceByHash(res.Kind, res.ContentHash); ok {
				*res = existing
				return nil
			}
		}
		return fmt.Errorf("写入资源目录失败: %v", err)
	}
	res.ID, _ = result.LastInsertId()
	return nil
}

// boolToInt SQLite 没有布尔类型，统一以 0/1 存取
func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

// archiveResourceColumns 查询列清单（与 scanResource 严格对齐）
const archiveResourceColumns = `id, kind, seq, name, ext, mime, content_hash, blob_path,
	size_bytes, width, height, duration_ms, title, summary, tags, origin, retain, state,
	section_count, created_at, used_at, blob_owned, url`

// scanResource 按 archiveResourceColumns 顺序扫描一行
func scanResource(scanner interface{ Scan(...any) error }) (Resource, error) {
	var res Resource
	var tagsJSON string
	var blobOwned int
	err := scanner.Scan(&res.ID, &res.Kind, &res.Seq, &res.Name, &res.Ext, &res.MIME,
		&res.ContentHash, &res.BlobPath, &res.SizeBytes, &res.Width, &res.Height,
		&res.DurationMS, &res.Title, &res.Summary, &tagsJSON, &res.Origin, &res.Retain,
		&res.State, &res.SectionCount, &res.CreatedAt, &res.UsedAt, &blobOwned, &res.URL)
	if err != nil {
		return res, err
	}
	res.BlobOwned = blobOwned != 0
	res.Handle = FormatHandle(res.Kind, res.Seq)
	res.Tags = []string{}
	if tagsJSON != "" {
		_ = json.Unmarshal([]byte(tagsJSON), &res.Tags)
	}
	return res, nil
}
