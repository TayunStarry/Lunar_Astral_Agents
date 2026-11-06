package library

// db.go 资源库变更库（local_data/database/library.db）的连接、建表与保留策略。
//
// 数据库是文件操作变化记录的唯一存储：PublishEdit 直接写库，Snapshot / Edits
// 直接读库，进程内不再保留一份用于展示的内存状态。

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/dbopen"
)

// Init 打开（或复用）资源库变更库并完成建表、旧数据导入与保留策略清理。
func Init() error {
	_, err := ensureDB()
	return err
}

// Close 关闭变更库（进程退出与测试隔离使用）。
func Close() error {
	dbMu.Lock()
	defer dbMu.Unlock()
	if db == nil {
		return nil
	}
	err := db.Close()
	db = nil
	dbPath = ""
	return err
}

// dbFilePath 返回变更库的绝对路径。
func dbFilePath() string {
	return filepath.Join(filepath.Clean(config.LocalDir), filepath.FromSlash(dbDirRel), dbFileName)
}

// ensureDB 打开或复用变更库；LocalDir 变化时自动重开并重建表结构。
func ensureDB() (*sql.DB, error) {
	dbMu.Lock()
	defer dbMu.Unlock()

	want := dbFilePath()
	if db != nil && dbPath == want {
		return db, nil
	}
	if db != nil {
		_ = db.Close()
		db = nil
		dbPath = ""
	}
	opened, err := dbopen.Open(want, dbopen.Options{
		CreateDir:       true,
		MaxOpenConns:    4,
		MaxIdleConns:    2,
		ConnMaxLifetime: 5 * time.Minute,
	})
	if err != nil {
		if cause, isDir := dbopen.DirCause(err); isDir {
			return nil, fmt.Errorf("创建变更库目录失败: %w", cause)
		}
		return nil, fmt.Errorf("连接变更库失败: %w", err)
	}
	if err := opened.Ping(); err != nil {
		_ = opened.Close()
		return nil, fmt.Errorf("测试变更库连接失败: %w", err)
	}
	if err := initSchema(opened); err != nil {
		_ = opened.Close()
		return nil, err
	}
	if err := migrateLegacyEdits(opened); err != nil {
		logger.Warn(Module, "旧编辑记录导入失败（原文件保留，稍后可重试）: %v", err)
	}
	if err := cleanupStore(opened); err != nil {
		logger.Warn(Module, "变更记录保留策略清理失败: %v", err)
	}
	db = opened
	dbPath = want
	logger.Info(Module, "资源库变更库初始化完成: %s", want)
	return db, nil
}

// initSchema 建立变更库全部表与索引。
func initSchema(conn *sql.DB) error {
	for _, stmt := range librarySchema {
		if _, err := conn.Exec(stmt); err != nil {
			return fmt.Errorf("变更库建表失败: %w", err)
		}
	}
	return nil
}

// Clear 清空全部变更记录与文件状态（测试 / 数据导出使用）。
func Clear() {
	conn, err := ensureDB()
	if err != nil {
		logger.Warn(Module, "清空变更库失败: %v", err)
		return
	}
	tx, err := conn.Begin()
	if err != nil {
		logger.Warn(Module, "清空变更库失败: %v", err)
		return
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM ` + editTable); err != nil {
		logger.Warn(Module, "清空变更记录失败: %v", err)
		return
	}
	if _, err := tx.Exec(`DELETE FROM ` + fileStateTable); err != nil {
		logger.Warn(Module, "清空文件状态失败: %v", err)
		return
	}
	if _, err := tx.Exec(`INSERT INTO `+metaTable+` (key, value) VALUES (?, '0')
		ON CONFLICT(key) DO UPDATE SET value = '0'`, metaSeqKey); err != nil {
		logger.Warn(Module, "重置变更序号失败: %v", err)
		return
	}
	if err := tx.Commit(); err != nil {
		logger.Warn(Module, "清空变更库失败: %v", err)
		return
	}
	broadcastSync(0, "", "")
}

// cleanupStore 执行容量与时效保留策略：按天裁剪、按总量裁剪、清理无记录的空状态。
func cleanupStore(conn *sql.DB) error {
	cutoff := time.Now().AddDate(0, 0, -maxRecordAgeDays).Unix()
	if _, err := conn.Exec(`DELETE FROM `+editTable+` WHERE at < ?`, cutoff); err != nil {
		return fmt.Errorf("清理过期变更记录失败: %w", err)
	}
	if _, err := conn.Exec(`DELETE FROM `+editTable+` WHERE id NOT IN (
		SELECT id FROM `+editTable+` ORDER BY seq DESC LIMIT ?)`, maxTotalRecords); err != nil {
		return fmt.Errorf("清理超量变更记录失败: %w", err)
	}
	// 只有运行状态的条目（没有正文记录）保留；其余无记录的状态一并回收。
	if _, err := conn.Exec(`DELETE FROM ` + fileStateTable + ` WHERE has_run = 0 AND path NOT IN (
		SELECT DISTINCT path FROM ` + editTable + `)`); err != nil {
		return fmt.Errorf("清理空文件状态失败: %w", err)
	}
	return nil
}
