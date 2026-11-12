package console

// view_tools_test.go 钉住「工具列表」屏的扫描目录展示，以及主菜单分组顺序与快捷键连续性。

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"LunarSubsystem/LunarHarness/protocol/agentinfo"
)

// TestMenuGroupsAndKeysSequential 主菜单：分组顺序固定；功能屏快捷键按显示顺序连续占用 0-9；
// 退出项是哨兵、不占项号，界面统一显示 X（大小写 x/X 仍都可按）。
func TestMenuGroupsAndKeysSequential(t *testing.T) {
	wantGroups := []string{"运行观测", "系统管理", "退出"}
	gotGroups := make([]string, 0, len(wantGroups))
	for _, item := range menuItems {
		if len(gotGroups) == 0 || gotGroups[len(gotGroups)-1] != item.group {
			gotGroups = append(gotGroups, item.group)
		}
	}
	if strings.Join(gotGroups, ">") != strings.Join(wantGroups, ">") {
		t.Fatalf("分组顺序应为 %v，实际 %v", wantGroups, gotGroups)
	}

	const keys = "0123456789"
	seq := 0
	for i, item := range menuItems {
		if item.screen == screenMenu {
			// 退出哨兵：界面标签必须是单个 X，且数字 8 不再命中退出项
			if item.key != "X" {
				t.Fatalf("退出项快捷键标签应为 %q，实际 %q", "X", item.key)
			}
			if idx := menuHotkeyIndex("8"); idx >= 0 && menuItems[idx].screen == screenMenu {
				t.Fatal("数字 8 不应再命中退出项")
			}
			if idx := menuHotkeyIndex(item.key); idx != i {
				t.Fatalf("%q 应命中第 %d 项，实际 %d", item.key, i, idx)
			}
			continue
		}
		if seq >= len(keys) {
			t.Fatalf("菜单条目超过快捷键容量（%d）", len(keys))
		}
		if want := string(keys[seq]); item.key != want {
			t.Fatalf("第 %d 项「%s」的快捷键应为 %q，实际 %q", i, item.title, want, item.key)
		}
		if idx := menuHotkeyIndex(item.key); idx != i {
			t.Fatalf("%q 应命中第 %d 项，实际 %d", item.key, i, idx)
		}
		seq++
	}

	// 工具列表仍在系统管理组内，且按键能正确回到该条目
	for i, item := range menuItems {
		if item.screen != screenTools || item.title != "工具列表" {
			continue
		}
		if item.group != "系统管理" {
			t.Fatalf("工具列表应归入系统管理，实际 %q", item.group)
		}
		if idx := menuHotkeyIndex(item.key); idx != i {
			t.Fatalf("工具列表快捷键 %q 应命中第 %d 项，实际 %d", item.key, i, idx)
		}
		return
	}
	t.Fatal("菜单里找不到工具列表条目")
}

// TestMenuQuitItemShowsXKey 退出项在主菜单、侧栏与详情栏都必须显示单个 X，不再显示旧项号 8 或 x/X。
func TestMenuQuitItemShowsXKey(t *testing.T) {
	t.Setenv(colorModeEnv, "")

	m := newAppModel()
	m.width, m.height = 120, 36
	m.screen = screenMenu
	m.menu = menuHotkeyIndex("X")
	out := ansi.Strip(m.View())
	if !strings.Contains(out, "[X]") || !strings.Contains(out, "退出程序") {
		t.Fatalf("主菜单退出项应显示 [X] 退出程序:\n%s", out)
	}
	if strings.Contains(out, "[8]") || strings.Contains(out, "x/X") {
		t.Fatalf("主菜单不应再出现旧的退出项号 [8] 或 x/X:\n%s", out)
	}

	// 侧栏与主菜单同源，退出项同样应显示 [X]
	m.screen = screenAgent
	nav := ansi.Strip(strings.Join(m.navLines(24, 24), "\n"))
	if !strings.Contains(nav, "[X]") {
		t.Fatalf("侧栏退出项应显示 [X]:\n%s", nav)
	}
	if strings.Contains(nav, "[8]") || strings.Contains(nav, "x/X") {
		t.Fatalf("侧栏不应再出现旧的退出项号 [8] 或 x/X:\n%s", nav)
	}
}

func TestToolsScreenRendersCatalog(t *testing.T) {
	SetToolCatalogHook(func() []agentinfo.Agent {
		return []agentinfo.Agent{{
			ID: "demo", Name: "演示智能体",
			Tools: []agentinfo.Tool{
				{Name: "demo_tool", Description: "这是一个用于验证工具列表展示的演示工具。"},
			},
		}}
	})
	defer SetToolCatalogHook(nil)

	m := newAppModel()
	m.width, m.height = 120, 36
	m.screen = screenTools
	m.toolsEnter()
	out := m.View()
	for _, want := range []string{"工具列表", "演示智能体", "demo_tool", "演示工具", "1 个智能体 · 1 个工具"} {
		if !strings.Contains(out, want) {
			t.Fatalf("工具列表缺少 %q:\n%s", want, out)
		}
	}
}

func TestToolsScreenCountsOnlyRealAgents(t *testing.T) {
	SetToolCatalogHook(func() []agentinfo.Agent {
		return []agentinfo.Agent{
			{
				ID: "demo", Name: "演示智能体", Kind: agentinfo.KindAgent,
				Tools: []agentinfo.Tool{{Name: "demo_tool", Description: "演示工具。"}},
			},
			{
				ID: "creative", Name: "自由创作", Kind: agentinfo.KindChain,
				Tools: []agentinfo.Tool{{Name: "creator", Description: "创作链工具。"}},
			},
			{
				ID: "archive_reader", Name: "归档解析", Kind: agentinfo.KindExecutor,
				Tools: []agentinfo.Tool{{Name: "archive_list", Description: "归档解析工具。"}},
			},
		}
	})
	defer SetToolCatalogHook(nil)

	m := newAppModel()
	m.width, m.height = 140, 40
	m.screen = screenTools
	m.toolsEnter()
	out := m.View()
	// 链与执行体不计入智能体数量，但仍完整展示
	for _, want := range []string{"1 个智能体", "3 个工具", "1 条链", "1 个执行体", "自由创作", "归档解析"} {
		if !strings.Contains(out, want) {
			t.Fatalf("工具列表缺少 %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "3 个智能体") {
		t.Fatalf("链与执行体不应计入智能体数量:\n%s", out)
	}
}

func TestToolsScreenWithoutHook(t *testing.T) {
	SetToolCatalogHook(nil)
	m := newAppModel()
	m.width, m.height = 120, 34
	m.screen = screenTools
	m.toolsEnter()
	out := m.View()
	if !strings.Contains(out, "宿主未接入") {
		t.Fatalf("未接入目录钩子时应明确提示:\n%s", out)
	}
}
