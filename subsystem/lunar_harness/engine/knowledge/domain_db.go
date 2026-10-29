package knowledge

// domain_db.go 领域数据库 —— 一个领域一个 SQLite 文件、一个连接句柄。
//
// 分域规则（见 docs/adr/0009-数据库按领域分文件.md）：
//
//	text.db       文本内容索引   file_mapping（文件 → 分块集合）
//	character.db  自定义角色库   painter_characters
//	knowledge.db  通用知识库     用户/插件自建表（/knowledge/ 与 LTP9 database.*）
//	handles.db    统一句柄库     url_master / handle_* / doc_sections / cache（由 engine/resource 登记）
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
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// RegisterDomain 由各模块在初始化时登记自己的 SQLite 域库（name 为 ExecuteOn 的 db 参数）。
// open 在每次取用时调用，需返回已建表且可用的连接；重复登记同名域以最后一次为准。
func RegisterDomain(name string, open func() (*sql.DB, error)) {
	if name == "" || open == nil {
		return
	}
	domainMu.Lock()
	defer domainMu.Unlock()
	domainOpeners[name] = open
}

// unregisterDomain 注销域库登记（测试清理使用）。
func unregisterDomain(name string) {
	domainMu.Lock()
	defer domainMu.Unlock()
	delete(domainOpeners, name)
}

// domainOpener 取已登记的域库打开函数。
func domainOpener(name string) (func() (*sql.DB, error), bool) {
	domainMu.RLock()
	defer domainMu.RUnlock()
	open, ok := domainOpeners[name]
	return open, ok
}

// Open 按数据源名取得连接：
//   - "" / "knowledge" → 通用知识库 knowledge.db；
//   - media / web / text / character（内建）与 archive（由 persistence 登记）→ 对应域库；
//   - 其他取值 → database 目录内的 *.db 文件（懒打开，不创建新文件）。
func Open(name string) (*sql.DB, error) {
	switch name {
	case "", "knowledge":
		if err := ensure(); err != nil {
			return nil, err
		}
		return store.db, nil
	}
	if open, ok := domainOpener(name); ok {
		return open()
	}
	if reservedDomains[name] {
		return nil, fmt.Errorf("数据库域未注册: %s", name)
	}
	return store.fileDB(name)
}

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

// Init 启动时统一打开各领域库并建表（幂等）。
// 打开失败不阻断启动：调用方记录告警，具体读写路径再次按需懒打开。
func Init() error {
	var firstErr error
	for _, d := range domains {
		if _, err := d.open(); err != nil {
			logger.Warn("Persistence", "%s数据库初始化失败: %v", d.label, err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// Close 关闭全部知识库连接（通用库 + 任意 *.db 缓存 + 领域库；进程退出与测试清理使用）。
func Close() error {
	firstErr := store.Close()
	for _, d := range domains {
		if err := d.close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
