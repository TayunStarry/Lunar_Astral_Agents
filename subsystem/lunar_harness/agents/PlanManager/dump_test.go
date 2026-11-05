package PlanManager

// dump_test.go 计划管理的导出与修复登记：Inspect 必须读自己的上下文。

import (
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/internal/testenv"
	"LunarSubsystem/LunarHarness/protocol/agentrepair"
)

// TestRepairInspectUsesOwnContext 注册后 Inspect 返回计划管理自己的上下文。
func TestRepairInspectUsesOwnContext(t *testing.T) {
	testenv.Isolate(t)
	recordRun("把明天的会议改到四点", Result{OK: true, Action: "update", Summary: "已改到四点"}, nil)

	out, err := agentrepair.Inspect(agentID, 0)
	if err != nil {
		t.Fatalf("计划管理应已登记修复目标: %v", err)
	}
	if !strings.Contains(out, `"kind": "plan-manager-context"`) {
		t.Fatalf("Inspect 未返回计划管理自己的上下文: %s", out)
	}
	if !strings.Contains(out, "把明天的会议改到四点") {
		t.Fatalf("Inspect 应包含本轮指令: %s", out)
	}
	if strings.Contains(out, `"lunar"`) {
		t.Fatalf("Inspect 不应回落到对话者上下文: %s", out)
	}

	if msg, rerr := agentrepair.Repair(agentID, "reset", nil); rerr != nil || !strings.Contains(msg, "已清空") {
		t.Fatalf("reset 失败: msg=%q err=%v", msg, rerr)
	}
	after, err := agentrepair.Inspect(agentID, 0)
	if err != nil {
		t.Fatalf("reset 后 Inspect 仍应可用: %v", err)
	}
	if strings.Contains(after, "last_run") {
		t.Fatalf("reset 后不应再带执行记录: %s", after)
	}
}
