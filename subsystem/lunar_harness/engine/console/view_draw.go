package console

import (
	"regexp"
	"strings"
)

// 采样进度解析（引擎输出形如 "10/20"、"25%"、"1.02s/it"、"3.2it/s"）
var (
	drawStepTotalRe = regexp.MustCompile(`(\d+)\s*/\s*(\d+)`)
	drawPercentRe   = regexp.MustCompile(`(\d+)\s*%`)
	drawSpeedRe     = regexp.MustCompile(`([\d.]+)\s*(s/it|it/s|ms/it)`)
)

// ==== 绘图引擎（sd.cpp）日志屏 ====

// renderDrawLogLine 绘图引擎单行排版：
//   - 错误/警告沿用红/橙并加标记；
//   - 进度/速度行以亮青特别标注（引擎主要输出就是这类采样节奏信息）；
//   - 其余引擎原始输出弱化为灰色，模块编排日志（Source=log）正常青色。
func renderDrawLogLine(line LogLine) string {
	switch {
	case line.Level == "ERROR":
		return logErrStyle.Render("✗ " + line.Text)
	case line.Level == "WARN":
		return logWarnStyle.Render("! " + line.Text)
	case isDrawProgressLine(line.Text):
		return drawProgressStyle.Render("▸ " + line.Text)
	case line.Source == "engine":
		return logDimStyle.Render("  " + line.Text)
	default:
		return logInfoStyle.Render("→ " + line.Text)
	}
}

// isDrawProgressLine 进度/速度行判定
func isDrawProgressLine(text string) bool {
	for _, pat := range drawProgressMarks {
		if strings.Contains(text, pat) {
			return true
		}
	}
	return false
}

// drawProgressMarks 进度/速度行的特征子串
var drawProgressMarks = []string{"s/it", "it/s", "ms/it", "%|", "==", "=>"}

// drawProgress 绘图引擎采样进度（从日志尾部解析最近一次报告）
type drawProgress struct {
	// Found 是否解析到进度信息
	Found bool
	// Step/Total 当前步 / 总步数（Total 未知时为 0）
	Step, Total int
	// Percent 采样百分比（无人为放大，0-100）
	Percent int
	// Speed 采样速度文本（如 "1.02s/it"）
	Speed string
}

// parseDrawProgress 从日志尾部解析最近的采样进度：
// 依次尝试「步/总步」与「百分比」两种写法，取最后一条命中记录。
func parseDrawProgress(logs []LogLine) drawProgress {
	var p drawProgress
	for _, l := range logs {
		text := l.Text
		if m := drawStepTotalRe.FindStringSubmatch(text); m != nil {
			step, total := atoiSafe(m[1]), atoiSafe(m[2])
			if total > 0 && step >= 0 {
				p = drawProgress{
					Found:   true,
					Step:    step,
					Total:   total,
					Percent: clampPercent(step * 100 / total),
					Speed:   p.Speed,
				}
			}
		} else if m := drawPercentRe.FindStringSubmatch(text); m != nil {
			percent := clampPercent(atoiSafe(m[1]))
			if p.Total > 0 {
				p.Percent = percent // 有总步数时以步数换算，百分比仅兜底
				p.Found = true
			} else if percent > 0 {
				p = drawProgress{Found: true, Percent: percent, Speed: p.Speed}
			}
		}
		if m := drawSpeedRe.FindStringSubmatch(text); m != nil {
			p.Speed = m[1] + m[2]
			p.Found = true
		}
	}
	return p
}

// clampPercent 百分比收敛到 0-100
func clampPercent(v int) int {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

// atoiSafe 宽松数字解析（解析失败返回 0）
func atoiSafe(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return n
		}
		n = n*10 + int(r-'0')
	}
	return n
}
