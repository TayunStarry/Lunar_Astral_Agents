package library

// ops.go 资源库内的文件管理：重命名 / 移动 / 删除。
// 所有操作都限定在 4 个存储根目录内，并同步维护句柄映射。

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/persistence"
)

// Rename 同目录内重命名。
func Rename(rel, newName string) (string, error) {
	rel = normalizeRel(rel)
	full, err := resolve(rel)
	if err != nil {
		return "", err
	}
	info, statErr := os.Stat(full)
	if statErr != nil || info.IsDir() {
		return "", fmt.Errorf("目标不是可重命名的文件")
	}
	name, err := validName(newName)
	if err != nil {
		return "", err
	}
	targetRel := joinRel(dirOf(rel), name)
	targetFull := absOf(targetRel)
	if _, statErr := os.Stat(targetFull); statErr == nil {
		return "", fmt.Errorf("目标位置已存在同名文件")
	}
	if err := os.Rename(full, targetFull); err != nil {
		return "", fmt.Errorf("重命名失败: %w", err)
	}
	persistence.NotifyMove(rel, targetRel)
	logger.Info(Module, "[资源库] 重命名: %s → %s", rel, targetRel)
	notifyChanged("rename", rel, targetRel)
	return targetRel, nil
}

// Move 把文件或目录移动到目标目录。
func Move(rel, targetDir string) (string, error) {
	rel = normalizeRel(rel)
	full, err := resolve(rel)
	if err != nil {
		return "", err
	}
	info, statErr := os.Stat(full)
	if statErr != nil {
		return "", fmt.Errorf("源路径不存在")
	}
	targetDir = normalizeRel(targetDir)
	targetBase, err := resolveDir(targetDir)
	if err != nil {
		return "", err
	}
	if info.IsDir() && strings.HasPrefix(filepath.Clean(targetBase), filepath.Clean(full)) {
		return "", fmt.Errorf("不能把目录移动到其自身内部")
	}
	targetRel := joinRel(targetDir, filepath.Base(full))
	targetFull := absOf(targetRel)
	if _, statErr := os.Stat(targetFull); statErr == nil {
		return "", fmt.Errorf("目标目录存在同名项")
	}
	if err := os.Rename(full, targetFull); err != nil {
		return "", fmt.Errorf("移动失败: %w", err)
	}
	persistence.NotifyMove(rel, targetRel)
	logger.Info(Module, "[资源库] 移动: %s → %s", rel, targetRel)
	notifyChanged("move", rel, targetRel)
	return targetRel, nil
}

// Delete 删除文件（目录需先清空，避免误删整棵子树）。
func Delete(rel string) (string, error) {
	rel = normalizeRel(rel)
	full, err := resolve(rel)
	if err != nil {
		return "", err
	}
	info, statErr := os.Stat(full)
	if statErr != nil {
		return "", fmt.Errorf("文件不存在")
	}
	if info.IsDir() {
		if entries, _ := os.ReadDir(full); len(entries) > 0 {
			return "", fmt.Errorf("目录非空，请先移动或删除其中的文件")
		}
	}
	if err := os.Remove(full); err != nil {
		return "", fmt.Errorf("删除失败: %w", err)
	}
	persistence.NotifyDelete(rel)
	logger.Info(Module, "[资源库] 删除: %s", rel)
	notifyChanged("delete", rel, "")
	return rel, nil
}

// notifyChanged 广播一次结构变化，供当前打开的资源库页面实时刷新。
func notifyChanged(op, from, to string) {
	broadcastMu.RLock()
	fn := broadcast
	broadcastMu.RUnlock()
	if fn == nil {
		return
	}
	fn("library_changed", map[string]any{"op": op, "path": from, "target": to})
}
