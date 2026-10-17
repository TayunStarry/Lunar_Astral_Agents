package TuiSystem

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// 覆盖：颜色档位的决策顺序——显式指定 > 环境约定 > 终端实测能力。
//
// 这是「界面一片纯白」的回归防线：旧实现让 lipgloss 自己检测，而界面的输出句柄
// 早已被日志管线接管（不是 TTY），于是永远退化为 Ascii、整套配色被丢弃。
func TestColorProfileDecision(t *testing.T) {
	console := termCaps{TTY: true, VT: true, TrueColor: false}
	cases := []struct {
		name string
		caps termCaps
		env  map[string]string
		want string // termenv.Profile.Name()
	}{
		{"控制台 + 256 色", console, nil, "ANSI256"},
		{"Windows Terminal 的 24 位色", termCaps{TTY: true, VT: true, TrueColor: true}, nil, "TrueColor"},
		{"输出被接管（非 TTY）", termCaps{}, nil, "Ascii"},
		{"控制台未开 VT", termCaps{TTY: true}, nil, "Ascii"},
		{"NO_COLOR 优先于能力", termCaps{TTY: true, VT: true, TrueColor: true}, map[string]string{"NO_COLOR": "1"}, "Ascii"},
		{"CLICOLOR=0", termCaps{TTY: true, VT: true}, map[string]string{"CLICOLOR": "0"}, "Ascii"},
		{"CLICOLOR=0 可被 FORCE 覆盖", termCaps{TTY: true, VT: true}, map[string]string{"CLICOLOR": "0", "CLICOLOR_FORCE": "1"}, "ANSI256"},
		{"显式 16 色", termCaps{TTY: true, VT: true, TrueColor: true}, map[string]string{colorModeEnv: "16"}, "ANSI"},
		{"显式关闭", termCaps{TTY: true, VT: true, TrueColor: true}, map[string]string{colorModeEnv: "dumb"}, "Ascii"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// 先清空所有相关环境变量：测试进程本身可能带着 NO_COLOR（本仓开发环境就有），
			// 不清会把「没设置」的用例误判成「被环境禁用」。
			for _, k := range []string{"NO_COLOR", "CLICOLOR", "CLICOLOR_FORCE", colorModeEnv} {
				t.Setenv(k, "")
			}
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			got, why := colorProfileFor(tc.caps)
			if got.Name() != tc.want {
				t.Fatalf("档位不符：期望 %s，实得 %s（依据: %s）", tc.want, got.Name(), why)
			}
			if why == "" {
				t.Fatal("判定依据不该为空：它就是「界面没颜色」时唯一的现场证据")
			}
		})
	}
}

// 覆盖：样式必须真的按显式档位输出 ANSI。
//
// 直接复现历史缺陷的形态——渲染目标不是 TTY（输出被接管）时，
// 只要档位被显式写死，界面就必须仍然是彩色的。
func TestSetupRendererKeepsColorOnPipedOutput(t *testing.T) {
	t.Setenv(colorModeEnv, "truecolor")
	var buf strings.Builder
	r, diag := setupRenderer(&buf)
	if got := r.ColorProfile().Name(); got != "TrueColor" {
		t.Fatalf("渲染器档位应被显式写死为 TrueColor，实得 %s（正文：%s）", got, diag)
	}
	bindTheme(r)
	out := titleStyle.Render("标题")
	if !strings.Contains(out, "\x1b[") {
		t.Fatalf("已绑定彩色档位的样式输出里没有 ANSI 序列: %q", out)
	}
}

// 覆盖：bindTheme 必须覆盖**每一条**样式。
//
// theme.go 的样式变量没有初值，靠 bindTheme 赋值；漏掉一条不会编译失败，
// 只会让那一处「没颜色」——正是这类缺陷最难被发现的地方。
func TestBindThemeCoversEveryStyle(t *testing.T) {
	t.Setenv(colorModeEnv, "truecolor")
	r, _ := setupRenderer(&strings.Builder{})
	bindTheme(r)

	styles := map[string]lipgloss.Style{
		"titleStyle": titleStyle, "crumbStyle": crumbStyle, "labelStyle": labelStyle,
		"valueStyle": valueStyle, "dimStyle": dimStyle, "faintStyle": faintStyle,
		"selectStyle": selectStyle, "okStyle": okStyle, "warnStyle": warnStyle,
		"errStyle": errStyle, "cursorStyle": cursorStyle, "ruleStyle": ruleStyle,
		"keyStyle": keyStyle, "hintStyle": hintStyle, "sectionStyle": sectionStyle,
		"selRowStyle": selRowStyle, "selDimStyle": selDimStyle,
		"navGroupStyle": navGroupStyle, "navItemStyle": navItemStyle, "navKeyStyle": navKeyStyle,
		"logInfoStyle": logInfoStyle, "logDimStyle": logDimStyle, "logProxyStyle": logProxyStyle,
		"logWarnStyle": logWarnStyle, "logErrStyle": logErrStyle, "thinkStyle": thinkStyle,
		"thinkHeadStyle": thinkHeadStyle, "roleStyle": roleStyle, "drawProgressStyle": drawProgressStyle,
	}
	for name, st := range styles {
		if out := st.Render("x"); !strings.Contains(out, "\x1b[") {
			t.Fatalf("%s 未绑定到彩色渲染器（输出 %q）——请把它写进 theme.go 的 bindTheme", name, out)
		}
	}
}

// 覆盖：NO_COLOR 下界面仍然可用（只是无色），且不会残留半截 ANSI。
func TestThemeUnderNoColorStaysPlain(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	r, diag := setupRenderer(&strings.Builder{})
	if got := r.ColorProfile().Name(); got != "Ascii" {
		t.Fatalf("NO_COLOR 下应为 Ascii，实得 %s（%s）", got, diag)
	}
	bindTheme(r)
	if out := titleStyle.Render("标题"); strings.Contains(out, "\x1b[") {
		t.Fatalf("NO_COLOR 下不该有 ANSI 序列: %q", out)
	}
	out := newRow(20, true).L("▸ ", "条目").String()
	if strings.Contains(out, "\x1b[") {
		t.Fatalf("选中行不该有 ANSI 序列: %q", out)
	}
	if w := dispWidth(out); w != 20 {
		t.Fatalf("无色档位下行宽仍须为 20，实得 %d", w)
	}
}
