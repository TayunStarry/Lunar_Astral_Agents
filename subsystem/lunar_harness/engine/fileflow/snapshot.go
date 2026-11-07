package fileflow

// snapshot.go 文件版本快照与回滚：操作前把目标文件原样复制到快照目录，
// 审查未通过时按快照还原。回滚失败逐条汇总并返回错误，调用方必须显式上报。

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/persistence"
	"LunarSubsystem/LunarHarness/engine/resource"
	"LunarSubsystem/LunarHarness/engine/resourcehub"
)

// Capture 在操作前抓取一组路径的版本快照。
//
// 路径不存在时记录 Existed=false，回滚时删除操作期间新建的实体；
// 目录会递归记录子目录与文件副本，回滚时按清单重建整棵目录树。
func Capture(flowID string, seq int, op, target string, paths []string) (Snapshot, error) {
	flowID = safeFlowID(flowID)
	dirRel := filepath.ToSlash(filepath.Join(SnapshotRootRel, flowID, fmt.Sprintf("%03d", seq)))
	snapshot := Snapshot{
		FlowID:    flowID,
		Seq:       seq,
		Op:        op,
		Target:    target,
		Dir:       dirRel,
		Entries:   []Entry{},
		CreatedAt: time.Now().Unix(),
	}
	seen := map[string]bool{}
	for _, raw := range paths {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		abs, rel, err := Resolve(raw)
		if err != nil {
			return Snapshot{}, fmt.Errorf("解析快照路径失败: %w", err)
		}
		if seen[abs] {
			continue
		}
		seen[abs] = true
		entry := Entry{Abs: abs, Rel: rel}
		info, statErr := os.Stat(abs)
		switch {
		case os.IsNotExist(statErr):
			// 操作前不存在：回滚时删除
		case statErr != nil:
			return Snapshot{}, fmt.Errorf("读取快照目标失败 %s: %w", rel, statErr)
		case info.IsDir():
			entry.Existed = true
			entry.IsDir = true
			entry.Size = 0
			dirs, files, size, treeErr := captureTree(abs, dirRel, len(snapshot.Entries))
			if treeErr != nil {
				return Snapshot{}, treeErr
			}
			entry.Dirs = dirs
			entry.Files = files
			entry.Size = size
		default:
			if info.Size() > maxSnapshotBytes {
				return Snapshot{}, fmt.Errorf("文件过大（%d 字节），超过快照上限 %d 字节，已取消写入: %s", info.Size(), maxSnapshotBytes, rel)
			}
			data, readErr := os.ReadFile(abs)
			if readErr != nil {
				return Snapshot{}, fmt.Errorf("读取快照目标失败 %s: %w", rel, readErr)
			}
			entry.Existed = true
			entry.Size = info.Size()
			entry.SHA256 = hashBytes(data)
			entry.Blob = filepath.ToSlash(filepath.Join(dirRel, blobsDir, fmt.Sprintf("%d.bin", len(snapshot.Entries))))
			if writeErr := persistence.WriteFileAtomic(entry.Blob, data); writeErr != nil {
				return Snapshot{}, fmt.Errorf("写入快照副本失败 %s: %w", rel, writeErr)
			}
		}
		snapshot.Entries = append(snapshot.Entries, entry)
	}
	raw, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return Snapshot{}, fmt.Errorf("序列化快照清单失败: %w", err)
	}
	if err := persistence.WriteFileAtomic(filepath.ToSlash(filepath.Join(dirRel, manifestFile)), raw); err != nil {
		return Snapshot{}, fmt.Errorf("写入快照清单失败: %w", err)
	}
	return snapshot, nil
}

// captureTree 递归抓取目录快照：记录子目录与文件副本。
func captureTree(abs, dirRel string, entryIndex int) ([]string, []TreeFile, int64, error) {
	dirs := make([]string, 0)
	files := make([]TreeFile, 0)
	blobRoot := filepath.ToSlash(filepath.Join(dirRel, blobsDir, fmt.Sprintf("tree-%d", entryIndex)))
	var total int64
	walkErr := filepath.WalkDir(abs, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == abs {
			return nil
		}
		rel, relErr := filepath.Rel(abs, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		if entry.IsDir() {
			dirs = append(dirs, rel)
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("目录快照不支持符号链接: %s", rel)
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return infoErr
		}
		if len(files) >= maxSnapshotTreeFiles {
			return fmt.Errorf("目录文件数超过快照上限 %d: %s", maxSnapshotTreeFiles, rel)
		}
		if info.Size() > maxSnapshotBytes {
			return fmt.Errorf("目录内文件过大（%d 字节），超过快照上限: %s", info.Size(), rel)
		}
		total += info.Size()
		if total > maxSnapshotTreeBytes {
			return fmt.Errorf("目录总字节超过快照上限 %d: %s", maxSnapshotTreeBytes, rel)
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("读取目录内文件失败 %s: %w", rel, readErr)
		}
		blobRel := filepath.ToSlash(filepath.Join(blobRoot, fmt.Sprintf("%d.bin", len(files))))
		if writeErr := persistence.WriteFileAtomic(blobRel, data); writeErr != nil {
			return fmt.Errorf("写入目录快照副本失败 %s: %w", rel, writeErr)
		}
		files = append(files, TreeFile{Rel: rel, Size: info.Size(), SHA256: hashBytes(data), Blob: blobRel})
		return nil
	})
	return dirs, files, total, walkErr
}

// Restore 把快照中的路径还原到操作前的版本。
//
// 逐条执行并收集错误：任何一条失败都会返回非空 error，由调用方写入审查结果与流程错误，
// 不允许静默忽略。
func Restore(snapshot Snapshot) error {
	if len(snapshot.Entries) == 0 {
		return nil
	}
	failures := make([]string, 0, len(snapshot.Entries))
	for _, entry := range snapshot.Entries {
		if err := restoreEntry(entry); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", entry.Rel, err))
		}
	}
	resourcehub.Invalidate()
	for _, entry := range snapshot.Entries {
		if entry.Existed {
			_, _, _ = resource.RegisterPath(entry.Rel)
			for _, file := range entry.Files {
				rel := filepath.ToSlash(filepath.Join(entry.Rel, file.Rel))
				_, _, _ = resource.RegisterPath(rel)
			}
			continue
		}
		resource.RemoveHandlesForPath(entry.Rel)
		for _, file := range entry.Files {
			resource.RemoveHandlesForPath(filepath.ToSlash(filepath.Join(entry.Rel, file.Rel)))
		}
	}
	writeRollback(snapshot, failures)
	if len(failures) > 0 {
		return errors.Join(joinErrors(failures)...)
	}
	return nil
}

// restoreEntry 还原单条快照。
func restoreEntry(entry Entry) error {
	if entry.IsDir {
		return restoreTree(entry)
	}
	if entry.Existed {
		blob, ok := persistence.ReadFileBytes(entry.Blob)
		if !ok {
			return fmt.Errorf("快照副本缺失（%s）", entry.Blob)
		}
		if entry.SHA256 != "" && hashBytes(blob) != entry.SHA256 {
			return fmt.Errorf("快照副本哈希不一致（%s）", entry.Blob)
		}
		if info, err := os.Stat(entry.Abs); err == nil && info.IsDir() {
			return fmt.Errorf("目标已变成目录，拒绝覆盖")
		}
		return persistence.WriteFileAtomic(entry.Rel, blob)
	}
	info, err := os.Stat(entry.Abs)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("目标已变成目录，拒绝删除")
	}
	if err := os.Remove(entry.Abs); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除新建文件失败: %w", err)
	}
	return nil
}

// restoreTree 还原目录快照：先清空目标目录，再按清单重建子目录与文件。
func restoreTree(entry Entry) error {
	if !entry.Existed {
		if _, err := os.Stat(entry.Abs); os.IsNotExist(err) {
			return nil
		}
		return os.RemoveAll(entry.Abs)
	}
	if info, err := os.Stat(entry.Abs); err == nil && !info.IsDir() {
		if removeErr := os.Remove(entry.Abs); removeErr != nil {
			return fmt.Errorf("目标已变成文件，拒绝目录还原: %w", removeErr)
		}
	}
	if err := os.RemoveAll(entry.Abs); err != nil {
		return fmt.Errorf("清空目标目录失败: %w", err)
	}
	if err := os.MkdirAll(entry.Abs, 0o755); err != nil {
		return fmt.Errorf("重建目标目录失败: %w", err)
	}
	for _, dir := range entry.Dirs {
		if err := os.MkdirAll(filepath.Join(entry.Abs, filepath.FromSlash(dir)), 0o755); err != nil {
			return fmt.Errorf("重建子目录 %s 失败: %w", dir, err)
		}
	}
	for _, file := range entry.Files {
		blob, ok := persistence.ReadFileBytes(file.Blob)
		if !ok {
			return fmt.Errorf("快照副本缺失（%s）", file.Blob)
		}
		if file.SHA256 != "" && hashBytes(blob) != file.SHA256 {
			return fmt.Errorf("快照副本哈希不一致（%s）", file.Blob)
		}
		rel := filepath.ToSlash(filepath.Join(entry.Rel, file.Rel))
		if parent := filepath.Dir(filepath.FromSlash(rel)); parent != "." {
			if err := os.MkdirAll(filepath.Join(filepath.Clean(config.LocalDir), parent), 0o755); err != nil {
				return fmt.Errorf("重建文件目录失败 %s: %w", file.Rel, err)
			}
		}
		if err := persistence.WriteFileAtomic(rel, blob); err != nil {
			return fmt.Errorf("还原文件 %s 失败: %w", file.Rel, err)
		}
	}
	return nil
}

// writeRollback 把回滚结果写入快照目录（失败不影响主流程，但会在返回错误里体现）。
func writeRollback(snapshot Snapshot, failures []string) {
	log := rollbackLog{OK: len(failures) == 0, Errors: failures, At: time.Now().Unix()}
	raw, err := json.MarshalIndent(log, "", "  ")
	if err != nil {
		return
	}
	rel := filepath.ToSlash(filepath.Join(snapshot.Dir, rollbackFile))
	_ = persistence.WriteFileAtomic(rel, raw)
}

// CleanupFlows 只保留最近 keep 个流程快照目录。
func CleanupFlows(keep int) {
	if keep <= 0 {
		keep = maxSnapshotFlows
	}
	root := filepath.Join(config.LocalDir, filepath.FromSlash(SnapshotRootRel))
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	type flowDir struct {
		name    string
		modTime time.Time
	}
	dirs := make([]flowDir, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			info, infoErr := entry.Info()
			if infoErr == nil {
				dirs = append(dirs, flowDir{name: entry.Name(), modTime: info.ModTime()})
			}
		}
	}
	if len(dirs) <= keep {
		return
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].modTime.After(dirs[j].modTime) })
	for _, dir := range dirs[keep:] {
		_ = os.RemoveAll(filepath.Join(root, dir.name))
	}
}

// hashBytes 计算内容哈希。
func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// joinErrors 把字符串错误转为 error 列表。
func joinErrors(failures []string) []error {
	out := make([]error, 0, len(failures))
	for _, failure := range failures {
		out = append(out, errors.New(failure))
	}
	return out
}

// safeFlowID 去掉路径分隔符，避免流程 ID 变成目录穿越。
func safeFlowID(flowID string) string {
	cleaned := strings.ReplaceAll(strings.TrimSpace(flowID), "\\", "")
	cleaned = strings.ReplaceAll(cleaned, "/", "")
	cleaned = strings.ReplaceAll(cleaned, "..", "")
	if cleaned == "" {
		cleaned = nowText()
	}
	return cleaned
}
