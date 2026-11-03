package PlanManager

// agent_test.go 计划管理智能体的工具成功 / 失败返回验收（不依赖模型）。

import (
	"context"
	"strings"
	"testing"

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

// TestPlanManagerToolLifecycle 新增 / 查询 / 改写 / 删除的成功返回。
func TestPlanManagerToolLifecycle(t *testing.T) {
	testenv.Isolate(t)

	created := callTool(t, ToolCreate, `{"time":"2026-06-14 15:30","content":"整理文档"}`)
	if !created.OK || created.Item == nil || created.Item.ID == "" {
		t.Fatalf("新增应成功: %+v", created)
	}
	if !strings.Contains(created.Summary, "整理文档") {
		t.Fatalf("新增简述应说明内容: %q", created.Summary)
	}

	queried := callTool(t, ToolQuery, `{"keywords":["文档"],"date":"2026-06-14"}`)
	if !queried.OK || !strings.Contains(queried.Detail["result"].(string), "整理文档") {
		t.Fatalf("查询应成功: %+v", queried)
	}

	updated := callTool(t, ToolUpdate, `{"id":"`+created.Item.ID+`","content":"整理并归档"}`)
	if !updated.OK || updated.Item == nil || updated.Item.Content != "整理并归档" {
		t.Fatalf("改写应成功: %+v", updated)
	}

	deleted := callTool(t, ToolDelete, `{"id":"`+created.Item.ID+`"}`)
	if !deleted.OK || deleted.Item == nil || deleted.Item.ID != created.Item.ID {
		t.Fatalf("删除应成功: %+v", deleted)
	}
}

// TestPlanManagerFailures 非法参数 / 不存在的计划项必须返回失败。
func TestPlanManagerFailures(t *testing.T) {
	testenv.Isolate(t)
	if result := callTool(t, ToolCreate, `{"time":"不是时间","content":"x"}`); result.OK {
		t.Fatalf("非法时间应失败: %+v", result)
	}
	if result := callTool(t, ToolUpdate, `{"id":"missing","content":"x"}`); result.OK {
		t.Fatalf("不存在的计划项应失败: %+v", result)
	}
	if result := callTool(t, ToolDelete, `{"id":"missing"}`); result.OK {
		t.Fatalf("删除不存在的计划项应失败: %+v", result)
	}
	if result := callTool(t, "unknown", `{}`); result.OK {
		t.Fatalf("未知工具应失败: %+v", result)
	}
	if result, err := Execute(context.Background(), ""); err != nil || result.OK {
		t.Fatalf("空指令应返回失败: %+v %v", result, err)
	}
}

// TestPlanManagerCatalog 计划管理智能体必须暴露四个计划表工具。
func TestPlanManagerCatalog(t *testing.T) {
	catalog := ToolCatalog()
	if len(catalog) != 1 || len(catalog[0].Tools) != 4 {
		t.Fatalf("工具目录不正确: %+v", catalog)
	}
}
