package filesupport

// versions.go 文件内容版本：每次写操作前保存一份原文件内容，
// 供 contentRollback 列出与回滚。
//
// 存储已改为 SQLite（local_data/database/versions.db）：版本元数据与内容同表，
// 单条 INSERT 即原子，不再依赖「.json + .bak」两个文件的先后写入。

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/resourcehub"
)

// Backup 在修改前记录版本；文件不存在时也记录一条 Exists=false 的记录。
func Backup(rel string) (Version, error) {
	return BackupWithOp(rel, "backup")
}

// BackupWithOp 记录一次带操作类型的版本备份（replace / append / delete_lines 等）。
func BackupWithOp(rel, op string) (Version, error) {
	rel = normalizeVersionPath(rel)
	if rel == "" {
		return Version{}, fmt.Errorf("非法版本路径")
	}
	version := Version{
		ID:        nextVersionID(),
		Path:      rel,
		Op:        strings.TrimSpace(op),
		CreatedAt: time.Now().Unix(),
	}
	var content []byte
	data, err := os.ReadFile(Abs(rel))
	switch {
	case err == nil:
		if int64(len(data)) > maxVersionBlobBytes {
			return Version{}, fmt.Errorf("文件超过单个版本上限（%d MiB），未保存版本：%s", maxVersionBlobBytes>>20, rel)
		}
		version.Exists = true
		version.Size = int64(len(data))
		version.SHA256 = hashBytes(data)
		content = data
	case os.IsNotExist(err):
		version.Exists = false
	default:
		return Version{}, err
	}
	conn, err := ensureVersionDB()
	if err != nil {
		return Version{}, err
	}
	if _, err := insertVersion(conn, version, content); err != nil {
		return Version{}, err
	}
	if err := pruneFile(conn, rel); err != nil {
		logger.Warn(moduleName, "版本裁剪失败: %v", err)
	}
	if versionWrites.Add(1)%cleanupEveryWrites == 0 {
		if err := pruneGlobal(conn); err != nil {
			logger.Warn(moduleName, "版本库全局清理失败: %v", err)
		}
	}
	return version, nil
}

// List 列出某个路径的全部版本（新到旧）。
func List(rel string) ([]Version, error) {
	rel = normalizeVersionPath(rel)
	if rel == "" {
		return nil, nil
	}
	conn, err := ensureVersionDB()
	if err != nil {
		return nil, err
	}
	return queryVersions(conn, rel, maxVersionsPerFile)
}

// Restore 回滚到指定版本。
func Restore(rel, versionID string) error {
	rel = normalizeVersionPath(rel)
	if rel == "" {
		return fmt.Errorf("非法版本路径")
	}
	conn, err := ensureVersionDB()
	if err != nil {
		return err
	}
	version, content, ok, err := queryVersion(conn, rel, versionID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("版本 %s 不存在", versionID)
	}
	abs := Abs(rel)
	if !version.Exists {
		if removeErr := os.Remove(abs); removeErr != nil && !os.IsNotExist(removeErr) {
			return removeErr
		}
		resourcehub.Invalidate()
		return nil
	}
	if content == nil {
		return fmt.Errorf("版本 %s 的内容缺失", versionID)
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(abs, content, 0o644); err != nil {
		return err
	}
	resourcehub.Invalidate()
	return nil
}

// OldestSince 返回创建时间不早于 sinceUnix 的最早版本。
//
// 一次任务内可能对同一文件连续写入多次，每次写入前都会备份；
// 要整体撤销这次任务，应恢复到「任务开始后第一次备份」——它保存的正是任务前的内容。
func OldestSince(rel string, sinceUnix int64) (Version, bool) {
	rel = normalizeVersionPath(rel)
	if rel == "" {
		return Version{}, false
	}
	conn, err := ensureVersionDB()
	if err != nil {
		return Version{}, false
	}
	var version Version
	var exists int
	err = conn.QueryRow(`SELECT id, path, op, exists_flag, size, sha256, created_at
		FROM file_version WHERE path = ? AND created_at >= ?
		ORDER BY created_at ASC, id ASC LIMIT 1`, rel, sinceUnix).
		Scan(&version.ID, &version.Path, &version.Op, &exists, &version.Size, &version.SHA256, &version.CreatedAt)
	if err != nil {
		return Version{}, false
	}
	version.Exists = exists != 0
	return version, true
}

// Latest 返回指定路径最近的版本。
func Latest(rel string) (Version, bool) {
	versions, err := List(rel)
	if err != nil || len(versions) == 0 {
		return Version{}, false
	}
	return versions[0], true
}

// Stats 返回版本库的总体占用情况。
func Stats() (VersionStats, error) {
	conn, err := ensureVersionDB()
	if err != nil {
		return VersionStats{}, err
	}
	var out VersionStats
	err = conn.QueryRow(`SELECT COUNT(DISTINCT path), COUNT(*), COALESCE(SUM(size), 0)
		FROM file_version`).Scan(&out.Files, &out.Versions, &out.Bytes)
	return out, err
}

// Prune 立即执行全部保留策略（单文件条数、最长天数、全库总量）。
func Prune() error {
	conn, err := ensureVersionDB()
	if err != nil {
		return err
	}
	rows, err := conn.Query(`SELECT DISTINCT path FROM file_version`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return err
		}
		if err := pruneFile(conn, path); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return pruneGlobal(conn)
}

// Clear 删除某个文件的全部历史版本。
func Clear(rel string) error {
	rel = normalizeVersionPath(rel)
	if rel == "" {
		return fmt.Errorf("非法版本路径")
	}
	conn, err := ensureVersionDB()
	if err != nil {
		return err
	}
	_, err = conn.Exec(`DELETE FROM file_version WHERE path = ?`, rel)
	return err
}

// normalizeVersionPath 规整相对 LocalDir 的版本路径；越界路径返回空串。
func normalizeVersionPath(rel string) string {
	rel = filepath.ToSlash(filepath.Clean(strings.TrimSpace(rel)))
	if rel == "." || rel == "" || rel == ".." || strings.HasPrefix(rel, "../") {
		return ""
	}
	return rel
}

// nextVersionID 生成单调递增的版本 ID：纳秒时间戳 + 同纳秒序号。
func nextVersionID() string {
	return fmt.Sprintf("%019d-%010d", time.Now().UnixNano(), versionSeq.Add(1))
}

// hashBytes 计算内容摘要。
func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
