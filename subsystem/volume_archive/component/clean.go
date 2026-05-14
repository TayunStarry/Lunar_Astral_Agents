package component

import (
	"os"
	"path/filepath"
	"strings"
)

func cleanOldParts(outBase string) {
	PrintInfo("清理旧分卷文件...\n")

	dirName := filepath.Dir(outBase)
	baseName := filepath.Base(outBase)

	entries, err := os.ReadDir(dirName)
	if err != nil {
		PrintWarning("无法读取目录 %s: %v\n", dirName, err)
		return
	}

	cleaned := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		fname := entry.Name()
		fullPath := filepath.Join(dirName, fname)

		if strings.HasPrefix(fname, baseName) && strings.Contains(fname, ".7z.") {
			if err := os.Remove(fullPath); err != nil {
				PrintWarning("无法删除旧分卷 %s: %v\n", fullPath, err)
			} else {
				PrintInfo("已删除: %s\n", fname)
				cleaned++
			}
		}
	}

	full7z := filepath.Join(dirName, baseName+".7z")
	if fileExists(full7z) {
		if err := os.Remove(full7z); err != nil {
			PrintWarning("无法删除完整压缩包 %s: %v\n", full7z, err)
		} else {
			PrintInfo("已删除: %s.7z\n", baseName)
			cleaned++
		}
	}

	if cleaned > 0 {
		PrintInfo("共清理 %d 个旧文件\n", cleaned)
	} else {
		PrintInfo("没有需要清理的旧文件\n")
	}
}
