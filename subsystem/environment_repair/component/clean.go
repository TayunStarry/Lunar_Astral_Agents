package component

import (
	"os"
	"path/filepath"
	"strings"
)

// cleanOldParts 清理输出目录下与输出路径同名的旧分卷文件和完整压缩包
func cleanOldParts(outBase string) {
	uiLogf("清理旧分卷文件...")

	dirName := filepath.Dir(outBase)
	baseName := filepath.Base(outBase)

	entries, err := os.ReadDir(dirName)
	if err != nil {
		uiLogf("  [WARN] 无法读取目录 %s: %v", dirName, err)
		return
	}

	cleaned := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		fname := entry.Name()
		fullPath := filepath.Join(dirName, fname)

		// 匹配分卷文件：{baseName}.7z.001, {baseName}.7z.002, ...
		if strings.HasPrefix(fname, baseName) && strings.Contains(fname, ".7z.") {
			if err := os.Remove(fullPath); err != nil {
				uiLogf("  [WARN] 无法删除旧分卷 %s: %v", fullPath, err)
			} else {
				uiLogf("  已删除: %s", fname)
				cleaned++
			}
		}
	}

	// 删除完整压缩包（非分卷）
	full7z := filepath.Join(dirName, baseName+".7z")
	if fileExists(full7z) {
		if err := os.Remove(full7z); err != nil {
			uiLogf("  [WARN] 无法删除完整压缩包 %s: %v", full7z, err)
		} else {
			uiLogf("  已删除: %s.7z", baseName)
			cleaned++
		}
	}

	if cleaned > 0 {
		uiLogf("  共清理 %d 个旧文件", cleaned)
	} else {
		uiLogf("  没有需要清理的旧文件")
	}
}
