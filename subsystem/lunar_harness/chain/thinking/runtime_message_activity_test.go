package thinking

import (
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/engine/activity"
	"LunarSubsystem/LunarHarness/engine/config"
)

// TestRuntimeMessageCarriesActivity 每轮运行时消息都带固定活动字段，且随写入刷新。
func TestRuntimeMessageCarriesActivity(t *testing.T) {
	oldDir := config.LocalDir
	config.LocalDir = t.TempDir()
	t.Cleanup(func() { config.LocalDir = oldDir })

	activity.Set("正在整理今天的观影笔记")
	first, _ := buildRuntimeGlobalMessage().Content.(string)
	if !strings.Contains(first, "我刚刚在干的事情：正在整理今天的观影笔记") {
		t.Fatalf("运行时消息未携带活动字段：%s", first)
	}

	activity.Set("正在和论坛里的朋友聊天")
	second, _ := buildRuntimeGlobalMessage().Content.(string)
	if !strings.Contains(second, "我刚刚在干的事情：正在和论坛里的朋友聊天") {
		t.Fatalf("运行时消息未刷新为最新活动：%s", second)
	}
}
