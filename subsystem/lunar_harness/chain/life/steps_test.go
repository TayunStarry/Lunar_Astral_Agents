package life

// steps_test.go 固定步骤编排回归测试：
// 顺序固定为「心情 → 态度 → 记忆待合并」，前一步失败不阻断后一步，
// 无待合并条目时不排队事件、不产生任何模型调用。

import (
	"context"
	"errors"
	"testing"
)

func withFixedStepHooks(t *testing.T) {
	t.Helper()
	oldMood, oldAffection, oldMemory := moodStepHook, affectionStepHook, memoryPendingStepHook
	t.Cleanup(func() {
		moodStepHook = oldMood
		affectionStepHook = oldAffection
		memoryPendingStepHook = oldMemory
	})
}

func TestFixedStepsRunInFixedOrder(t *testing.T) {
	resetLifeForTest(t)
	withFixedStepHooks(t)

	var order []string
	moodStepHook = func(context.Context) error {
		order = append(order, "mood")
		return nil
	}
	affectionStepHook = func(context.Context) error {
		order = append(order, "affection")
		return nil
	}
	memoryPendingStepHook = func(context.Context) error {
		order = append(order, "memory")
		return nil
	}

	if failures := runFixedSteps(context.Background()); len(failures) != 0 {
		t.Fatalf("无失败步骤时不应记录失败: %v", failures)
	}
	want := []string{"mood", "affection", "memory"}
	if len(order) != len(want) {
		t.Fatalf("固定步骤数量应为 %d，实际 %v", len(want), order)
	}
	for i, name := range want {
		if order[i] != name {
			t.Fatalf("固定步骤顺序错误：期望 %v，实际 %v", want, order)
		}
	}
}

func TestFixedStepFailureDoesNotBlockFollowingSteps(t *testing.T) {
	resetLifeForTest(t)
	withFixedStepHooks(t)

	var order []string
	moodStepHook = func(context.Context) error {
		order = append(order, "mood")
		return errors.New("嵌入服务不可用")
	}
	affectionStepHook = func(context.Context) error {
		order = append(order, "affection")
		return nil
	}
	memoryPendingStepHook = func(context.Context) error {
		order = append(order, "memory")
		return nil
	}

	failures := runFixedSteps(context.Background())
	if len(failures) != 1 {
		t.Fatalf("应记录 1 个失败步骤，实际 %v", failures)
	}
	if len(order) != 3 || order[0] != "mood" || order[1] != "affection" || order[2] != "memory" {
		t.Fatalf("前一步失败不应阻断后续步骤，实际顺序 %v", order)
	}
}

func TestPendingMergeNoPendingIsNoop(t *testing.T) {
	resetLifeForTest(t)
	oldCount, oldRun := pendingCountHook, memoryMergeRunHook
	t.Cleanup(func() {
		pendingCountHook = oldCount
		memoryMergeRunHook = oldRun
	})

	pendingCountHook = func() int { return 0 }
	runCalled := false
	memoryMergeRunHook = func(context.Context) error {
		runCalled = true
		return nil
	}

	if err := mergePendingMemories(context.Background()); err != nil {
		t.Fatal(err)
	}
	if HasPendingEvent() {
		t.Fatal("记忆整理不应再排队事件")
	}
	if runCalled {
		t.Fatal("无待合并条目时不应产生模型调用")
	}
}

// TestMemoryStepRunsOncePerRound 固定步骤三每回合只执行一次，
// 且事件挑选不会再次命中 memory。
func TestMemoryStepRunsOncePerRound(t *testing.T) {
	resetLifeForTest(t)
	withFixedStepHooks(t)
	oldCount, oldRun := pendingCountHook, memoryMergeRunHook
	t.Cleanup(func() {
		pendingCountHook = oldCount
		memoryMergeRunHook = oldRun
	})

	// 前序用例会把固定步骤替换成记录桩，这里显式指回真实实现再统计调用次数。
	memoryPendingStepHook = mergePendingMemories
	pendingCountHook = func() int { return 3 }
	runCount := 0
	memoryMergeRunHook = func(context.Context) error {
		runCount++
		return nil
	}

	runFixedSteps(context.Background())
	if runCount != 1 {
		t.Fatalf("每回合记忆整理应只执行一次，实际 %d 次", runCount)
	}

	oldRand := pickRandHook
	t.Cleanup(func() { pickRandHook = oldRand })
	for r := 0; r < 100; r++ {
		pickRandHook = func(int) int { return r }
		if picked := pickLocked(); picked != nil && picked.ID() == "memory" {
			t.Fatalf("事件挑选不应命中 memory（r=%d）", r)
		}
	}
}

func TestWatchBranchWeightIncluded(t *testing.T) {
	resetLifeForTest(t)
	if event := eventByID("watch"); event == nil {
		t.Fatal("watch 事件应作为正式分支注册")
	}
	want := map[string]int{"daze": 40, "watch": 26, "forum": 20, "creative": 14}
	if _, ok := defaultEvents["memory"]; ok {
		t.Fatal("记忆整理不应再出现在权重事件表中")
	}
	total := 0
	for id, weight := range want {
		if got := defaultEvents[id].Weight; got != weight {
			t.Fatalf("事件 %s 权重应为 %d，实际 %d", id, weight, got)
		}
		total += weight
	}
	if total != 100 {
		t.Fatalf("加入看视频后的权重总和应为 100，实际 %d", total)
	}
	if len(defaultEvents) != len(want) {
		t.Fatalf("不应残留额外事件，实际 %v", defaultEvents)
	}
}
