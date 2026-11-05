package life

// memory_batch_test.go 整理事件的自动触发开关回归测试。
//
// 「按条数/时间挑选候选」的旧规则已随整理精简移除（整理只处理待合并组），
// 候选与批次规划的用例见 memory_pending_test.go / memory_consolidate_test.go。

import (
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
)

// TestMemoryAutoConsolidateGate 自动整理开关关闭时权重为 0（随机挑选跳过），
// 打开且存在待合并条目时恢复默认权重；手动触发在关闭状态下依然可用。
func TestMemoryAutoConsolidateGate(t *testing.T) {
	resetLifeForTest(t)
	oldSwitch := config.MemoryAutoConsolidate
	oldCount := pendingCountHook
	t.Cleanup(func() {
		config.MemoryAutoConsolidate = oldSwitch
		pendingCountHook = oldCount
	})

	// 即便有待合并条目，关闭开关后也不参与随机挑选
	pendingCountHook = func() int { return 1 }
	config.MemoryAutoConsolidate = false
	if got := registeredMemoryEvent(t).Weight(); got != 0 {
		t.Fatalf("开关关闭时权重应为 0，实得 %d", got)
	}

	mu.Lock()
	for _, event := range registry {
		if event.ID() != "memory" && event.ID() != "daze" {
			state.Used[event.ID()] = event.DailyQuota()
		}
	}
	picked := pickLocked()
	mu.Unlock()
	if picked == nil || picked.ID() != "daze" {
		t.Fatalf("关闭自动整理后随机挑选应跳过 memory，实际 %v", picked)
	}

	if !RequestMemoryConsolidation("") {
		t.Fatal("关闭自动整理后仍应支持手动触发")
	}
	env := Env{}
	mu.Lock()
	taken := takeForcedEventLocked(&env)
	mu.Unlock()
	if taken == nil || taken.ID() != "memory" {
		t.Fatalf("手动触发应取出 memory 事件，实际 %v", taken)
	}
	if env.Instruction != defaultMemoryInstruction {
		t.Fatalf("手动触发应带默认指令，实际 %q", env.Instruction)
	}

	config.MemoryAutoConsolidate = true
	if got := registeredMemoryEvent(t).Weight(); got != defaultEvents["memory"].Weight {
		t.Fatalf("开关打开后权重应恢复为 %d，实得 %d", defaultEvents["memory"].Weight, got)
	}

	// 没有待合并条目时整理无事可做，权重归零，避免空跑占额度
	pendingCountHook = func() int { return 0 }
	if got := registeredMemoryEvent(t).Weight(); got != 0 {
		t.Fatalf("没有待合并条目时权重应为 0，实得 %d", got)
	}
}

// registeredMemoryEvent 返回注册表中的 memory 事件。
func registeredMemoryEvent(t *testing.T) Event {
	t.Helper()
	mu.Lock()
	defer mu.Unlock()
	for _, event := range registry {
		if event.ID() == "memory" {
			return event
		}
	}
	t.Fatal("memory 事件未注册")
	return nil
}
