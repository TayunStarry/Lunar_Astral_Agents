package WishManager

// dump_test.go 愿望管理的导出与诊断登记：Inspect 必须读自己的上下文。

import (
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/engine/wishchain"
	"LunarSubsystem/LunarHarness/internal/testenv"
	"LunarSubsystem/LunarHarness/protocol/agentrepair"
)

// TestRepairInspectUsesOwnContext 注册后 Inspect 返回愿望管理自己的上下文。
func TestRepairInspectUsesOwnContext(t *testing.T) {
	testenv.Isolate(t)
	wishchain.Reset()
	recordRun("把看电影的愿望提到最高优先级", Result{OK: true, Action: "priority", Summary: "已提到 P0"}, nil)

	out, err := agentrepair.Inspect(AgentID, 0)
	if err != nil {
		t.Fatalf("愿望管理应已登记修复目标: %v", err)
	}
	if !strings.Contains(out, `"kind": "wish-chain-context"`) {
		t.Fatalf("Inspect 未返回愿望链路上下文: %s", out)
	}
	if !strings.Contains(out, "把看电影的愿望提到最高优先级") {
		t.Fatalf("Inspect 应包含本轮指令: %s", out)
	}
	if strings.Contains(out, `"lunar"`) {
		t.Fatalf("Inspect 不应回落到对话者上下文: %s", out)
	}
}
