package console

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// 全部屏幕的清单：新增屏幕时请一并加进来，宽度不变式对它们同样生效。
func allScreens() []screenID {
	return []screenID{
		screenMenu, screenAgent, screenStats, screenLogs,
		screenProxy, screenArchive, screenPorts, screenDump, screenTools,
	}
}

// 覆盖：界面骨架的硬不变式——**整屏高度恒等于终端高度，每行恒为「宽度-1」列**。
//
// 为什么按宽度-1：经典 conhost 写满整行时换行会吞掉最后一个字符（实测：右角 ╮/╯ 消失）。
// 只要这两条成立，界面就永远不会把内容顶出终端、也永远不会因超宽而折行滚屏——
// 那正是用户实测过的「一打开就滚动 / 显示混乱」。
func TestFrameNeverExceedsTerminal(t *testing.T) {
	t.Setenv(colorModeEnv, "truecolor")
	r, _ := setupRenderer(&strings.Builder{})
	bindTheme(r)

	sizes := [][2]int{{44, 10}, {80, 24}, {96, 30}, {120, 40}, {200, 50}}
	for _, size := range sizes {
		w, h := size[0], size[1]
		for _, id := range allScreens() {
			for _, help := range []bool{false, true} {
				m := newAppModel()
				m.width, m.height = w, h
				m.screen = id
				m.help = help
				lines := strings.Split(m.View(), "\n")
				if len(lines) != h {
					t.Fatalf("%dx%d screen=%d help=%v: 渲染 %d 行，应为 %d 行", w, h, id, help, len(lines), h)
				}
				for i, line := range lines {
					if got := dispWidth(line); got != w-1 {
						t.Fatalf("%dx%d screen=%d help=%v: 第 %d 行宽 %d 列，应为 %d 列\n%s",
							w, h, id, help, i+1, got, w-1, line)
					}
				}
			}
		}
	}
}

// 覆盖：侧栏的显隐规则——主菜单不需要（它本身就是导航）、窄终端不显示、
// 宽屏功能屏显示，且显示时内容区确实变窄（否则会与外框撞上）。
func TestSidebarResponsive(t *testing.T) {
	cases := []struct {
		screen screenID
		w, h   int
		want   bool
	}{
		{screenMenu, 200, 40, false},
		{screenStats, 200, 40, true},
		{screenStats, navShowFrom, 30, true},
		{screenStats, navShowFrom - 1, 30, false},
		{screenStats, 120, navMinHeight - 1, false},
	}
	for _, tc := range cases {
		m := newAppModel()
		m.width, m.height = tc.w, tc.h
		m.screen = tc.screen
		if got := m.navVisible(); got != tc.want {
			t.Fatalf("screen=%d %dx%d: navVisible=%v，应为 %v", tc.screen, tc.w, tc.h, got, tc.want)
		}
		f := m.frameFor("x", "", nil)
		bodyW, _ := f.bodySize()
		if tc.want {
			if f.navW <= 0 {
				t.Fatalf("宽屏应给出侧栏宽度，实得 %d", f.navW)
			}
			fullW, _ := (frame{width: tc.w, height: tc.h}).bodySize()
			if bodyW >= fullW {
				t.Fatalf("显示侧栏后内容区应变窄：%d >= %d", bodyW, fullW)
			}
		} else if bodyW != tc.w-3 {
			t.Fatalf("无侧栏时内容区应为 %d 列，实得 %d", tc.w-3, bodyW)
		}
	}
}

// 覆盖：侧栏条目与主菜单同源——每一项（除退出哨兵）都必须能被 [ / ] 顺序访问，
// 并且当前屏在侧栏里处于高亮态所需的定位信息（menuIndexOf）可用。
func TestSidebarMirrorsMenu(t *testing.T) {
	navs := navigableScreens()
	if want := len(menuItems) - 1; len(navs) != want {
		t.Fatalf("[ / ] 可切换屏数 %d，应为菜单条目数-1 = %d", len(navs), want)
	}
	seen := map[screenID]bool{}
	for _, id := range navs {
		if id == screenMenu {
			t.Fatal("退出哨兵不应当作可切换屏")
		}
		if seen[id] {
			t.Fatalf("屏幕 %d 在侧栏里重复出现", id)
		}
		seen[id] = true
		if menuIndexOf(id) < 0 {
			t.Fatalf("屏幕 %d 无法在菜单表中定位", id)
		}
	}
	// 分组面包屑：功能屏都应有分组名
	for _, id := range navs {
		m := newAppModel()
		m.screen = id
		if m.screenGroup() == "" {
			t.Fatalf("屏幕 %d 缺少面包屑分组", id)
		}
	}
	m := newAppModel()
	m.screen = screenMenu
	if m.screenGroup() != "" {
		t.Fatalf("主菜单不该有面包屑分组，实得 %q", m.screenGroup())
	}
}

// 覆盖：顺序切屏的边界行为——首尾环绕，且在主菜单上 [ 落到末屏、] 落到首屏。
func TestSwitchScreenCycles(t *testing.T) {
	navs := navigableScreens()

	m := newAppModel()
	if id := func() screenID { m.switchScreen(1); return m.screen }(); id != navs[0] {
		t.Fatalf("主菜单按 ] 应进入首屏 %d，实得 %d", navs[0], id)
	}

	m = newAppModel()
	m.switchScreen(-1)
	if m.screen != navs[len(navs)-1] {
		t.Fatalf("主菜单按 [ 应进入末屏 %d，实得 %d", navs[len(navs)-1], m.screen)
	}

	m = newAppModel()
	m.screen = navs[len(navs)-1]
	m.switchScreen(1)
	if m.screen != navs[0] {
		t.Fatalf("末屏按 ] 应环绕回首屏 %d，实得 %d", navs[0], m.screen)
	}

	// 进入功能屏时主菜单高亮要跟着走，否则「Esc 回主菜单」会停在旧位置
	m = newAppModel()
	m.switchScreen(1)
	if want := menuIndexOf(m.screen); m.menu != want {
		t.Fatalf("进入屏幕后主菜单下标应同步为 %d，实得 %d", want, m.menu)
	}
}

// 覆盖：空标题的分隔线不会留下「──  ────」空档（导出屏的分组标题可能就是空的）。
func TestSectionRuleEmptyTitle(t *testing.T) {
	out := sectionRule("", 20)
	if w := dispWidth(out); w != 20 {
		t.Fatalf("分隔线宽 %d 列，应为 20", w)
	}
	if strings.Contains(out, "──  ") {
		t.Fatalf("空标题不该产生「──  」前缀: %q", out)
	}
}

// 覆盖：窄终端底栏被压缩时，「? 帮助」仍然保留在最前，帮助入口不会随截断消失。
func TestAgentFooterHelpReachable(t *testing.T) {
	t.Setenv(colorModeEnv, "")
	for _, size := range [][2]int{{44, 10}, {80, 24}} {
		m := newAppModel()
		m.width, m.height = size[0], size[1]
		m.screen = screenAgent

		lines := strings.Split(m.View(), "\n")
		if foot := ansi.Strip(lines[len(lines)-1]); !strings.Contains(foot, "? 帮助") {
			t.Fatalf("%dx%d 底栏缺少帮助入口: %q", size[0], size[1], foot)
		}
	}
}

// 覆盖：实况屏的分栏阈值——宽屏把状态单独成栏，窄屏退回单栏并把状态放进底栏。
func TestAgentStatusRailResponsive(t *testing.T) {
	t.Setenv(colorModeEnv, "")
	resetTurnStore()
	defer resetTurnStore()
	StreamBegin("对话者")
	StreamDelta("对话者", KindThink, "先看问题，再决定是否调用工具。")
	StreamTool("对话者", "dispatch_painter", `{"character":"琉璃"}`)
	StreamDelta("对话者", KindContent, "画好了。")
	StreamEnd("对话者")

	wide := newAppModel()
	wide.width, wide.height = 120, 40
	wide.screen = screenAgent
	wideOut := wide.View()
	if !strings.Contains(wideOut, "状态") || !strings.Contains(wideOut, "轮次") {
		t.Fatal("宽屏实况屏应出现独立的状态栏")
	}

	narrow := newAppModel()
	narrow.width, narrow.height = 80, 24
	narrow.screen = screenAgent
	narrowLines := strings.Split(narrow.View(), "\n")
	if strings.Contains(strings.Join(narrowLines, "\n"), "── 状态") {
		t.Fatal("窄屏不应强行分栏")
	}
	foot := ansi.Strip(narrowLines[len(narrowLines)-1])
	if !strings.Contains(foot, "行") {
		t.Fatalf("窄屏状态应退回底栏，实得: %q", foot)
	}
	// 窄屏是分栏状态栏的紧凑退化形态：用时与步骤仍要在底栏可见
	if !strings.Contains(foot, "上轮") || !strings.Contains(foot, "等待工具") {
		t.Fatalf("窄屏底栏应保留用时与步骤，实得: %q", foot)
	}
}

// 覆盖：状态栏的「用时 / 步骤」随流式事件推进——
// 进行中给「本轮」，工具未返回时保持「等待工具」，工具返回后才落到「上轮 / 已结束」。
func TestAgentStatusRailTracksPhase(t *testing.T) {
	t.Setenv(colorModeEnv, "")
	resetTurnStore()
	defer resetTurnStore()

	StreamBegin("对话者")
	StreamDelta("对话者", KindThink, "先想想。")

	m := newAppModel()
	m.width, m.height = 120, 40
	m.screen = screenAgent
	out := ansi.Strip(m.View())
	for _, want := range []string{"用时", "步骤", "本轮", "思考中"} {
		if !strings.Contains(out, want) {
			t.Fatalf("进行中应显示 %q，实得:\n%s", want, out)
		}
	}

	StreamTool("对话者", "dispatch_painter", "{}")
	if out := ansi.Strip(m.View()); !strings.Contains(out, "等待工具") {
		t.Fatalf("工具调用后应显示等待工具，实得:\n%s", out)
	}

	StreamEnd("对话者")
	if out := ansi.Strip(m.View()); !strings.Contains(out, "等待工具") {
		t.Fatalf("工具尚未返回时应保持等待工具，实得:\n%s", out)
	}

	StreamToolResult("对话者", "绘制完成")
	out = ansi.Strip(m.View())
	for _, want := range []string{"上轮", "已结束"} {
		if !strings.Contains(out, want) {
			t.Fatalf("工具返回后应显示 %q，实得:\n%s", want, out)
		}
	}
}

// 覆盖：键位提示放不下时的取舍顺序——右侧状态优先，键位可压缩，但两侧之间
// 必须留下可见的填充（否则框线看起来是断开的）。
func TestComposeRuleKeepsGap(t *testing.T) {
	left := strings.Repeat("键", 10)  // 20 列
	right := strings.Repeat("值", 10) // 20 列
	out := composeRule("╭", "╮", left, right, "─", 40)
	if w := dispWidth(out); w != 40 {
		t.Fatalf("行宽 %d，应为 40: %q", w, out)
	}
	if !strings.Contains(out, "─") {
		t.Fatalf("两侧都有内容时应保留填充线: %q", out)
	}
	// 压缩到极限时至少不能超宽
	for _, width := range []int{2, 3, 10, 20, 30} {
		out := composeRule("╭", "╮", left, right, "─", width)
		if w := dispWidth(out); w > width {
			t.Fatalf("width=%d 时行宽 %d 超宽", width, w)
		}
	}
}
