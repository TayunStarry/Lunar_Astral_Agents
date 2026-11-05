package viewerstore

// slot.go 选片口味 / 评论倾向两张单行槽的读写。

import (
	"database/sql"
	"fmt"
	"time"
)

// SaveProfile 覆盖写选片口味单行。
func SaveProfile(value string) error {
	return SaveProfileAt(value, time.Now().Unix())
}

// SaveProfileAt 按指定时间覆盖写选片口味单行。
func SaveProfileAt(value string, at int64) error {
	return saveSlot("viewer_profile", profileSlotKey, value, at)
}

// LoadProfile 读取选片口味单行。
func LoadProfile() (string, int64, bool, error) {
	return loadSlot("viewer_profile", profileSlotKey)
}

// SaveStance 覆盖写评论倾向单行。
func SaveStance(value string) error {
	return SaveStanceAt(value, time.Now().Unix())
}

// SaveStanceAt 按指定时间覆盖写评论倾向单行。
func SaveStanceAt(value string, at int64) error {
	return saveSlot("viewer_stance", stanceSlotKey, value, at)
}

// LoadStance 读取评论倾向单行。
func LoadStance() (string, int64, bool, error) {
	return loadSlot("viewer_stance", stanceSlotKey)
}

// saveSlot 固定 key 的单行覆盖写（upsert）。
func saveSlot(table, key, value string, at int64) error {
	if at == 0 {
		at = time.Now().Unix()
	}
	db, err := openStore()
	if err != nil {
		return err
	}
	_, err = db.Exec(`INSERT INTO `+table+` (slot, value, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(slot) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`,
		key, value, at)
	if err != nil {
		return fmt.Errorf("写入 %s 失败: %w", table, err)
	}
	return nil
}

// loadSlot 读取固定 key 的单行。
func loadSlot(table, key string) (string, int64, bool, error) {
	db, err := openStore()
	if err != nil {
		return "", 0, false, err
	}
	var value string
	var at int64
	err = db.QueryRow(`SELECT value, updated_at FROM `+table+` WHERE slot = ?`, key).Scan(&value, &at)
	if err == sql.ErrNoRows {
		return "", 0, false, nil
	}
	if err != nil {
		return "", 0, false, err
	}
	return value, at, true, nil
}
