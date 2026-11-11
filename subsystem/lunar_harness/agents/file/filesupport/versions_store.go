package filesupport

// versions_store.go 版本库的 SQLite 存储：建表、导入旧目录、裁剪保留策略。
//
// 版本内容以 BLOB 存在 local_data/database/versions.db，记录与内容同事务写入，
// 不会再出现「有副本没记录」或「有记录没副本」的半截状态。

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/dbopen"
)

// Close 关闭版本库连接（进程退出与测试隔离使用）。
func Close() error {
	versionMu.Lock()
	defer versionMu.Unlock()
	if versionDB == nil {
		return nil
	}
	err := versionDB.Close()
	versionDB = nil
	versionDBPath = ""
	return err
}

// versionDBFilePath 返回版本库绝对路径。
func versionDBFilePath() string {
	return filepath.Join(filepath.Clean(config.LocalDir), versionDBDirRel, versionDBFileName)
}

// ensureVersionDB 打开或复用版本库；LocalDir 变化时自动重开。
func ensureVersionDB() (*sql.DB, error) {
	versionMu.Lock()
	defer versionMu.Unlock()

	want := versionDBFilePath()
	if versionDB != nil && versionDBPath == want {
		return versionDB, nil
	}
	if versionDB != nil {
		_ = versionDB.Close()
		versionDB = nil
		versionDBPath = ""
	}
	opened, err := dbopen.Open(want, dbopen.Options{
		CreateDir:       true,
		MaxOpenConns:    2,
		MaxIdleConns:    1,
		ConnMaxLifetime: 5 * time.Minute,
	})
	if err != nil {
		return nil, fmt.Errorf("打开版本库失败: %w", err)
	}
	for _, stmt := range versionSchema {
		if _, err := opened.Exec(stmt); err != nil {
			_ = opened.Close()
			return nil, fmt.Errorf("版本库建表失败: %w", err)
		}
	}
	versionDB = opened
	versionDBPath = want
	if err := migrateLegacyVersions(opened); err != nil {
		logger.Warn(moduleName, "旧版本目录导入失败（原目录保留）: %v", err)
	}
	return opened, nil
}

// insertVersion 写入一条版本记录；同 ID 重复导入时忽略。
func insertVersion(conn *sql.DB, version Version, content []byte) (bool, error) {
	res, err := conn.Exec(`INSERT OR IGNORE INTO file_version
		(id, path, op, exists_flag, size, sha256, content, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		version.ID, version.Path, version.Op, boolInt(version.Exists),
		version.Size, version.SHA256, content, version.CreatedAt)
	if err != nil {
		return false, err
	}
	affected, _ := res.RowsAffected()
	return affected > 0, nil
}

// queryVersions 按路径读取版本元数据（不含内容），新到旧。
func queryVersions(conn *sql.DB, path string, limit int) ([]Version, error) {
	if limit <= 0 {
		limit = maxVersionsPerFile
	}
	rows, err := conn.Query(`SELECT id, path, op, exists_flag, size, sha256, created_at
		FROM file_version WHERE path = ?
		ORDER BY created_at DESC, id DESC LIMIT ?`, path, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Version, 0, limit)
	for rows.Next() {
		var version Version
		var exists int
		if err := rows.Scan(&version.ID, &version.Path, &version.Op, &exists,
			&version.Size, &version.SHA256, &version.CreatedAt); err != nil {
			return nil, err
		}
		version.Exists = exists != 0
		out = append(out, version)
	}
	return out, rows.Err()
}

// queryVersion 读取单个版本及其内容；不存在时 ok=false。
func queryVersion(conn *sql.DB, path, id string) (Version, []byte, bool, error) {
	var version Version
	var exists int
	var content []byte
	err := conn.QueryRow(`SELECT id, path, op, exists_flag, size, sha256, created_at, content
		FROM file_version WHERE path = ? AND id = ?`, path, id).
		Scan(&version.ID, &version.Path, &version.Op, &exists,
			&version.Size, &version.SHA256, &version.CreatedAt, &content)
	if err == sql.ErrNoRows {
		return Version{}, nil, false, nil
	}
	if err != nil {
		return Version{}, nil, false, err
	}
	version.Exists = exists != 0
	return version, content, true, nil
}

// pruneFile 只保留单个文件最近的 maxVersionsPerFile 条版本。
func pruneFile(conn *sql.DB, path string) error {
	_, err := conn.Exec(`DELETE FROM file_version WHERE path = ? AND id NOT IN (
		SELECT id FROM file_version WHERE path = ?
		ORDER BY created_at DESC, id DESC LIMIT ?)`, path, path, maxVersionsPerFile)
	return err
}

// pruneGlobal 执行天数与总量保留策略。
func pruneGlobal(conn *sql.DB) error {
	cutoff := time.Now().AddDate(0, 0, -maxVersionAgeDays).Unix()
	if _, err := conn.Exec(`DELETE FROM file_version WHERE created_at < ?`, cutoff); err != nil {
		return err
	}
	var total int64
	if err := conn.QueryRow(`SELECT COUNT(*) FROM file_version`).Scan(&total); err != nil {
		return err
	}
	excess := total - maxTotalVersions
	if excess <= 0 {
		return nil
	}
	_, err := conn.Exec(`DELETE FROM file_version WHERE id IN (
		SELECT id FROM file_version ORDER BY created_at ASC, id ASC LIMIT ?)`, excess)
	return err
}

// migrateLegacyVersions 把旧版 file_versions 目录一次性导入版本库。
// 导入完成后给旧目录改名，避免每次启动重复扫描。
func migrateLegacyVersions(conn *sql.DB) error {
	root := filepath.Join(config.LocalDir, legacyVersionRootRel)
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	imported := 0
	for _, dir := range entries {
		if !dir.IsDir() {
			continue
		}
		sub := filepath.Join(root, dir.Name())
		files, readErr := os.ReadDir(sub)
		if readErr != nil {
			continue
		}
		for _, file := range files {
			if strings.ToLower(filepath.Ext(file.Name())) != ".json" {
				continue
			}
			version, content, ok := loadLegacyVersion(filepath.Join(sub, file.Name()))
			if !ok {
				continue
			}
			if len(content) > maxVersionBlobBytes {
				continue
			}
			inserted, insertErr := insertVersion(conn, version, content)
			if insertErr != nil || !inserted {
				continue
			}
			imported++
		}
	}
	logger.Info(moduleName, "旧版本备份已导入版本库: %d 条", imported)
	return archiveLegacyDir(root)
}

// loadLegacyVersion 读取一条旧格式版本记录与它的 .bak 副本。
func loadLegacyVersion(manifest string) (Version, []byte, bool) {
	raw, err := os.ReadFile(manifest)
	if err != nil {
		return Version{}, nil, false
	}
	var version Version
	if err := json.Unmarshal(raw, &version); err != nil || version.ID == "" || version.Path == "" {
		return Version{}, nil, false
	}
	if version.CreatedAt == 0 {
		version.CreatedAt = time.Now().Unix()
	}
	var content []byte
	if version.Exists {
		if version.BackupRel == "" {
			return Version{}, nil, false
		}
		content, err = os.ReadFile(filepath.Join(config.LocalDir, filepath.FromSlash(version.BackupRel)))
		if err != nil {
			return Version{}, nil, false
		}
		version.Size = int64(len(content))
		version.SHA256 = hashBytes(content)
	}
	return version, content, true
}

// archiveLegacyDir 迁移后给旧目录改名；目标已存在时追加时间戳。
func archiveLegacyDir(root string) error {
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return nil
	}
	target := filepath.Join(config.LocalDir, migratedVersionRootRel)
	if _, err := os.Stat(target); err == nil {
		target = target + "-" + strconv.FormatInt(time.Now().Unix(), 10)
	}
	if err := os.Rename(root, target); err != nil {
		return fmt.Errorf("旧版本目录改名失败: %w", err)
	}
	return nil
}

// boolInt 把布尔值转成 SQLite 的 0/1。
func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
