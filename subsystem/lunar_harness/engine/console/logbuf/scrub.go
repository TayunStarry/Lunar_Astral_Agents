package logbuf

// scrub.go 日志正文清洗：剥离 ANSI 转义序列、归位控制符、按行拆分。
// 缓冲、导出与界面渲染必须共享同一套清洗口径，否则三处会各自漂移。

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// ScrubLogText 清洗日志正文：剥离 ANSI 转义序列、归位控制符、按行拆分。
//
// 必须做这一步的原因（实测）：日志正文里普遍带终端控制序列——
//   - `putchar('\r')` 原地刷新进度（模型加载、sd.cpp 采样、7z 进度）；
//   - 彩色工具输出（git / npm / 编译器）与 OSC 序列（终端标题、超链接）。
//
// 不清洗会有三处坏结果：
//  1. 导出文件里混入 `\u001b[…` 甚至裸 `\r`，用 grep / jq / 文本编辑器都难以阅读；
//  2. 界面渲染是「正文 + lipgloss 着色」，正文再带自己的颜色时颜色错乱；
//  3. `\r` 返回行首会让「按显示宽度折行」的宽度计算与实际占据列数不符，界面错位。
//
// 实现顺序有讲究（踩过坑）：
//   - `ansi.Strip` 实测**保留** `\r`（它只剥转义序列），所以 `\r` 必须由本函数自己消费；
//   - 清理其余控制符时**必须显式放行 `\r`**，否则"进度行"会被拼成一整行、丢掉分行信息；
//   - 先剥序列再按 `\r` 分行：顺序反了会把 `\x1b[2K`（清行）截断，留下残缺的 `[2K` 当正文。
//
// 返回切片：正文可能被 `\r` 拆成多行（进度行的历史片段本身就是有价值的日志）。
func ScrubLogText(raw string) []string {
	if raw == "" {
		return nil
	}
	clean := ansi.Strip(raw)
	// 去掉无显示意义的控制符（\x00-\x08、\x0b、\x0c、\x0e-\x1f、\x7f）。
	// 注意：\n 与 \t 有意义需保留；\r 也要放行——它是下面分行的依据，
	// 若在此处删掉，"10%\r55%\r完成" 会变成一行 "10%55%完成"。
	clean = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' || r == '\r' || r >= 0x20 {
			return r
		}
		return -1
	}, clean)

	parts := strings.Split(clean, "\r")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		// \r 拆分处常残留空格（原地刷新时常先写空格覆盖旧内容）
		if p = strings.TrimRight(p, " \t"); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ScrubSingleLine 清洗单行日志正文（取清洗后的首行；无内容时返回空串）。
func ScrubSingleLine(raw string) string {
	if lines := ScrubLogText(raw); len(lines) > 0 {
		return lines[0]
	}
	return ""
}
