package archive

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// progressTracker 进度追踪器，解析 7z 输出并通过 progressHook 上报进度
type progressTracker struct {
	lastPercent int
	hasProgress bool // 是否已解析到真实进度（false 表示仍处于准备阶段）
}

// newProgressTracker 创建新的进度追踪器
func newProgressTracker() *progressTracker {
	return &progressTracker{lastPercent: 0, hasProgress: false}
}

// updateProgress 从 7z 输出行解析进度百分比并上报
func (pt *progressTracker) updateProgress(output string) {
	re := regexp.MustCompile(`(\d+)%`)
	matches := re.FindStringSubmatch(output)

	if len(matches) > 1 {
		var percent int
		_, _ = fmt.Sscanf(matches[1], "%d", &percent)

		if percent >= 0 && percent <= 100 {
			pt.hasProgress = true
			if percent != pt.lastPercent {
				pt.lastPercent = percent
				reportProgress(percent)
			}
		}
	}
}

// reportPreparing 上报准备阶段（未知进度，-1）
func (pt *progressTracker) reportPreparing() {
	reportProgress(-1)
}

// cleanOldParts 清理输出目录下与输出路径同名的旧分卷文件和完整压缩包
func cleanOldParts(outBase string) {
	logf("清理旧分卷文件...")

	dirName := filepath.Dir(outBase)
	baseName := filepath.Base(outBase)

	entries, err := os.ReadDir(dirName)
	if err != nil {
		logf("  [WARN] 无法读取目录 %s: %v", dirName, err)
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
				logf("  [WARN] 无法删除旧分卷 %s: %v", fullPath, err)
			} else {
				logf("  已删除: %s", fname)
				cleaned++
			}
		}
	}

	// 删除完整压缩包（非分卷）
	full7z := filepath.Join(dirName, baseName+".7z")
	if fileExists(full7z) {
		if err := os.Remove(full7z); err != nil {
			logf("  [WARN] 无法删除完整压缩包 %s: %v", full7z, err)
		} else {
			logf("  已删除: %s.7z", baseName)
			cleaned++
		}
	}

	if cleaned > 0 {
		logf("  共清理 %d 个旧文件", cleaned)
	} else {
		logf("  没有需要清理的旧文件")
	}
}
