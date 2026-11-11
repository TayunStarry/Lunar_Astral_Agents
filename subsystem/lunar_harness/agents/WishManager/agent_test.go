package WishManager

// agent_test.go 愿望管理智能体的工具成功 / 失败 / 容量确认验收（不依赖模型）。

import (
	"context"
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/engine/wishchain"
	"LunarSubsystem/LunarHarness/internal/testenv"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

func callTool(t *testing.T, name, args string) Result {
	t.Helper()
	return executeTool(agentbase.ToolCall{
		ID:   "test",
		Type: "function",
		Function: agentbase.ToolFunction{
			Name:      name,
			Arguments: args,
		},
	})
}

// TestWishManagerToolLifecycle 新增 / 查询 / 改写 / 完成 / 删除的成功返回。
func TestWishManagerToolLifecycle(t *testing.T) {
	testenv.Isolate(t)
	wishchain.Reset()

	created := callTool(t, ToolAdd, `{"content":"看一部科幻电影","due":"2026-06-14 15:30"}`)
	if !created.OK || created.Item == nil || created.Item.ID == "" {
		t.Fatalf("新增应成功: %+v", created)
	}
	if created.Item.Priority != 0 {
		t.Fatalf("自动分配应从最高优先级 0 开始，实际 %d", created.Item.Priority)
	}
	if !strings.Contains(created.Summary, "看一部科幻电影") {
		t.Fatalf("新增简述应说明内容: %q", created.Summary)
	}

	queried := callTool(t, ToolQuery, `{"keywords":["科幻"]}`)
	if !queried.OK || !strings.Contains(queried.Detail["result"].(string), "看一部科幻电影") {
		t.Fatalf("查询应成功: %+v", queried)
	}

	updated := callTool(t, ToolUpdate, `{"id":"`+created.Item.ID+`","content":"看一部科幻悬疑电影"}`)
	if !updated.OK || updated.Item == nil || updated.Item.Content != "看一部科幻悬疑电影" {
		t.Fatalf("改写应成功: %+v", updated)
	}

	priority := callTool(t, ToolPriority, `{"id":"`+created.Item.ID+`","priority":7}`)
	if !priority.OK || priority.Item == nil || priority.Item.Priority != 7 {
		t.Fatalf("调整优先级应成功: %+v", priority)
	}

	completed := callTool(t, ToolComplete, `{"id":"`+created.Item.ID+`"}`)
	if !completed.OK || completed.Item == nil || completed.Item.Status != wishchain.StatusCompleted {
		t.Fatalf("完成应成功: %+v", completed)
	}

	deleted := callTool(t, ToolDelete, `{"id":"`+created.Item.ID+`"}`)
	if !deleted.OK || deleted.Item == nil || deleted.Item.ID != created.Item.ID {
		t.Fatalf("删除应成功: %+v", deleted)
	}
}

// TestWishManagerCapacityAsksForConfirmation 超过 16 条时必须返回容量确认。
func TestWishManagerCapacityAsksForConfirmation(t *testing.T) {
	testenv.Isolate(t)
	wishchain.Reset()
	for i := 0; i < wishchain.MaxPending; i++ {
		if _, err := wishchain.Add("愿望", "", wishchain.AutoPriority); err != nil {
			t.Fatalf("第 %d 条愿望新增失败: %v", i+1, err)
		}
	}
	result := callTool(t, ToolAdd, `{"content":"第 17 条愿望"}`)
	if result.OK || !result.NeedConfirm {
		t.Fatalf("容量已满时应要求确认: %+v", result)
	}
	if result.Detail["capacity"] != wishchain.MaxPending {
		t.Fatalf("确认结果应带容量上限: %+v", result.Detail)
	}
	if _, ok := result.Detail["wishes"]; !ok {
		t.Fatalf("确认结果应带当前愿望列表: %+v", result.Detail)
	}
}

// TestWishManagerFailures 非法参数 / 不存在的愿望必须返回失败。
func TestWishManagerFailures(t *testing.T) {
	testenv.Isolate(t)
	wishchain.Reset()
	if result := callTool(t, ToolAdd, `{"content":"x","due":"不是时间"}`); result.OK {
		t.Fatalf("非法时间应失败: %+v", result)
	}
	if result := callTool(t, ToolAdd, `{"content":"x","priority":99}`); result.OK {
		t.Fatalf("非法优先级应失败: %+v", result)
	}
	if result := callTool(t, ToolUpdate, `{"id":"missing","content":"x"}`); result.OK {
		t.Fatalf("不存在的愿望应失败: %+v", result)
	}
	if result := callTool(t, ToolDelete, `{"id":"missing"}`); result.OK {
		t.Fatalf("删除不存在的愿望应失败: %+v", result)
	}
	if result := callTool(t, ToolPriority, `{"id":"missing"}`); result.OK {
		t.Fatalf("缺失 priority 应失败: %+v", result)
	}
	if result := callTool(t, "unknown", `{}`); result.OK {
		t.Fatalf("未知工具应失败: %+v", result)
	}
	if result, err := Execute(context.Background(), ""); err != nil || result.OK {
		t.Fatalf("空指令应返回失败: %+v %v", result, err)
	}
}

// TestWishManagerCatalog 愿望管理智能体必须暴露六个愿望工具。
func TestWishManagerCatalog(t *testing.T) {
	catalog := ToolCatalog()
	if len(catalog) != 1 || len(catalog[0].Tools) != 6 {
		t.Fatalf("工具目录不正确: %+v", catalog)
	}
}
