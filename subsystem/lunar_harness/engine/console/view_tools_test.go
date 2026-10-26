package console

// view_tools_test.go 钉住「工具列表」屏的扫描目录展示与 B 键归属。

import (
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/engine/agentinfo"
)

func TestMenuKeyBPointsToTools(t *testing.T) {
	idx := menuHotkeyIndex("b")
	if idx < 0 {
		t.Fatal("快捷键 b 未绑定任何条目")
	}
	if menuItems[idx].screen != screenTools || menuItems[idx].title != "工具列表" {
		t.Fatalf("b 键应进入工具列表，实际 %q / %v", menuItems[idx].title, menuItems[idx].screen)
	}
}

func TestToolsScreenRendersCatalog(t *testing.T) {
	SetToolCatalogHook(func() []agentinfo.Agent {
		return []agentinfo.Agent{{
			ID: "demo", Name: "演示智能体", Source: "agents/Demo",
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
