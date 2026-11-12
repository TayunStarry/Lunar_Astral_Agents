package viewerstore

// json_import.go 旧版 JSON（inbox.json / seen.json）的一次性导入。
//
// 触发条件：viewer_meta 中没有对应标记。导入在单个事务内完成，
// 逐条 INSERT ... ON CONFLICT DO NOTHING，因此重复执行不会产生重复行；
// 解析失败则回滚且不写标记，下次打开重试，错误里带文件名与字节偏移便于定位。
//
// 导入阶段不做 TTL 裁剪：历史判定口径必须原样保留，过期清理交给读写时的既有语义。

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// importLegacyJSON 依次导入两份历史 JSON；单份失败不阻断另一份，返回首个错误。
func importLegacyJSON(db *sql.DB) error {
	specs := []legacyImportSpec{
		{
			File:       inboxSeenJSONFile,
			Field:      "keys",
			Table:      "inbox_seen",
			KeyColumn:  "key",
			TimeColumn: "seen_at",
			Mark:       inboxSeenImportMark,
		},
		{
			File:       seenVideoJSONFile,
			Field:      "videos",
			Table:      "seen_video",
			KeyColumn:  "bv",
			TimeColumn: "watched_at",
			Mark:       seenVideoImportMark,
		},
	}
	var firstErr error
	for _, spec := range specs {
		if err := importLegacyFile(db, spec); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// importLegacyFile 导入一份历史 JSON：文件缺失写 absent 标记，成功导入后写对账标记。
func importLegacyFile(db *sql.DB, spec legacyImportSpec) error {
	done, err := legacyImportDone(db, spec.Mark)
	if err != nil || done {
		return err
	}
	path := filepath.Join(DataDir(), spec.File)
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return writeLegacyImportMark(db, spec.Mark, legacyImportAbsent)
		}
		return fmt.Errorf("检查历史文件 %s 失败: %w", spec.File, err)
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("开始 %s 导入事务失败: %w", spec.File, err)
	}
	defer tx.Rollback()

	insertSQL := fmt.Sprintf(
		`INSERT INTO %s(%s, %s) VALUES(?, ?) ON CONFLICT(%s) DO NOTHING`,
		spec.Table, spec.KeyColumn, spec.TimeColumn, spec.KeyColumn)
	stmt, err := tx.Prepare(insertSQL)
	if err != nil {
		return fmt.Errorf("准备 %s 导入写入失败: %w", spec.File, err)
	}
	defer stmt.Close()

	result := legacyImportResult{}
	err = forEachJSONMapEntry(path, spec.Field, func(key string, at int64) error {
		if key == "" {
			result.Skipped++
			return nil
		}
		execResult, err := stmt.Exec(key, at)
		if err != nil {
			return fmt.Errorf("写入 %s 失败（键 %q）: %w", spec.Table, key, err)
		}
		affected, err := execResult.RowsAffected()
		if err != nil {
			return fmt.Errorf("读取 %s 导入结果失败（键 %q）: %w", spec.Table, key, err)
		}
		if affected == 0 {
			result.Skipped++
			return nil
		}
		result.Inserted++
		return nil
	})
	if err != nil {
		return err
	}

	mark := fmt.Sprintf("inserted=%d skipped=%d at=%d", result.Inserted, result.Skipped, time.Now().Unix())
	if err := writeLegacyImportMark(tx, spec.Mark, mark); err != nil {
		return fmt.Errorf("写入 %s 迁移标记失败: %w", spec.File, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交 %s 导入事务失败: %w", spec.File, err)
	}
	logger.Info("BilibiliViewer", "历史 JSON %s 导入完成：新增 %d 条、跳过 %d 条", spec.File, result.Inserted, result.Skipped)
	return nil
}

// legacyImportDone 查询迁移标记是否已存在。
func legacyImportDone(db *sql.DB, mark string) (bool, error) {
	var value string
	err := db.QueryRow(`SELECT value FROM viewer_meta WHERE key = ?`, mark).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("读取迁移标记 %s 失败: %w", mark, err)
	}
	return true, nil
}

// writeLegacyImportMark 写入/刷新迁移标记。
func writeLegacyImportMark(db sqlExecer, mark, value string) error {
	_, err := db.Exec(`INSERT INTO viewer_meta(key, value) VALUES(?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, mark, value)
	return err
}

// forEachJSONMapEntry 流式遍历 JSON 顶层对象的指定字段（键 → 整数），不整份读入内存。
func forEachJSONMapEntry(path, field string, fn func(key string, value int64) error) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("打开历史文件 %s 失败: %w", filepath.Base(path), err)
	}
	defer file.Close()

	dec := json.NewDecoder(file)
	name := filepath.Base(path)
	token, err := dec.Token()
	if err != nil {
		return earlyJSONError(name, "读取顶层对象", dec, err)
	}
	if delim, ok := token.(json.Delim); !ok || delim != '{' {
		return fmt.Errorf("%s 顶层不是 JSON 对象（第 %d 字节）", name, dec.InputOffset())
	}
	found := false
	for dec.More() {
		keyToken, err := dec.Token()
		if err != nil {
			return earlyJSONError(name, "读取字段名", dec, err)
		}
		key, ok := keyToken.(string)
		if !ok {
			return fmt.Errorf("%s 字段名不是字符串（第 %d 字节）", name, dec.InputOffset())
		}
		if key != field {
			var skip json.RawMessage
			if err := dec.Decode(&skip); err != nil {
				return earlyJSONError(name, "跳过字段 "+key, dec, err)
			}
			continue
		}
		found = true
		if err := decodeJSONIntMap(dec, name, field, fn); err != nil {
			return err
		}
	}
	if _, err := dec.Token(); err != nil {
		return earlyJSONError(name, "读取顶层对象结尾", dec, err)
	}
	if !found {
		return fmt.Errorf("%s 缺少 %q 字段（第 %d 字节）", name, field, dec.InputOffset())
	}
	return nil
}

// decodeJSONIntMap 遍历字段内的「字符串 → 整数」对象，逐个交给 fn。
func decodeJSONIntMap(dec *json.Decoder, name, field string, fn func(string, int64) error) error {
	token, err := dec.Token()
	if err != nil {
		return earlyJSONError(name, "读取字段 "+field, dec, err)
	}
	if delim, ok := token.(json.Delim); !ok || delim != '{' {
		return fmt.Errorf("%s 的 %q 字段不是 JSON 对象（第 %d 字节）", name, field, dec.InputOffset())
	}
	for dec.More() {
		keyToken, err := dec.Token()
		if err != nil {
			return earlyJSONError(name, "读取 "+field+" 的键", dec, err)
		}
		key, ok := keyToken.(string)
		if !ok {
			return fmt.Errorf("%s 的 %q 字段包含非字符串键（第 %d 字节）", name, field, dec.InputOffset())
		}
		var value int64
		if err := dec.Decode(&value); err != nil {
			return fmt.Errorf("%s 字段 %q 的值不是整数（第 %d 字节）: %w",
				name, key, dec.InputOffset(), err)
		}
		if err := fn(key, value); err != nil {
			return err
		}
	}
	if _, err := dec.Token(); err != nil {
		return earlyJSONError(name, "读取 "+field+" 字段结尾", dec, err)
	}
	return nil
}

// earlyJSONError 给解析错误补上文件名与当前字节偏移。
func earlyJSONError(name, where string, dec *json.Decoder, err error) error {
	return fmt.Errorf("%s 解析失败（%s，第 %d 字节）: %w", name, where, dec.InputOffset(), err)
}
