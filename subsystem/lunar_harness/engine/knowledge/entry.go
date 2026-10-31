package knowledge

// entry.go 知识条目通道（月华 knowledge 存储复用）与表定位。
//
// 上层按「逻辑表名」读写二元组条目；内建表按 knowledgeBuiltinTables 路由到各自领域库，
// 其余落通用知识库 knowledge.db。

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// LoadEntries 读取指定逻辑表的全部条目（月华 knowledge 存储通道）。
// 条目格式 [key, text][]，按 rowid 稳定排序；表不存在时返回空数组。
// 内建表按 knowledgeBuiltinTables 路由到 media / text / character 库，其余落 knowledge.db。
func LoadEntries(table string) ([][]string, error) {
	if !knowledgeValidName.MatchString(table) {
		return nil, fmt.Errorf("无效的知识库表名: %q", table)
	}
	target, err := tableTarget(table)
	if err != nil {
		return nil, err
	}
	rows, err := target.Query("SELECT key, value FROM " + table + " ORDER BY rowid")
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return [][]string{}, nil
		}
		return nil, err
	}
	defer rows.Close()
	entries := [][]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		entries = append(entries, []string{k, v})
	}
	return entries, rows.Err()
}

// Value 读取指定逻辑表中某个 key 的值（不存在返回空串）。
// 仅供内部按已知键回查（如视频摘要按 URL / BV 号），不暴露给模型的任意查询通道。
func Value(table, key string) string {
	if !knowledgeValidName.MatchString(table) || strings.TrimSpace(key) == "" {
		return ""
	}
	target, err := tableTarget(table)
	if err != nil {
		return ""
	}
	var value string
	if err := target.QueryRow("SELECT value FROM "+table+" WHERE key = ?", key).Scan(&value); err != nil {
		return ""
	}
	return value
}

// ReplaceEntries 以「全表替换」语义写入指定逻辑表（月华 knowledge 存储通道）。
// 事务内：建表（幂等）→ 清空 → 逐条插入；任一步失败整体回滚。
func ReplaceEntries(table string, entries [][]string) error {
	if !knowledgeValidName.MatchString(table) {
		return fmt.Errorf("无效的知识库表名: %q", table)
	}
	target, err := tableTarget(table)
	if err != nil {
		return err
	}
	tx, err := target.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec("CREATE TABLE IF NOT EXISTS " + table + " (key TEXT PRIMARY KEY, value TEXT)"); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.Exec("DELETE FROM " + table); err != nil {
		_ = tx.Rollback()
		return err
	}
	stmt, err := tx.Prepare("INSERT OR REPLACE INTO " + table + " (key, value) VALUES (?, ?)")
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	defer stmt.Close()
	for _, pair := range entries {
		if len(pair) == 0 {
			continue
		}
		key := pair[0]
		val := ""
		if len(pair) >= 2 {
			val = pair[1]
		}
		if _, err := stmt.Exec(key, val); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// tableTarget 返回逻辑表实际落库的连接。
func tableTarget(table string) (*sql.DB, error) {
	if domain, ok := knowledgeBuiltinTables[table]; ok {
		return domain.open()
	}
	if err := ensure(); err != nil {
		return nil, err
	}
	return store.db, nil
}

// TableExists 判断库内是否存在指定表。
func TableExists(handle *sql.DB, table string) (bool, error) {
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

// CopyTableRows 把源库一张表的指定列按 INSERT OR IGNORE 复制到目标库（同列顺序）。
// 目标表须已存在；返回复制的行数。用于历史库的「按领域拆表」迁移，可重复执行；
// 目标库已有同主键行时保留目标值（迁移后新写入的数据优先于旧库残留）。
func CopyTableRows(src *sql.DB, srcTable string, dst *sql.DB, dstTable string, columns []string) (int64, error) {
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

// BackupDatabase 迁移前把整库快照到 {LocalDir}/backup/database/。
// 使用 VACUUM INTO 获取一致快照（WAL 下也不会漏事务）；失败则中止迁移，避免无备份删表。
func BackupDatabase(handle *sql.DB, dbPath, tag string) error {
	dir := filepath.Join(config.LocalDir, "backup", "database")
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
