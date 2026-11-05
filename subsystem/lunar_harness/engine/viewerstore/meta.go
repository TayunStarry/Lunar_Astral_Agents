package viewerstore

// meta.go 内部状态与迁移标记的读写。

import "database/sql"

// MigrationDone 历史记忆是否已完成迁移。
func MigrationDone() bool {
	value, ok, err := LoadMeta(migrationMetaKey)
	return err == nil && ok && value == migrationDoneValue
}

// MarkMigrationDone 标记历史记忆迁移完成（幂等）。
func MarkMigrationDone() error {
	return SaveMeta(migrationMetaKey, migrationDoneValue)
}

// LoadMeta 读取一条内部状态。
func LoadMeta(key string) (string, bool, error) {
	db, err := openStore()
	if err != nil {
		return "", false, err
	}
	var value string
	err = db.QueryRow(`SELECT value FROM viewer_meta WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

// SaveMeta 写入一条内部状态（upsert）。
func SaveMeta(key, value string) error {
	db, err := openStore()
	if err != nil {
		return err
	}
	_, err = db.Exec(`INSERT INTO viewer_meta (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
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
