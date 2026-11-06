package library

// migrate.go 旧编辑记录的一次性迁移：把旧格式 JSON 导入 library.db。
//
// 旧版本把编辑状态只放在进程内存里，进程退出即丢失；库内已有记录时不做任何导入，
// 避免重复。导入成功后把原文件改名为 .migrated，保留可追溯性。

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// migrateLegacyEdits 在库为空时导入旧格式编辑状态 JSON。
func migrateLegacyEdits(conn *sql.DB) error {
	var count int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM ` + editTable).Scan(&count); err != nil {
		return fmt.Errorf("检查变更记录失败: %w", err)
	}
	if count > 0 {
		return nil
	}
	path := filepath.Join(filepath.Clean(config.LocalDir), filepath.FromSlash(legacyEditsFileRel))
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("读取旧编辑记录失败: %w", err)
	}
	var snapshot EditSnapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return fmt.Errorf("解析旧编辑记录失败: %w", err)
	}
	if err := importLegacySnapshot(conn, snapshot); err != nil {
		return err
	}
	markLegacyMigrated(path)
	logger.Info(Module, "旧编辑记录已导入变更库: %d 个文件", len(snapshot.Files))
	return nil
}

// importLegacySnapshot 把旧快照写入变更库（逐条分配新序号，记录按时间正序）。
func importLegacySnapshot(conn *sql.DB, snapshot EditSnapshot) error {
	tx, err := conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, file := range snapshot.Files {
		path := normalizeRel(file.Path)
		if path == "" || rootOf(path) == "" {
			continue
		}
		state := legacyStateOf(path, file)
		for _, record := range file.Records {
			seq, seqErr := nextSeq(tx)
			if seqErr != nil {
				return seqErr
			}
			at := time.Unix(record.At, 0)
			if record.At <= 0 {
				at = time.Now()
			}
			notice := EditNotice{
				Path:      path,
				Op:        record.Op,
				Added:     record.Added,
				Removed:   record.Removed,
				StartLine: record.StartLine,
				Lines:     record.Lines,
				Handle:    file.Handle,
				Failed:    file.Failed,
			}
			if err := insertRecord(tx, path, notice, at, seq); err != nil {
				return err
			}
			state.lastSeq = seq
		}
		if state.lastSeq == 0 {
			seq, seqErr := nextSeq(tx)
			if seqErr != nil {
				return seqErr
			}
			state.lastSeq = seq
		}
		if err := pruneRecords(tx, path); err != nil {
			return err
		}
		if err := saveState(tx, state); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// legacyStateOf 把旧文件状态还原成当前状态结构（运行状态由配色反推）。
func legacyStateOf(path string, file FileState) *FileState {
	state := &FileState{
		Path:   path,
		Status: file.Status,
		Op:     file.Op,
		Failed: file.Status == StatusError,
		Handle: file.Handle,
		At:     file.At,
	}
	if file.Status == StatusOK {
		state.hasRun = true
		state.runOK = true
	}
	if state.At <= 0 {
		state.At = time.Now().Unix()
	}
	return state
}

// markLegacyMigrated 把旧文件改名为 .migrated（同名时附加时间戳）。
func markLegacyMigrated(path string) {
	target := path + ".migrated"
	if _, err := os.Stat(target); err == nil {
		target = fmt.Sprintf("%s.%d", target, time.Now().Unix())
	}
	if err := os.Rename(path, target); err != nil {
		logger.Warn(Module, "旧编辑记录已导入，但原文件改名失败: %v", err)
	}
}
