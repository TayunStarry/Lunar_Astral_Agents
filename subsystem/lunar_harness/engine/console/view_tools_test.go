package console

// view_tools_test.go 钉住「工具列表」屏的扫描目录展示，以及主菜单分组顺序与快捷键连续性。

import (
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/protocol/agentinfo"
)

// TestMenuGroupsAndKeysSequential 主菜单：分组顺序固定，快捷键按显示顺序连续占用 0-9、a-z
func TestMenuGroupsAndKeysSequential(t *testing.T) {
	wantGroups := []string{"运行观测", "系统管理", "更新与发布", "会话"}
	gotGroups := make([]string, 0, len(wantGroups))
	for _, item := range menuItems {
		if len(gotGroups) == 0 || gotGroups[len(gotGroups)-1] != item.group {
			gotGroups = append(gotGroups, item.group)
		}
	}
	if strings.Join(gotGroups, ">") != strings.Join(wantGroups, ">") {
		t.Fatalf("分组顺序应为 %v，实际 %v", wantGroups, gotGroups)
	}

	const keys = "0123456789abcdefghijklmnopqrstuvwxyz"
	for i, item := range menuItems {
		if i >= len(keys) {
			t.Fatalf("菜单条目超过快捷键容量（%d）", len(keys))
		}
		if want := string(keys[i]); item.key != want {
			t.Fatalf("第 %d 项「%s」的快捷键应为 %q，实际 %q", i, item.title, want, item.key)
		}
		if idx := menuHotkeyIndex(item.key); idx != i {
			t.Fatalf("%q 应命中第 %d 项，实际 %d", item.key, i, idx)
		}
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
