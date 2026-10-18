package module

// archive_store.go 资源目录（SQLite archive.db）—— 三层存储的第一层
//
// 职责：句柄分配与解析、资源元数据、段落结构表、跨资源搜索。
// 明确不存：正文内容（在内容仓库里）、向量（在记忆库里）。
//
// 序号分配规则（ADR「双身份」）：序号是持久化主键，永不复用。
// 进程启动时从 resources 表重建计数器（_meta 表记录 next_seq），
// 因此 @I.007 在重启后仍指向同一资源。

import (
	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/LoggerGeneral"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// ArchiveResource 资源目录中的一行（模型与前端可见的资源视图）
type ArchiveResource struct {
	ID           int64    `json:"id"`
	Kind         string   `json:"kind"`  // 类型码 D/I/V/A/T
	Seq          int64    `json:"seq"`   // 资源序号（句柄主体）
	Handle       string   `json:"handle"`// 规范句柄文本，形如 @D.003
	Name         string   `json:"name"`  // 原始文件名（可空）
	Ext          string   `json:"ext"`   // 小写扩展名（含点，可空）
	MIME         string   `json:"mime"`
	ContentHash  string   `json:"content_hash"` // sha16（SHA-256 前 16 位十六进制）
	BlobPath     string   `json:"blob_path"`    // 相对 LocalDir 的物理路径（内容仓库位置）
	SizeBytes    int64    `json:"size_bytes"`
	Width        int      `json:"width,omitempty"`
	Height       int      `json:"height,omitempty"`
	DurationMS   int64    `json:"duration_ms,omitempty"`
	Title        string   `json:"title"`     // 标题（文档取首个标题或文件名）
	Summary      string   `json:"summary"`   // 摘要/描述（图片为视觉描述，文档可空）
	Tags         []string `json:"tags"`
	Origin       string   `json:"origin"`        // 来源
	Retain       string   `json:"retain"`        // 保留等级
	State        string   `json:"state"`         // blob 状态 active/lost
	SectionCount int      `json:"section_count"` // 段落数（文档）
	CreatedAt    int64    `json:"created_at"`
	UsedAt       int64    `json:"used_at"`
	// BlobOwned 实体是否为「归档私有副本」：true = 归档自己写入、配额回收可清理；
	// false = 前端/引擎写入的实体，回收器不得删除（防止误删用户可见文件）
	BlobOwned bool `json:"blob_owned"`
}

// ArchiveSection 段落结构表的一行（文档的大纲节点）
type ArchiveSection struct {
	ID        int64  `json:"id"`
	Resource  int64  `json:"resource_id"`
	Ordinal   int    `json:"ordinal"`    // 章节序号（句柄 |§n 使用）
	Level     int    `json:"level"`      // 标题层级 1..6
	Title     string `json:"title"`      // 标题文本
	Path      string `json:"path"`       // 层级路径（如「一、功能 / 1.2 用法」）
	LineStart int    `json:"line_start"` // 正文起始行（1 基）
	LineEnd   int    `json:"line_end"`   // 正文结束行（含）
	Chars     int    `json:"chars"`      // 正文字数
}

// ArchiveSectionDraft 写入段落表时的草稿（不含自增 ID 与资源 ID）
type ArchiveSectionDraft struct {
	Ordinal   int
	Level     int
	Title     string
	Path      string
	LineStart int
	LineEnd   int
	Chars     int
}

// ArchiveSearchHit 跨资源搜索命中
type ArchiveSearchHit struct {
	Handle    string `json:"handle"`
	Kind      string `json:"kind"`
	Title     string `json:"title"`
	Summary   string `json:"summary"`
	Sections  []ArchiveSection `json:"sections,omitempty"` // 命中章节（文档）
	MatchedIn string `json:"matched_in"`                   // title / summary / tag / section / name
}

// ArchiveStats 目录统计（GC 与运维视图）
type ArchiveStats struct {
	Total     int64            `json:"total"`
	ByKind    map[string]int64 `json:"by_kind"`
	ByState   map[string]int64 `json:"by_state"`
	ByRetain  map[string]int64 `json:"by_retain"`
	Bytes     int64            `json:"bytes"`
}

var (
	// archiveMu 串行化序号分配与目录写入，避免并发下句柄重号
	archiveMu sync.Mutex
	// archiveSeqCache 各类型码的下一个可用序号缓存（启动时从 _meta 重建）
	archiveSeqCache = map[string]int64{}
	// archiveSeqLoaded 序号缓存是否已从库中重建
	archiveSeqLoaded bool
)

// InitArchiveDB 初始化资源目录（SQLite），幂等
func InitArchiveDB(dbPath string) error {
	if ArchiveDatabase != nil && ArchiveDatabase.initialized {
		return nil
	}
	db := &ArchiveDB{}
	if err := db.init(dbPath); err != nil {
		return err
	}
	ArchiveDatabase = db
	LoggerGeneral.Info("FileManager", "资源目录初始化完成: %s", dbPath)
	return nil
}

// EnsureArchiveInitialized 确保资源目录可用，必要时按默认路径初始化
func EnsureArchiveInitialized() error {
	if ArchiveDatabase != nil && ArchiveDatabase.initialized && ArchiveDatabase.db != nil {
		return nil
	}
	return InitArchiveDB(*GeneralConfig.ArchiveDBPath)
}

func (d *ArchiveDB) init(dbPath string) error {
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
			LoggerGeneral.Error("FileManager", "资源目录 PRAGMA 失败(%s): %v", pragma, err)
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

// ensureSchema 建表（幂等）并初始化序号元数据
func (d *ArchiveDB) ensureSchema() error {
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
			blob_owned    INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_resources_handle ON resources(kind, seq)`,
		`CREATE INDEX IF NOT EXISTS idx_resources_hash ON resources(content_hash)`,
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
		`CREATE TABLE IF NOT EXISTS _meta (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL DEFAULT ''
		)`,
		// 视觉读取副本缓存表（/file/read-multimodal 缩放副本的簿记；副本本体在 multimedia/vision/）
		// used_at 驱动 LRU 淘汰（容量超限时按最久未用清理，副本为派生数据可随时重建）
		`CREATE TABLE IF NOT EXISTS vision_cache (
			content_hash TEXT NOT NULL,
			side         INTEGER NOT NULL,
			tag          TEXT NOT NULL DEFAULT '',
			cache_path   TEXT NOT NULL DEFAULT '',
			bytes        INTEGER NOT NULL DEFAULT 0,
			created_at   INTEGER NOT NULL DEFAULT 0,
			used_at      INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (content_hash, side)
		)`,
	}
	for _, stmt := range stmts {
		if _, err := d.db.Exec(stmt); err != nil {
			return fmt.Errorf("资源目录建表失败: %v", err)
		}
	}
	// 兼容既有库：老版本没有 blob_owned 列，缺失时补上（ALTER 失败仅代表列已存在）
	if _, err := d.db.Exec(`ALTER TABLE resources ADD COLUMN blob_owned INTEGER NOT NULL DEFAULT 0`); err != nil {
		// 列已存在是正常情况，不视为错误
		_ = err
	}
	// 兼容既有库：老版本 vision_cache 没有 used_at 列（LRU 淘汰依据），缺失时补上
	if _, err := d.db.Exec(`ALTER TABLE vision_cache ADD COLUMN used_at INTEGER NOT NULL DEFAULT 0`); err != nil {
		_ = err
	}
	return nil
}

// loadSeqCache 从库中重建各类型码的下一可用序号（序号永不复用）
func (d *ArchiveDB) loadSeqCache() error {
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

// Close 关闭资源目录连接（进程退出与测试清理使用）
func (d *ArchiveDB) Close() error {
	if d == nil || d.db == nil {
		return nil
	}
	err := d.db.Close()
	d.db = nil
	d.initialized = false
	return err
}

// CloseArchiveDB 关闭全局资源目录实例（幂等）
func CloseArchiveDB() error {
	if ArchiveDatabase == nil {
		return nil
	}
	err := ArchiveDatabase.Close()
	ArchiveDatabase = nil
	return err
}

// nextSeqLocked 分配下一个序号（调用方须持有 archiveMu）
func (d *ArchiveDB) nextSeqLocked(kind string) (int64, error) {
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
func (d *ArchiveDB) InsertResource(res *ArchiveResource) error {
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
		 duration_ms, title, summary, tags, origin, retain, state, section_count, created_at, used_at, blob_owned)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		res.Kind, res.Seq, res.Name, res.Ext, res.MIME, res.ContentHash, res.BlobPath,
		res.SizeBytes, res.Width, res.Height, res.DurationMS, res.Title, res.Summary,
		string(tagsJSON), res.Origin, res.Retain, res.State, res.SectionCount,
		res.CreatedAt, res.UsedAt, boolToInt(res.BlobOwned))
	if err != nil {
		// 回滚序号缓存，避免因写入失败造成序号空洞
		archiveSeqCache[res.Kind] = seq - 1
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
	section_count, created_at, used_at, blob_owned`

// scanResource 按 archiveResourceColumns 顺序扫描一行
func scanResource(scanner interface{ Scan(...any) error }) (ArchiveResource, error) {
	var res ArchiveResource
	var tagsJSON string
	var blobOwned int
	err := scanner.Scan(&res.ID, &res.Kind, &res.Seq, &res.Name, &res.Ext, &res.MIME,
		&res.ContentHash, &res.BlobPath, &res.SizeBytes, &res.Width, &res.Height,
		&res.DurationMS, &res.Title, &res.Summary, &tagsJSON, &res.Origin, &res.Retain,
		&res.State, &res.SectionCount, &res.CreatedAt, &res.UsedAt, &blobOwned)
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

// GetResourceByHandle 按句柄解析资源（句柄不存在时返回 false）
func (d *ArchiveDB) GetResourceByHandle(kind string, seq int64) (ArchiveResource, bool) {
	row := d.db.QueryRow(`SELECT `+archiveResourceColumns+` FROM resources WHERE kind = ? AND seq = ?`, kind, seq)
	res, err := scanResource(row)
	if err != nil {
		return ArchiveResource{}, false
	}
	return res, true
}

// GetResourceByID 按自增 ID 解析资源
func (d *ArchiveDB) GetResourceByID(id int64) (ArchiveResource, bool) {
	row := d.db.QueryRow(`SELECT `+archiveResourceColumns+` FROM resources WHERE id = ?`, id)
	res, err := scanResource(row)
	if err != nil {
		return ArchiveResource{}, false
	}
	return res, true
}

// FindResourceByBlobPath 按实体路径查找既有目录行（用于沿用实体归属）
func (d *ArchiveDB) FindResourceByBlobPath(blobPath string) (ArchiveResource, bool) {
	if blobPath == "" {
		return ArchiveResource{}, false
	}
	row := d.db.QueryRow(`SELECT `+archiveResourceColumns+
		` FROM resources WHERE blob_path = ? ORDER BY id ASC LIMIT 1`, blobPath)
	res, err := scanResource(row)
	if err != nil {
		return ArchiveResource{}, false
	}
	return res, true
}

// FindResourceByHash 按类型码与内容哈希查找既有资源（用于同名同内容复用）
func (d *ArchiveDB) FindResourceByHash(kind, hash string) (ArchiveResource, bool) {
	if hash == "" {
		return ArchiveResource{}, false
	}
	row := d.db.QueryRow(`SELECT `+archiveResourceColumns+
		` FROM resources WHERE kind = ? AND content_hash = ? ORDER BY id DESC LIMIT 1`, kind, hash)
	res, err := scanResource(row)
	if err != nil {
		return ArchiveResource{}, false
	}
	return res, true
}

// ListResources 列出资源（kind 为空表示全部），按 id 倒序（越靠前越新）
func (d *ArchiveDB) ListResources(kind string, limit int) ([]ArchiveResource, error) {
	if limit <= 0 {
		limit = 50
	}
	query := `SELECT ` + archiveResourceColumns + ` FROM resources`
	args := []any{}
	if kind != "" {
		query += ` WHERE kind = ?`
		args = append(args, kind)
	}
	query += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ArchiveResource{}
	for rows.Next() {
		res, err := scanResource(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, res)
	}
	return out, rows.Err()
}

// ListSections 读取资源的段落表（按章节序号升序）
func (d *ArchiveDB) ListSections(resourceID int64) ([]ArchiveSection, error) {
	rows, err := d.db.Query(`SELECT id, resource_id, ordinal, level, title, path,
		line_start, line_end, chars FROM resource_sections WHERE resource_id = ? ORDER BY ordinal`, resourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ArchiveSection{}
	for rows.Next() {
		var s ArchiveSection
		if err := rows.Scan(&s.ID, &s.Resource, &s.Ordinal, &s.Level, &s.Title, &s.Path,
			&s.LineStart, &s.LineEnd, &s.Chars); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ReplaceSections 全量替换资源的段落表（重导 / 迁移重建时使用）
func (d *ArchiveDB) ReplaceSections(resourceID int64, drafts []ArchiveSectionDraft) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM resource_sections WHERE resource_id = ?`, resourceID); err != nil {
		_ = tx.Rollback()
		return err
	}
	stmt, err := tx.Prepare(`INSERT INTO resource_sections
		(resource_id, ordinal, level, title, path, line_start, line_end, chars) VALUES (?,?,?,?,?,?,?,?)`)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	defer stmt.Close()
	for _, s := range drafts {
		if _, err := stmt.Exec(resourceID, s.Ordinal, s.Level, s.Title, s.Path, s.LineStart, s.LineEnd, s.Chars); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	if _, err := tx.Exec(`UPDATE resources SET section_count = ? WHERE id = ?`, len(drafts), resourceID); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// UpdateResourceExtras 更新资源的摘要/标题/标签（描述生成与迁移回填使用）
func (d *ArchiveDB) UpdateResourceExtras(id int64, title, summary string, tags []string) error {
	archiveMu.Lock()
	defer archiveMu.Unlock()
	tagsJSON, _ := json.Marshal(tags)
	if tags == nil {
		tagsJSON = []byte("[]")
	}
	_, err := d.db.Exec(`UPDATE resources SET title = ?, summary = ?, tags = ? WHERE id = ?`,
		title, summary, string(tagsJSON), id)
	return err
}

// UpdateResourceMeta 更新资源的尺寸/时长等探测元数据
func (d *ArchiveDB) UpdateResourceMeta(id int64, width, height int, durationMS int64) error {
	archiveMu.Lock()
	defer archiveMu.Unlock()
	_, err := d.db.Exec(`UPDATE resources SET width = ?, height = ?, duration_ms = ? WHERE id = ?`,
		width, height, durationMS, id)
	return err
}

// RefreshResourceContent 刷新资源的正文元数据（文本类按原文件名稳定复用的重复入库）
//
// 只更新与文件内容相关的列，保留标题、摘要、标签等人工/模型产物。
func (d *ArchiveDB) RefreshResourceContent(id int64, name, ext, mime, hash string, size int64) error {
	archiveMu.Lock()
	defer archiveMu.Unlock()
	_, err := d.db.Exec(`UPDATE resources SET name = ?, ext = ?, mime = ?, content_hash = ?,
		size_bytes = ?, state = ?, used_at = ? WHERE id = ?`,
		name, ext, mime, hash, size, StateActive, time.Now().Unix(), id)
	return err
}

// TouchResource 记录资源最近被取用（GC 的 LRU 依据）
func (d *ArchiveDB) TouchResource(id int64) {
	archiveMu.Lock()
	defer archiveMu.Unlock()
	_, _ = d.db.Exec(`UPDATE resources SET used_at = ? WHERE id = ?`, time.Now().Unix(), id)
}

// MarkState 更新资源 blob 状态（回收后置 lost，句柄保留）
func (d *ArchiveDB) MarkState(id int64, state string) error {
	archiveMu.Lock()
	defer archiveMu.Unlock()
	_, err := d.db.Exec(`UPDATE resources SET state = ? WHERE id = ?`, state, id)
	return err
}

// DeleteResource 显式删除资源目录行与其段落（用户主动删除时使用）
func (d *ArchiveDB) DeleteResource(id int64) error {
	archiveMu.Lock()
	defer archiveMu.Unlock()
	if _, err := d.db.Exec(`DELETE FROM resource_sections WHERE resource_id = ?`, id); err != nil {
		return err
	}
	_, err := d.db.Exec(`DELETE FROM resources WHERE id = ?`, id)
	return err
}

// SearchResources 跨资源目录搜索：标题 / 摘要 / 标签 / 文件名 / 段落标题 子串匹配
//
// 说明：刻意不使用 FTS5 —— 中文无空格分词在 unicode61 下切分不可用，
// 且资源目录规模有限（千级），子串匹配的召回率与确定性优于虚表。
func (d *ArchiveDB) SearchResources(query string, kind string, limit int) ([]ArchiveSearchHit, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("搜索关键词不能为空")
	}
	if limit <= 0 {
		limit = 10
	}
	like := "%" + query + "%"
	cond := `(title LIKE ? OR summary LIKE ? OR tags LIKE ? OR name LIKE ?)`
	args := []any{like, like, like, like}
	sqlText := `SELECT ` + archiveResourceColumns + ` FROM resources WHERE ` + cond
	if kind != "" {
		sqlText += ` AND kind = ?`
		args = append(args, kind)
	}
	sqlText += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := d.db.Query(sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	hits := []ArchiveSearchHit{}
	seen := map[int64]bool{}
	for rows.Next() {
		res, err := scanResource(rows)
		if err != nil {
			return nil, err
		}
		seen[res.ID] = true
		hits = append(hits, ArchiveSearchHit{
			Handle: res.Handle, Kind: res.Kind, Title: displayTitle(res),
			Summary: res.Summary, MatchedIn: "card",
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// 段落标题命中：文档按章节补齐（同一资源聚合到一条命中上）
	secSQL := `SELECT s.resource_id, s.id, s.resource_id, s.ordinal, s.level, s.title, s.path,
		s.line_start, s.line_end, s.chars FROM resource_sections s WHERE s.title LIKE ? OR s.path LIKE ?`
	secArgs := []any{like, like}
	if kind != "" {
		secSQL += ` AND s.resource_id IN (SELECT id FROM resources WHERE kind = ?)`
		secArgs = append(secArgs, kind)
	}
	secSQL += ` ORDER BY s.resource_id DESC, s.ordinal LIMIT ?`
	secArgs = append(secArgs, limit*4)
	secRows, err := d.db.Query(secSQL, secArgs...)
	if err != nil {
		return hits, nil
	}
	defer secRows.Close()
	byResource := map[int64][]ArchiveSection{}
	order := []int64{}
	for secRows.Next() {
		var resID int64
		var s ArchiveSection
		if err := secRows.Scan(&resID, &s.ID, &s.Resource, &s.Ordinal, &s.Level, &s.Title,
			&s.Path, &s.LineStart, &s.LineEnd, &s.Chars); err != nil {
			continue
		}
		if !seen[resID] {
			seen[resID] = true
			order = append(order, resID)
		}
		byResource[resID] = append(byResource[resID], s)
	}
	for _, resID := range order {
		res, ok := d.GetResourceByID(resID)
		if !ok {
			continue
		}
		hits = append(hits, ArchiveSearchHit{
			Handle: res.Handle, Kind: res.Kind, Title: displayTitle(res),
			Summary: res.Summary, Sections: byResource[resID], MatchedIn: "section",
		})
	}
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits, nil
}

// displayTitle 资源的展示标题（标题为空时退回文件名，再退回句柄）
func displayTitle(res ArchiveResource) string {
	if res.Title != "" {
		return res.Title
	}
	if res.Name != "" {
		return res.Name
	}
	return res.Handle
}

// totalActiveBytes 统计 state=active 的资源总体积（GC 配额判据）
func (d *ArchiveDB) totalActiveBytes() (int64, error) {
	var total int64
	err := d.db.QueryRow(`SELECT COALESCE(SUM(size_bytes),0) FROM resources WHERE state = ?`, StateActive).Scan(&total)
	return total, err
}

// collectCandidates 取指定保留等级的回收候选：state=active，按 used_at 升序（最久未用优先）
func (d *ArchiveDB) collectCandidates(retain string, limit int) ([]ArchiveResource, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := d.db.Query(`SELECT `+archiveResourceColumns+
		` FROM resources WHERE state = ? AND retain = ? ORDER BY used_at ASC, id ASC LIMIT ?`,
		StateActive, retain, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ArchiveResource{}
	for rows.Next() {
		res, err := scanResource(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, res)
	}
	return out, rows.Err()
}

// blobReferencedByOthers 判断同一实体是否仍被其它目录行引用（共享去重保护）
func (d *ArchiveDB) blobReferencedByOthers(blobPath string, selfID int64) (bool, error) {
	if blobPath == "" {
		return false, nil
	}
	var count int64
	err := d.db.QueryRow(`SELECT COUNT(*) FROM resources WHERE blob_path = ? AND id <> ?`,
		blobPath, selfID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// blobReferencedByUnowned 判断同一实体是否被「非归档所有」的目录行引用
//
// 内容寻址下同内容共用实体：若任一行标记该实体归前端/引擎所有（blob_owned=0），
// 那么它就是用户可见文件，回收器一律不得删除 —— 归属按最保守的一侧处理。
func (d *ArchiveDB) blobReferencedByUnowned(blobPath string, selfID int64) (bool, error) {
	if blobPath == "" {
		return false, nil
	}
	var count int64
	err := d.db.QueryRow(`SELECT COUNT(*) FROM resources WHERE blob_path = ? AND id <> ? AND blob_owned = 0`,
		blobPath, selfID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// Stats 目录统计
func (d *ArchiveDB) Stats() (ArchiveStats, error) {
	stats := ArchiveStats{ByKind: map[string]int64{}, ByState: map[string]int64{}, ByRetain: map[string]int64{}}
	if err := d.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(size_bytes),0) FROM resources`).
		Scan(&stats.Total, &stats.Bytes); err != nil {
		return stats, err
	}
	for _, group := range []struct {
		column string
		target map[string]int64
	}{
		{"kind", stats.ByKind},
		{"state", stats.ByState},
		{"retain", stats.ByRetain},
	} {
		rows, err := d.db.Query(`SELECT ` + group.column + `, COUNT(*) FROM resources GROUP BY ` + group.column)
		if err != nil {
			continue
		}
		for rows.Next() {
			var key string
			var count int64
			if err := rows.Scan(&key, &count); err == nil {
				group.target[key] = count
			}
		}
		rows.Close()
	}
	return stats, nil
}
