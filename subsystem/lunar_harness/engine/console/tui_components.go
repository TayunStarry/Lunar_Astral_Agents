package console

// ui_components.go 全屏共享的渲染小部件（表单行、表单框、二维码、居中）。

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/skip2/go-qrcode"
)

// ==== 共享渲染组件 ====

// fieldRow 表单行：标签定宽 + 输入框（聚焦时高亮并加 ▸ 标记）
func fieldRow(label string, input textinput.Model, focused bool, labelW int) string {
	mark := "  "
	name := dimStyle.Render(fitWidth(label, labelW))
	if focused {
		mark = cursorStyle.Render("▸ ")
		name = selectStyle.Render(fitWidth(label, labelW))
	}
	return mark + name + input.View()
}

// formBox 表单框：给输入区加一层圆角描边，聚焦范围更明确
func formBox(lines []string, width int) []string {
	if width < 12 {
		return lines
	}
	inner := width - 4
	out := make([]string, 0, len(lines)+2)
	out = append(out, composeRule("╭", "╮", "", "", "─", width))
	for _, l := range lines {
		out = append(out, ruleStyle.Render("│ ")+fitWidth(l, inner)+ruleStyle.Render(" │"))
	}
	out = append(out, composeRule("╰", "╯", "", "", "─", width))
	return out
}

// generateQRText 生成终端半块字符二维码
func generateQRText(url string) string {
	qr, err := qrcode.New(url, qrcode.Low)
	if err != nil {
		return ""
	}
	qr.DisableBorder = true
	return qr.ToSmallString(false)
}

// centerBlock 整块内容按宽度居中（按显示宽度补足左侧空格）
func centerBlock(lines []string, width int) []string {
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		d := dispWidth(l)
		if d >= width {
			out = append(out, truncTo(l, width))
			continue
		}
		pad := (width - d) / 2
		out = append(out, strings.Repeat(" ", pad)+l)
	}
	return out
}
