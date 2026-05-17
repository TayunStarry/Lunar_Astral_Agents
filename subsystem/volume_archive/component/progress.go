package component

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

type ProgressTracker struct {
	LastPercent  int
	StartTime    time.Time
	IsStarted    bool
	HasProgress  bool
	SpinnerIndex int
}

func NewProgressTracker() *ProgressTracker {
	return &ProgressTracker{
		LastPercent:  0,
		StartTime:    time.Now(),
		IsStarted:    true,
		HasProgress:  false,
		SpinnerIndex: 0,
	}
}

func (pt *ProgressTracker) UpdateProgress(output string) {
	re := regexp.MustCompile(`(\d+)%`)
	matches := re.FindStringSubmatch(output)

	if len(matches) > 1 {
		var percent int
		fmt.Sscanf(matches[1], "%d", &percent)

		if percent >= 0 && percent <= 100 {
			pt.HasProgress = true
			if percent != pt.LastPercent {
				pt.LastPercent = percent
				pt.displayProgress(percent)
			}
		}
	}
}

func (pt *ProgressTracker) displayPreparing() {
	spinners := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	elapsed := time.Since(pt.StartTime).Seconds()

	pt.SpinnerIndex = (pt.SpinnerIndex + 1) % len(spinners)
	spinner := spinners[pt.SpinnerIndex]

	fmt.Printf("\r\033[36m[%s]\033[0m \033[33m正在准备压缩...\033[0m 耗时: %.1fs", spinner, elapsed)
}

func (pt *ProgressTracker) displayProgress(percent int) {
	barLength := 50
	filled := int(float64(percent) / 100.0 * float64(barLength))

	if filled > barLength {
		filled = barLength
	}

	var bar string
	if percent < 100 {
		if filled > 0 {
			bar = strings.Repeat("█", filled)
		}
		if filled < barLength {
			bar += strings.Repeat("░", barLength-filled)
		}

		percentColor := "\033[36m"
		if percent >= 75 {
			percentColor = "\033[32m"
		} else if percent >= 50 {
			percentColor = "\033[33m"
		} else if percent >= 25 {
			percentColor = "\033[93m"
		}

		elapsed := time.Since(pt.StartTime).Seconds()
		fmt.Printf("\r\033[36m[\033[0m%s\033[36m]\033[0m %s%4d%%\033[0m 耗时: %.1fs", bar, percentColor, percent, elapsed)
	} else {
		bar = strings.Repeat("█", barLength)
		elapsed := time.Since(pt.StartTime).Seconds()
		fmt.Printf("\r\033[32m[\033[0m%s\033[32m]\033[0m \033[32m 100%%\033[0m \033[32m✓ 完成! 总耗时: %.1fs\033[0m\n", bar, elapsed)
	}
}
