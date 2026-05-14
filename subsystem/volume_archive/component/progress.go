package component

import (
	"fmt"
	"regexp"
	"strings"
)

type ProgressTracker struct {
	LastPercent int
	StartTime   int64
}

func NewProgressTracker() *ProgressTracker {
	return &ProgressTracker{LastPercent: 0}
}

func (pt *ProgressTracker) UpdateProgress(output string) {
	re := regexp.MustCompile(`(\d+)%`)
	matches := re.FindStringSubmatch(output)

	if len(matches) > 1 {
		var percent int
		fmt.Sscanf(matches[1], "%d", &percent)

		if percent >= 0 && percent <= 100 && percent != pt.LastPercent {
			pt.LastPercent = percent
			pt.displayProgress(percent)
		}
	}
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

		fmt.Printf("\r\033[36m[\033[0m%s\033[36m]\033[0m %s%4d%%\033[0m", bar, percentColor, percent)
	} else {
		bar = strings.Repeat("█", barLength)
		fmt.Printf("\r\033[32m[\033[0m%s\033[32m]\033[0m \033[32m 100%%\033[0m \033[32m✓ 完成!\033[0m\n", bar)
	}
}
