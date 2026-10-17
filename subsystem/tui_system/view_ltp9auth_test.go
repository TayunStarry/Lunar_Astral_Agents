package TuiSystem

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// 覆盖：授权状态的**配色映射**——本屏的核心要求是「用颜色区分哪些 LTP9 包没签发、
// 哪些签发过了」，且要区分「密钥与脚本已对不上（需重签）」与「从未签发」两种不同的处置。
func TestLTP9AuthLabelStates(t *testing.T) {
	cases := []struct {
		name  string
		pkg   ltp9AuthPkg
		want  string
		style string // 期望命中的样式（用同一段文本渲染后比对，避免比较样式对象本身）
	}{
		{
			name:  "读取失败",
			pkg:   ltp9AuthPkg{Err: errNotWired},
			want:  "不可读取",
			style: "err",
		},
		{
			name:  "无入口脚本",
			pkg:   ltp9AuthPkg{Auth: LTP9AuthStatus{Required: []string{"allow-file"}}},
			want:  "无入口脚本",
			style: "err",
		},
		{
			name:  "只使用常驻能力",
			pkg:   ltp9AuthPkg{Auth: LTP9AuthStatus{HasScript: true}},
			want:  "无需签发",
			style: "faint",
		},
		{
			name:  "从未签发",
			pkg:   ltp9AuthPkg{Auth: LTP9AuthStatus{HasScript: true, Required: []string{"allow-file"}}},
			want:  "未签发",
			style: "warn",
		},
		{
			name:  "密钥与脚本已不对应",
			pkg:   ltp9AuthPkg{Auth: LTP9AuthStatus{HasScript: true, HasKey: true, Required: []string{"allow-file"}}},
			want:  "需重签",
			style: "err",
		},
		{
			name: "已签发但权限不全",
			pkg: ltp9AuthPkg{Auth: LTP9AuthStatus{
				HasScript: true, HasKey: true, Signed: true,
				Granted: []string{"allow-file"}, Required: []string{"allow-file", "allow-network"},
			}},
			want:  "已签发·缺 allow-network",
			style: "warn",
		},
		{
			name: "已签发且覆盖引用权限",
			pkg: ltp9AuthPkg{Auth: LTP9AuthStatus{
				HasScript: true, HasKey: true, Signed: true,
				Granted: []string{"allow-file", "allow-network"}, Required: []string{"allow-file", "allow-network"},
			}},
			want:  "已签发",
			style: "ok",
		},
	}

	styles := map[string]lipgloss.Style{
		"ok":    okStyle,
		"warn":  warnStyle,
		"err":   errStyle,
		"faint": faintStyle,
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			label, style := ltp9AuthLabel(tc.pkg)
			if label != tc.want {
				t.Fatalf("标签不符：期望 %q，实得 %q", tc.want, label)
			}
			// 颜色断言：该样式渲染同一段文本必须与期望样式完全一致（含 ANSI 序列）
			probe := "状态"
			want := styles[tc.style].Render(probe)
			if got := style.Render(probe); got != want {
				t.Fatalf("配色不符：期望 %s 档（%q），实得 %q", tc.style, want, got)
			}
		})
	}
}

// 覆盖：候选筛选判据——只有 tags 含 LTP9（大小写不敏感）的包才进候选。
func TestLTP9Tagged(t *testing.T) {
	cases := []struct {
		tags []string
		want bool
	}{
		{nil, false},
		{[]string{}, false},
		{[]string{"Node-LTP", "LTP3"}, false},
		{[]string{"ltp9"}, true},
		{[]string{" LTP9 "}, true},
		{[]string{"绘画", "LTP9"}, true},
	}
	for _, tc := range cases {
		if got := ltp9Tagged(tc.tags); got != tc.want {
			t.Fatalf("ltp9Tagged(%v) = %v，期望 %v", tc.tags, got, tc.want)
		}
	}
}

// 覆盖：底栏键位数量受控——键位栏会与右侧状态争宽，条目一多就被截断（本仓既有教训）。
func TestLTP9AuthKeysStayCompact(t *testing.T) {
	if n := len(ltp9AuthKeys(ltp9AuthState{})); n > 4 {
		t.Fatalf("浏览态底栏键位过多（%d 条），会与右侧状态争宽被截断", n)
	}
	if n := len(ltp9AuthKeys(ltp9AuthState{mode: ltp9AuthConfirm})); n > 2 {
		t.Fatalf("确认态底栏键位过多（%d 条）", n)
	}
	if n := len(ltp9AuthKeys(ltp9AuthState{running: true})); n != 1 {
		t.Fatalf("进行中应只提示状态，实际 %d 条", n)
	}
}
