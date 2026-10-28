package knowledge

// filedb.go database 目录内任意 *.db 数据源（懒打开、不创建文件）。

import (
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// fileDB 打开（或复用）database 目录内指定 *.db 的连接（懒加载，不创建文件）。
// name 为相对 database 目录的文件路径去 .db（顶层如 "notes"，子目录如 "sub/foo"）；
// 逐段校验名称白名单、文件存在性与 SQLite 魔数，全部通过才打开。
func (d *Store) fileDB(name string) (*sql.DB, error) {
	if d == nil {
		store = &Store{}
		d = store
	}
	rel, err := relPath(name)
	if err != nil {
		return nil, err
	}
	d.fileDBsMu.Lock()
	defer d.fileDBsMu.Unlock()
	if db, ok := d.fileDBs[name]; ok {
		return db, nil
	}
	dbDir := filepath.Clean(filepath.Dir(config.KnowledgeDBPath))
	dbPath := filepath.Clean(filepath.Join(dbDir, rel+".db"))
	// 防穿越复核：最终路径必须仍位于 database 目录内
	if !strings.HasPrefix(dbPath, dbDir+string(os.PathSeparator)) {
		return nil, fmt.Errorf("非法数据库路径: %s", name)
	}
	if _, err := os.Stat(dbPath); err != nil {
		return nil, fmt.Errorf("数据库文件不存在: %s", dbPath)
	}
	// 验证 SQLite 魔数，拒绝误传非 SQLite 文件
	f, err := os.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("读取数据库失败: %v", err)
	}
	magic := make([]byte, 16)
	_, rerr := io.ReadFull(f, magic)
	f.Close()
	if rerr != nil {
		return nil, fmt.Errorf("读取数据库头失败: %s", dbPath)
	}
	if string(magic) != "SQLite format 3\x00" {
		return nil, fmt.Errorf("非有效的 SQLite 数据库: %s", dbPath)
	}
	db, err := sql.Open("sqlite3", dbPath+"?_busy_timeout=10000&_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("连接 SQLite 失败: %v", err)
	}
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(5 * time.Minute)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("测试 SQLite 连接失败: %v", err)
	}
	if d.fileDBs == nil {
		d.fileDBs = map[string]*sql.DB{}
	}
	d.fileDBs[name] = db
	logger.Info("FileManager", "数据源连接完成: %s", dbPath)
	return db, nil
}

// relPath 校验并规整相对 database 目录的库文件路径（不含 .db 后缀）。
// 仅允许「段/段/…」形式，每段为字母数字-_，返回以 / 分隔的规整路径。
func relPath(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("数据库名不能为空")
	}
	segs := strings.Split(filepath.ToSlash(name), "/")
	for _, seg := range segs {
		if !knowledgeValidName.MatchString(seg) {
			return "", fmt.Errorf("非法数据库名: %q（仅允许字母数字-_）", name)
		}
	}
	return strings.Join(segs, "/"), nil
}
