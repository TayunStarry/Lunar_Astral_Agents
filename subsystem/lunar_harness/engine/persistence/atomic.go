package persistence

// atomic.go 状态文件的原子写：临时文件 → rename。
//
// 运行时状态（Tick 计数、生活额度、心情向量、记忆整理日志）必须避免
// 进程在写一半时被杀导致的半截 JSON。这里只负责字节落盘，不做句柄登记与内容哈希，
// 因此不能被 SaveFileEx 替代（后者会触发资源仓库观察者）。

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"LunarSubsystem/LunarHarness/engine/config"
)

// WriteFileAtomic 把 data 原子写入 LocalDir 下的 relPath。
func WriteFileAtomic(relPath string, data []byte) error {
	rel := filepath.ToSlash(strings.TrimSpace(relPath))
	if rel == "" || rel == ".." || strings.HasPrefix(rel, "../") || filepath.IsAbs(rel) {
		return fmt.Errorf("无效的文件名: %q", relPath)
	}
	localDir := filepath.Clean(config.LocalDir)
	fullPath := filepath.Clean(filepath.Join(localDir, filepath.FromSlash(rel)))
	if fullPath != localDir && !strings.HasPrefix(fullPath, localDir+string(os.PathSeparator)) {
		return fmt.Errorf("访问被拒绝: %s", relPath)
	}
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}
	tmpPath := fullPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("写入临时文件失败: %w", err)
	}
	if err := os.Rename(tmpPath, fullPath); err != nil {
		// Windows 上目标已存在时部分文件系统会拒绝 rename；退回 remove + rename。
		_ = os.Remove(fullPath)
		if retryErr := os.Rename(tmpPath, fullPath); retryErr != nil {
			_ = os.Remove(tmpPath)
			return fmt.Errorf("替换状态文件失败: %w", retryErr)
		}
	}
	return nil
}

// ReadFileBytes 读取 LocalDir 下的状态文件；不存在时返回 (nil, false)。
func ReadFileBytes(relPath string) ([]byte, bool) {
	rel := filepath.ToSlash(strings.TrimSpace(relPath))
	if rel == "" {
		return nil, false
	}
	fullPath := filepath.Join(config.LocalDir, filepath.FromSlash(rel))
	raw, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, false
	}
	return raw, true
}
