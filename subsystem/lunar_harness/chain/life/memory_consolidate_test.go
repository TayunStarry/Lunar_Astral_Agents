package life

// memory_consolidate_test.go 整理精简后的回归测试：
// 只处理待合并组、单组可输出多条、先写后删（写入失败旧条目全在）、
// 空输出/无合并保持原状、多组顺序处理、权重总和 100 与按权重挑选。
//
// 所有外部依赖（模型合并、向量校验、事务替换、待处理标记清除）均通过注入点替换，
// 测试不依赖真实记忆库实例、嵌入服务或模型服务。

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/memory"
)

// setupConsolidateTest 准备隔离的归档/日志目录，并在用例结束后还原全部注入点。
func setupConsolidateTest(t *testing.T) {
	t.Helper()
	resetLifeForTest(t)
	oldMerge, oldVerify, oldReplace := mergeHook, verifyHook, replaceHook
	oldClear, oldFailure := pendingClearHook, pendingFailureHook
	pendingFailureHook = func(context.Context, []string, int, time.Duration) (memory.PendingFailureResult, error) {
		return memory.PendingFailureResult{}, nil
	}
	t.Cleanup(func() {
		mergeHook = oldMerge
		verifyHook = oldVerify
		replaceHook = oldReplace
		pendingClearHook = oldClear
		pendingFailureHook = oldFailure
	})
}

func testBatch(ids ...string) memoryBatch {
	items := make([]memoryItem, 0, len(ids))
	for i, id := range ids {
		items = append(items, memoryItem{
			ID:        id,
			Content:   "旧记忆 " + id,
			Timestamp: int64(100 + i),
		})
	}
	return memoryBatch{Items: items}
}

// pendingTestBatch 构造带待处理标记的批次，用于验证「原样返回」终止态与失败兜底。
func pendingTestBatch(ids ...string) memoryBatch {
	batch := testBatch(ids...)
	for i := range batch.Items {
		batch.Items[i].Pending = true
	}
	return batch
}

// TestProcessMemoryBatchAcceptsMultipleNewEntries 一组旧条目可以输出多条整合结果，
// 不是固定压缩成一条；替换请求必须带上全部旧条目 ID。
func TestProcessMemoryBatchAcceptsMultipleNewEntries(t *testing.T) {
	setupConsolidateTest(t)
	batch := testBatch("a", "b", "c")

	var replacedOld []string
	var replacedNew []string
	mergeHook = func(_ context.Context, _ []string) ([]string, error) {
		return []string{"整合结果一：天气不错", "整合结果二：项目上线"}, nil
	}
	verifyHook = func(_ context.Context, oldIDs, newTexts []string, _ float32) error {
		if len(oldIDs) != 3 || len(newTexts) != 2 {
			t.Fatalf("校验入参异常: old=%v new=%v", oldIDs, newTexts)
		}
		return nil
	}
	replaceHook = func(_ context.Context, oldIDs, newTexts []string, _ memory.ReplaceOptions) (memory.ReplaceResult, error) {
		replacedOld = append([]string(nil), oldIDs...)
		replacedNew = append([]string(nil), newTexts...)
		return memory.ReplaceResult{CreatedIDs: []string{"new-1", "new-2"}}, nil
	}

	if err := processMemoryBatch(context.Background(), "mem-multi", 0, batch); err != nil {
		t.Fatalf("单组输出多条不应失败: %v", err)
	}
	if len(replacedOld) != 3 {
		t.Fatalf("替换应覆盖组内全部旧条目，实得 %v", replacedOld)
	}
	if len(replacedNew) != 2 {
		t.Fatalf("整理结果应保留多条事实条目，实得 %v", replacedNew)
	}
}

// TestProcessMemoryBatchKeepsOriginalWhenModelReturnsNothing 模型报错/空输出保持待处理，
// 「没有实际合并」（模型原样返回）则是合法终止态：解除待处理标记恢复检索可见。
func TestProcessMemoryBatchKeepsOriginalWhenModelReturnsNothing(t *testing.T) {
	cases := []struct {
		name      string
		merge     func(context.Context, []string) ([]string, error)
		wantClear bool
	}{
		{
			name: "模型调用失败",
			merge: func(context.Context, []string) ([]string, error) {
				return nil, errors.New("模型不可用")
			},
		},
		{
			name: "输出为空",
			merge: func(context.Context, []string) ([]string, error) {
				return nil, nil
			},
		},
		{
			name: "没有实际合并",
			merge: func(_ context.Context, contents []string) ([]string, error) {
				return append([]string(nil), contents...), nil
			},
			wantClear: true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			setupConsolidateTest(t)
			batch := pendingTestBatch("a", "b")
			mergeHook = c.merge

			verifyCalled := false
			replaceCalled := false
			clearCount := 0
			failureCount := 0
			verifyHook = func(context.Context, []string, []string, float32) error {
				verifyCalled = true
				return nil
			}
			replaceHook = func(context.Context, []string, []string, memory.ReplaceOptions) (memory.ReplaceResult, error) {
				replaceCalled = true
				return memory.ReplaceResult{}, nil
			}
			pendingClearHook = func(context.Context, string) error {
				clearCount++
				return nil
			}
			pendingFailureHook = func(context.Context, []string, int, time.Duration) (memory.PendingFailureResult, error) {
				failureCount++
				return memory.PendingFailureResult{}, nil
			}

			if err := processMemoryBatch(context.Background(), "mem-keep", 0, batch); err != nil {
				t.Fatalf("保持原状不应返回错误: %v", err)
			}
			if verifyCalled || replaceCalled {
				t.Fatalf("没有可合并内容时不应触发校验/替换: verify=%v replace=%v",
					verifyCalled, replaceCalled)
			}
			if c.wantClear {
				if clearCount != len(batch.Items) {
					t.Fatalf("原样返回应解除全部待处理标记，期望 %d 次，实得 %d", len(batch.Items), clearCount)
				}
				if failureCount != 0 {
					t.Fatalf("原样返回是合法终止态，不应记为失败，实得 %d 次", failureCount)
				}
			} else {
				if clearCount != 0 {
					t.Fatalf("失败时应保留待处理标记等待兜底，实得清除 %d 次", clearCount)
				}
				if failureCount != 1 {
					t.Fatalf("失败应记账一次用于重试/超时兜底，实得 %d 次", failureCount)
				}
			}
			// 旧条目仍在归档之外的原位，且待处理组完整保留
			if len(batch.Items) != 2 {
				t.Fatalf("原批次条目不应被改动，实得 %d", len(batch.Items))
			}
		})
	}
}

// TestProcessMemoryBatchKeepsOriginalWhenReplaceFails 写入失败时旧条目全部保留：
// 事务由引擎 ReplaceTexts 负责回滚，链侧不得立即清除待处理标记或中断其它组，
// 但要记账一次失败供重试上限/超时兜底；同时留下 failed 意图日志供下一轮重放重试。
func TestProcessMemoryBatchKeepsOriginalWhenReplaceFails(t *testing.T) {
	setupConsolidateTest(t)
	batch := pendingTestBatch("a", "b")

	mergeHook = func(_ context.Context, _ []string) ([]string, error) {
		return []string{"整合后的记忆"}, nil
	}
	verifyHook = func(context.Context, []string, []string, float32) error { return nil }

	var oldIDsSeen []string
	replaceHook = func(_ context.Context, oldIDs, _ []string, _ memory.ReplaceOptions) (memory.ReplaceResult, error) {
		oldIDsSeen = append([]string(nil), oldIDs...)
		// 模拟引擎「先写新、写入中途失败」：已创建的部分由引擎回滚，旧条目全部保留。
		return memory.ReplaceResult{CreatedIDs: []string{"partial"}}, errors.New("写入失败")
	}
	clearCalled := false
	failureCount := 0
	pendingClearHook = func(context.Context, string) error {
		clearCalled = true
		return nil
	}
	pendingFailureHook = func(context.Context, []string, int, time.Duration) (memory.PendingFailureResult, error) {
		failureCount++
		return memory.PendingFailureResult{}, nil
	}

	if err := processMemoryBatch(context.Background(), "mem-fail", 0, batch); err != nil {
		t.Fatalf("替换失败应保持原组并继续其它组，不应返回错误: %v", err)
	}
	if len(oldIDsSeen) != len(batch.Items) {
		t.Fatalf("替换请求应包含组内全部旧条目，实得 %v", oldIDsSeen)
	}
	if clearCalled {
		t.Fatal("替换失败时不得立即清除待处理标记")
	}
	if failureCount != 1 {
		t.Fatalf("替换失败应记账一次用于重试/超时兜底，实得 %d 次", failureCount)
	}

	// 失败日志必须保留，下一轮 recoverJournals 会重放（幂等：同文复用 + 缺失按已删）
	raw, err := os.ReadFile(filepath.Join(config.LocalDir, filepath.FromSlash(journalPath("mem-fail-000"))))
	if err != nil {
		t.Fatalf("替换失败应保留意图日志: %v", err)
	}
	if !strings.Contains(string(raw), `"failed"`) {
		t.Fatalf("意图日志应记录失败状态: %s", raw)
	}
}

// TestMemoryRunNoPendingDoesNothing 没有待合并组时本轮直接结束：
// 不调用模型/校验/替换，且 NoWork() 为真（调度层据此不消耗额度）。
func TestMemoryRunNoPendingDoesNothing(t *testing.T) {
	setupConsolidateTest(t)
	mergeCalled := false
	replaceCalled := false
	mergeHook = func(context.Context, []string) ([]string, error) {
		mergeCalled = true
		return nil, nil
	}
	replaceHook = func(context.Context, []string, []string, memory.ReplaceOptions) (memory.ReplaceResult, error) {
		replaceCalled = true
		return memory.ReplaceResult{}, nil
	}

	r := &memoryRun{data: memoryRunData{PlanFile: memoryPlanFile, Reason: "没有待合并的记忆"}}
	if !r.NoWork() {
		t.Fatal("没有批次时应判定为无实际动作")
	}
	step, err := r.Next(context.Background(), Env{})
	if err != nil {
		t.Fatalf("无待合并组不应失败: %v", err)
	}
	if !step.Done {
		t.Fatal("无待合并组时应直接完成")
	}
	if mergeCalled || replaceCalled {
		t.Fatalf("无待合并组时不应调用模型或替换: merge=%v replace=%v", mergeCalled, replaceCalled)
	}
}

// TestMemoryRunProcessesBatchesInOrder 多个待合并组逐组处理，顺序与计划一致。
func TestMemoryRunProcessesBatchesInOrder(t *testing.T) {
	setupConsolidateTest(t)
	batches := []memoryBatch{
		testBatch("a1", "a2"),
		testBatch("b1", "b2"),
	}
	var order []string
	mergeHook = func(_ context.Context, contents []string) ([]string, error) {
		return []string{"整合 " + contents[0]}, nil
	}
	verifyHook = func(context.Context, []string, []string, float32) error { return nil }
	replaceHook = func(_ context.Context, _, newTexts []string, _ memory.ReplaceOptions) (memory.ReplaceResult, error) {
		order = append(order, newTexts[0])
		return memory.ReplaceResult{CreatedIDs: []string{"new"}}, nil
	}

	r := &memoryRun{
		data: memoryRunData{Total: len(batches)},
		plan: &memoryPlan{Version: memoryPlanVersion, ID: "mem-seq", Batches: batches},
	}
	for i := 0; i < len(batches); i++ {
		step, err := r.Next(context.Background(), Env{})
		if err != nil {
			t.Fatalf("第 %d 组处理失败: %v", i, err)
		}
		if i < len(batches)-1 && step.Done {
			t.Fatalf("第 %d 组后不应整体完成", i)
		}
		if i == len(batches)-1 && !step.Done {
			t.Fatal("全部组处理完成后应结束")
		}
	}
	want := []string{"整合 旧记忆 a1", "整合 旧记忆 b1"}
	if len(order) != len(want) || order[0] != want[0] || order[1] != want[1] {
		t.Fatalf("多组应按计划顺序处理，实得 %v", order)
	}
}

// TestDefaultEventWeightsSumTo100 生活事件权重总和固定为 100，
// 且记忆整理不再是权重事件。
func TestDefaultEventWeightsSumTo100(t *testing.T) {
	total := 0
	maxID := ""
	maxWeight := 0
	for id, params := range defaultEvents {
		// 权重为 0 的事件只接受强制触发（如同步演绎 / 文件操作），不参与抽签。
		if params.Weight == 0 && params.Quota == 0 {
			continue
		}
		if params.Weight <= 0 {
			t.Fatalf("事件 %s 权重必须为正: %d", id, params.Weight)
		}
		if params.Quota <= 0 {
			t.Fatalf("事件 %s 每日额度必须为正: %d", id, params.Quota)
		}
		total += params.Weight
		if params.Weight > maxWeight {
			maxWeight = params.Weight
			maxID = id
		}
	}
	if total != 100 {
		t.Fatalf("权重总和应为 100，实得 %d", total)
	}
	if maxID != "daze" {
		t.Fatalf("兜底事件 daze 应保持最高权重，实际最高为 %s=%d", maxID, maxWeight)
	}
	if _, ok := defaultEvents["memory"]; ok {
		t.Fatal("记忆整理已收敛为固定步骤，不应再占事件权重")
	}
}

// TestPickLockedUsesWeightProportion 调度器按权重比例挑选，不假定总和的具体值。
func TestPickLockedUsesWeightProportion(t *testing.T) {
	resetLifeForTest(t)
	oldRand := pickRandHook
	t.Cleanup(func() { pickRandHook = oldRand })

	mu.Lock()
	registry = nil
	state.Used = map[string]int{}
	mu.Unlock()
	RegisterEvent(weightTestEvent{id: "a", weight: 30, quota: 1000})
	RegisterEvent(weightTestEvent{id: "b", weight: 20, quota: 1000})
	RegisterEvent(weightTestEvent{id: "c", weight: 50, quota: 1000})

	cases := []struct {
		r    int
		want string
	}{
		{0, "a"}, {29, "a"},
		{30, "b"}, {49, "b"},
		{50, "c"}, {99, "c"},
	}
	for _, c := range cases {
		r := c.r
		pickRandHook = func(int) int { return r }
		mu.Lock()
		picked := pickLocked()
		mu.Unlock()
		if picked == nil || picked.ID() != c.want {
			t.Fatalf("r=%d 应选中 %s，实际 %v", r, c.want, picked)
		}
	}
}

// ==== 测试用事件 ====

type weightTestEvent struct {
	id     string
	weight int
	quota  int
}

func (e weightTestEvent) ID() string       { return e.id }
func (e weightTestEvent) Weight() int      { return e.weight }
func (e weightTestEvent) DailyQuota() int  { return e.quota }
func (e weightTestEvent) NewRun() EventRun { return weightTestRun{} }

type weightTestRun struct{}

func (weightTestRun) Start(context.Context, Env) error             { return nil }
func (weightTestRun) Next(context.Context, Env) (EventStep, error) { return EventStep{Done: true}, nil }
func (weightTestRun) Snapshot() json.RawMessage                    { return nil }
func (weightTestRun) Restore(json.RawMessage) error                { return nil }
