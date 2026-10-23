package persistence

// domain_db.go 领域数据库 —— 一个领域一个 SQLite 文件、一个连接句柄。
//
// 分域规则（见 docs/adr/0009-数据库按领域分文件.md）：
//
//	archive.db    统一资源名录   resources / resource_sections
//	media.db      媒体派生数据   video_summary（动图/视频/音频理解摘要）/ vision_cache
//	web.db        网络检索缓存   web_ltp_page_cache
//	text.db       文本内容索引   file_mapping（文件 → 分块集合）
//	character.db  自定义角色库   painter_characters
//	knowledge.db  通用知识库     用户/插件自建表（/knowledge/ 与 LTP9 database.*）
//
// 约束：
//   - 一张表只属于一个领域库，不再把内建缓存塞进 knowledge.db；
//   - 跨库不做外键，只按句柄 / 内容哈希 / URL 这类稳定键弱关联（迁移与删除由代码负责）；
//   - 打开策略为懒打开 + 幂等建表；配置路径变化时自动重开（测试与运行期改配置都安全）。

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"

	_ "github.com/mattn/go-sqlite3"
)

// domainDB 一个领域库的懒加载连接。
type domainDB struct {
	label      string        // 日志用中文名
	path       func() string // 运行期读取配置（测试可改 flag）
	schema     []string      // 幂等建表语句
	mu         sync.Mutex    // 串行化打开 / 重开 / 关闭
	handle     *sql.DB       // 已打开的连接
	openedPath string        // handle 对应的规整路径（检测配置变化）
}

// kvTable 生成 key/value 知识表建表语句（表名仅接受内部常量）。
func kvTable(name string) string {
	return fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
		key   TEXT PRIMARY KEY,
		value TEXT NOT NULL DEFAULT ''
	)`, name)
}

var (
	// mediaDatabase 媒体派生数据：媒体理解摘要 + 视觉副本簿记。
	mediaDatabase = &domainDB{
		label: "媒体派生",
		path:  func() string { return *config.MediaDBPath },
		schema: []string{
			kvTable("video_summary"),
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
		},
	}
	// webDatabase 网络检索缓存：页面摘要。
	webDatabase = &domainDB{
		label: "网络检索",
		path:  func() string { return *config.WebDBPath },
		schema: []string{
			`CREATE TABLE IF NOT EXISTS web_ltp_page_cache (
				url        TEXT PRIMARY KEY,
				title      TEXT NOT NULL DEFAULT '',
				domain     TEXT NOT NULL DEFAULT '',
				summary    TEXT NOT NULL DEFAULT '',
				query_hint TEXT NOT NULL DEFAULT '',
				fetched_at INTEGER NOT NULL
			)`,
		},
	}
	// textDatabase 文本内容索引：文件标识 → 分块集合。
	textDatabase = &domainDB{
		label:  "文本索引",
		path:   func() string { return *config.TextDBPath },
		schema: []string{kvTable("file_mapping")},
	}
	// characterDatabase 自定义角色库：绘图角色档案。
	characterDatabase = &domainDB{
		label:  "自定义角色",
		path:   func() string { return *config.CharacterDBPath },
		schema: []string{kvTable("painter_characters")},
	}
)

// open 打开（或复用）领域库连接，并确保表结构就绪。
func (d *domainDB) open() (*sql.DB, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	want := filepath.Clean(d.path())
	if d.handle != nil && d.openedPath == want {
		return d.handle, nil
	}
	if d.handle != nil {
		// 配置路径已变化：先释放旧连接，避免测试/多环境互相串库
		_ = d.handle.Close()
		d.handle = nil
	}

	if dir := filepath.Dir(want); dir != "." && dir != string(filepath.Separator) {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("创建%s数据库目录失败: %w", d.label, err)
		}
	}
	db, err := sql.Open("sqlite3", want+"?_busy_timeout=10000&_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("连接%s数据库失败: %w", d.label, err)
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(5 * time.Minute)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("测试%s数据库连接失败: %w", d.label, err)
	}
	for _, pragma := range []string{
		"PRAGMA synchronous = NORMAL",
		"PRAGMA foreign_keys = ON",
		"PRAGMA cache_size = 10000",
	} {
		if _, err := db.Exec(pragma); err != nil {
			logger.Error("Persistence", "%s数据库 PRAGMA 失败(%s): %v", d.label, pragma, err)
		}
	}
	for _, stmt := range d.schema {
		if _, err := db.Exec(stmt); err != nil {
			db.Close()
			return nil, fmt.Errorf("%s数据库建表失败: %w", d.label, err)
		}
	}
	d.handle = db
	d.openedPath = want
	logger.Info("Persistence", "%s数据库初始化完成: %s", d.label, want)
	return db, nil
}

// close 关闭连接（幂等）；下次 open 会重新建连。
func (d *domainDB) close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.handle == nil {
		return nil
	}
	err := d.handle.Close()
	d.handle = nil
	d.openedPath = ""
	return err
}

// EnsureWebDB 打开网络检索缓存库（web.db）；供 WebLTP 等上层模块复用同一连接与表结构。
func EnsureWebDB() (*sql.DB, error) { return webDatabase.open() }

// InitDomainDBs 启动时统一打开各领域库并建表（幂等）。
// 打开失败不阻断启动：调用方记录告警，具体读写路径再次按需懒打开。
func InitDomainDBs() error {
	var firstErr error
	for _, d := range []*domainDB{mediaDatabase, webDatabase, textDatabase, characterDatabase} {
		if _, err := d.open(); err != nil {
			logger.Warn("Persistence", "%s数据库初始化失败: %v", d.label, err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// CloseDomainDBs 关闭全部领域库连接（进程退出与测试清理使用）。
func CloseDomainDBs() error {
	var firstErr error
	for _, d := range []*domainDB{mediaDatabase, webDatabase, textDatabase, characterDatabase} {
		if err := d.close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// tableExists 判断库内是否存在指定表。
func tableExists(handle *sql.DB, table string) (bool, error) {
	if handle == nil {
		return false, nil
	}
	var n int
	err := handle.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// copyTableRows 把源库一张表的指定列按 INSERT OR IGNORE 复制到目标库（同列顺序）。
// 目标表须已存在；返回复制的行数。用于历史库的「按领域拆表」迁移，可重复执行；
// 目标库已有同主键行时保留目标值（迁移后新写入的数据优先于旧库残留）。
func copyTableRows(src *sql.DB, srcTable string, dst *sql.DB, dstTable string, columns []string) (int64, error) {
	if len(columns) == 0 {
		return 0, fmt.Errorf("复制 %s 失败: 列清单为空", srcTable)
	}
	colList := strings.Join(columns, ", ")
	rows, err := src.Query("SELECT " + colList + " FROM " + srcTable)
	if err != nil {
		return 0, fmt.Errorf("读取 %s 失败: %w", srcTable, err)
	}
	defer rows.Close()

	tx, err := dst.Begin()
	if err != nil {
		return 0, fmt.Errorf("开启目标库事务失败: %w", err)
	}
	defer tx.Rollback()

	placeholders := strings.TrimSuffix(strings.Repeat("?, ", len(columns)), ", ")
	stmt, err := tx.Prepare("INSERT OR IGNORE INTO " + dstTable + " (" + colList + ") VALUES (" + placeholders + ")")
	if err != nil {
		return 0, fmt.Errorf("准备写入 %s 失败: %w", dstTable, err)
	}
	defer stmt.Close()

	var copied int64
	for rows.Next() {
		values := make([]any, len(columns))
		ptrs := make([]any, len(columns))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return copied, fmt.Errorf("扫描 %s 失败: %w", srcTable, err)
		}
		if _, err := stmt.Exec(values...); err != nil {
			return copied, fmt.Errorf("写入 %s 失败: %w", dstTable, err)
		}
		copied++
	}
	if err := rows.Err(); err != nil {
		return copied, fmt.Errorf("遍历 %s 失败: %w", srcTable, err)
	}
	if err := tx.Commit(); err != nil {
		return copied, fmt.Errorf("提交 %s 迁移事务失败: %w", dstTable, err)
	}
	return copied, nil
}

// backupDatabase 迁移前把整库快照到 {LocalDir}/backup/database/。
// 使用 VACUUM INTO 获取一致快照（WAL 下也不会漏事务）；失败则中止迁移，避免无备份删表。
func backupDatabase(handle *sql.DB, dbPath, tag string) error {
	dir := filepath.Join(*config.LocalDir, "backup", "database")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("创建备份目录失败: %w", err)
	}
	base := strings.TrimSuffix(filepath.Base(dbPath), filepath.Ext(dbPath))
	target := filepath.Join(dir, fmt.Sprintf("%s.%s-%s.db", base, tag, time.Now().Format("20060102-150405")))
	if _, err := handle.Exec("VACUUM INTO ?", target); err != nil {
		return fmt.Errorf("备份数据库失败: %w", err)
	}
	logger.Info("Persistence", "迁移前已备份数据库: %s", target)
	return nil
}
