package component

import (
	"fmt"
	"regexp"
)

// ProgressTracker 进度追踪器，解析 7z 输出并通过 progressHook 上报进度
type ProgressTracker struct {
	LastPercent int
	HasProgress bool // 是否已解析到真实进度（false 表示仍处于准备阶段）
}

// NewProgressTracker 创建新的进度追踪器
func NewProgressTracker() *ProgressTracker {
	return &ProgressTracker{LastPercent: 0, HasProgress: false}
}

// UpdateProgress 从 7z 输出行解析进度百分比并上报
func (pt *ProgressTracker) UpdateProgress(output string) {
	re := regexp.MustCompile(`(\d+)%`)
	matches := re.FindStringSubmatch(output)

	if len(matches) > 1 {
		var percent int
		_, _ = fmt.Sscanf(matches[1], "%d", &percent)

		if percent >= 0 && percent <= 100 {
			pt.HasProgress = true
			if percent != pt.LastPercent {
				pt.LastPercent = percent
				reportProgress(percent)
			}
		}
	}
}

// reportPreparing 上报准备阶段（未知进度，-1）
func (pt *ProgressTracker) reportPreparing() {
	reportProgress(-1)
}
