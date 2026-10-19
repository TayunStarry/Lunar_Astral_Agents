package console

// ==== 颜色能力与渲染器 ====
//
// 界面「一片纯白」的根因就在这里，务必读完再改（详见 docs/adr/0002-*）。
//
// lipgloss 的 Style 是**结构体值**，在 `lipgloss.NewStyle()` 那一刻就把当时的渲染器
// 指针记进了 `Style.r`（见 lipgloss/style.go 的 `if s.r == nil { s.r = renderer }`）。
// 事后调 `lipgloss.SetDefaultRenderer(...)` 只替换了包级变量，**已经建好的样式仍然
// 指向旧渲染器**。而旧渲染器的输出是 init 时的 `os.Stdout`——Install() 会把进程级
// 标准句柄与 CRT 的 fd 1/2 一并接到日志管道上（pipeline.go / stdio_capture_windows.go），
// 于是 termenv 的 isatty 判定变假 → `colorprofile` 退化为 Ascii → 所有 SGR 序列被丢弃，
// 整个界面只剩默认前景色，也就是「纯白」。
//
// 两条结论（本文件的全部内容都服务于它们）：
//  1. 颜色档位**不能靠检测**：检测对象是「当前 stdout 是不是 TTY」，而 stdout 恰恰被
//     自己接管了。必须在对的控制台上探测能力，然后 `SetColorProfile` 显式写入。
//  2. 探测结果必须**重绑到每一条样式**（theme.go 的 bindTheme），否则样式仍在用旧渲染器。

import (
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// colorModeEnv 显式指定颜色档位的环境变量（排障与录屏时需要确定性输出）
const colorModeEnv = "LUNAR_TUI_COLOR"

// termCaps 渲染目标的颜色能力（由各平台文件填充）
type termCaps struct {
	TTY       bool // 渲染目标是不是真正的控制台
	VT        bool // 是否启用了 ANSI/VT 转义处理（没开时写入 SGR 会变成可见乱码）
	TrueColor bool // 是否可信 24 位色（Windows Terminal / COLORTERM=truecolor）
}

// profileFromEnv 读取显式指定的颜色档位（LUNAR_TUI_COLOR）
//
// 取值：dumb/ascii/none → 无色；16/ansi → 16 色；256 → 256 色；truecolor/24bit → 24 位色。
func profileFromEnv() (termenv.Profile, bool) {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(colorModeEnv))) {
	case "dumb", "ascii", "none", "off", "no":
		return termenv.Ascii, true
	case "16", "ansi", "basic":
		return termenv.ANSI, true
	case "256", "ansi256", "8bit":
		return termenv.ANSI256, true
	case "truecolor", "24bit", "24", "true":
		return termenv.TrueColor, true
	}
	return termenv.Ascii, false
}

// envNoColor 是否按环境约定禁用颜色（https://no-color.org/ 与 CLICOLOR）
func envNoColor() bool {
	if os.Getenv("NO_COLOR") != "" {
		return true
	}
	return os.Getenv("CLICOLOR") == "0" && os.Getenv("CLICOLOR_FORCE") == ""
}

// colorProfileFor 决策颜色档位：显式指定 > 环境约定 > 终端实测能力。
// 第二个返回值为判定依据，会写进 console.log——「界面没颜色」类反馈的第一手证据。
func colorProfileFor(caps termCaps) (termenv.Profile, string) {
	if p, ok := profileFromEnv(); ok {
		return p, "环境变量 " + colorModeEnv
	}
	if envNoColor() {
		return termenv.Ascii, "环境变量 NO_COLOR / CLICOLOR=0"
	}
	switch {
	case !caps.TTY:
		return termenv.Ascii, "渲染目标不是控制台（输出被接管或重定向）"
	case !caps.VT:
		return termenv.Ascii, "控制台未启用 ANSI 转义处理（开了也没用，只会显示乱码）"
	case caps.TrueColor:
		return termenv.TrueColor, "终端支持 24 位色"
	default:
		// 色板全部是 256 色索引，ANSI256 已能完整表达；conhost 的 24 位色支持并不统一，
		// 因此这里刻意不上调（上了反而在部分 conhost 上出杂色）。
		return termenv.ANSI256, "终端支持 256 色"
	}
}

// setupRenderer 建立界面渲染器，并把颜色档位显式写死。
//
// 为什么用 `SetColorProfile` 而不是听任渲染器自己检测：检测读的是「渲染目标的当前状态」，
// 而界面的中文控制台句柄是我们自己另取的（console.go），标准流已被日志管道占用；
// 让检测决定档位，就等于让「stdout 被接管」这件事把整套配色关掉。
func setupRenderer(w io.Writer) (*lipgloss.Renderer, string) {
	profile, why := colorProfileFor(detectTermCaps(w))
	r := lipgloss.NewRenderer(w)
	r.SetColorProfile(profile)
	return r, profile.Name() + "（" + why + "）"
}

// activateTheme 切换全局渲染器并把全部样式重绑到它上面。
// 返回颜色档位说明（供日志留痕）。必须在 tea 程序启动前调用。
func activateTheme(w io.Writer) string {
	r, diag := setupRenderer(w)
	lipgloss.SetDefaultRenderer(r)
	bindTheme(r)
	return diag
}
