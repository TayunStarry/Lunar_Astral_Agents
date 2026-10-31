package LunarPersonality

// network_explore_tool_test.go 钉住联网探索分类与生活链触发响应。

import (
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/life"
)

func TestNetworkExploreExposesLifeCategories(t *testing.T) {
	enum := networkExploreEnum(t)
	for _, want := range []string{"web_search", "bilibili_viewer", "mutual_forum"} {
		if !enumContains(t, enum, want) {
			t.Fatalf("联网探索应始终暴露 %s，实际 %v", want, enum)
		}
	}
}

func TestNetworkExploreLifeTriggersFixedReply(t *testing.T) {
	oldDir := config.LocalDir
	config.LocalDir = t.TempDir()
	life.Init()
	t.Cleanup(func() {
		// 用全新目录重新载入生活状态，清掉测试留下的待执行事件，避免污染其他用例。
		config.LocalDir = t.TempDir()
		life.Init()
		config.LocalDir = oldDir
	})

	cases := []struct {
		tool  string
		reply string
	}{
		{networkBilibiliTool, bilibiliFixedReply},
		{networkMutualForumTool, forumFixedReply},
	}
	for _, tc := range cases {
		result := handleNetworkExplore(map[string]any{
			"tool":        tc.tool,
			"instruction": "想看点猫咪视频",
		})
		if result.Text != tc.reply {
			t.Fatalf("%s 应返回固定响应 %q，实际 %q", tc.tool, tc.reply, result.Text)
		}
		if !life.HasPendingEvent() {
			t.Fatalf("%s 触发后应登记强制生活事件", tc.tool)
		}
	}
}

func TestNetworkExploreUnknownTool(t *testing.T) {
	result := handleNetworkExplore(map[string]any{"tool": "unknown", "instruction": "随便看看"})
	if !strings.Contains(result.Text, "web_search") {
		t.Fatalf("未知工具应给出可选分类，实际 %q", result.Text)
	}
}

func networkExploreEnum(t *testing.T) []string {
	t.Helper()
	tools := networkExploreToolCalls()
	if len(tools) != 1 {
		t.Fatalf("聚合工具数量应为 1，实际 %d", len(tools))
	}
	params, ok := tools[0].Function.Parameters.(map[string]any)
	if !ok {
		t.Fatalf("工具参数 Schema 类型异常: %T", tools[0].Function.Parameters)
	}
	properties := params["properties"].(map[string]any)
	toolProp := properties["tool"].(map[string]any)
	enum, ok := toolProp["enum"].([]string)
	if !ok {
		t.Fatalf("tool 枚举类型异常: %T", toolProp["enum"])
	}
	return enum
}

func enumContains(t *testing.T, values []string, target string) bool {
	t.Helper()
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
