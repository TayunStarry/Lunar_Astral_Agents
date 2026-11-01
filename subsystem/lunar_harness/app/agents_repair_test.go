package app

// agents_repair_test.go 全量智能体的修复覆盖校验：
// 每个登记了数据导出的 Go 智能体都必须同时登记智能修复目标，
// 否则「故障能上报、却没有任何可修动作」——这正是本次补齐的约束。

import (
	"testing"

	_ "LunarSubsystem/LunarHarness/agents/Creator"
	_ "LunarSubsystem/LunarHarness/agents/Responder"

	"LunarSubsystem/LunarHarness/engine/agentdump"
	"LunarSubsystem/LunarHarness/engine/agentrepair"
)

func TestAllGoAgentsRegisteredForRepair(t *testing.T) {
	targets := map[string]bool{}
	for _, info := range agentrepair.List() {
		targets[info.ID] = true
	}

	ids := agentdump.IDs()
	if len(ids) == 0 {
		t.Fatal("没有任何智能体登记数据导出")
	}
	for _, id := range ids {
		if !targets[id] {
			t.Errorf("智能体 %s 未登记智能修复目标", id)
		}
	}
}
