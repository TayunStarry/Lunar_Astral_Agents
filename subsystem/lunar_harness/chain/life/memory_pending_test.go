package life

// memory_pending_test.go 待合并（待合并组）队列的链侧回归测试：
// 组识别、孤儿回收、批次规划顺序、无组时不动作。
//
// 整理不再有基于条数/时间的强制兜底，也不再是权重事件，因此本文件不再覆盖
// 「超限排空」「按权重选中」相关行为。

import (
	"context"
	"fmt"
	"testing"

	"LunarSubsystem/LunarHarness/engine/memory"
)

func TestPendingCandidateSetCollectsPartners(t *testing.T) {
	items := []memoryItem{
		{ID: "old", Content: "天气不错", Timestamp: 100},
		{ID: "new", Content: "天气很好", Timestamp: 200, Pending: true, PendingPartner: "old"},
	}
	pending, partners := pendingCandidateSet(items)
	if len(pending) != 1 || pending[0].ID != "new" {
		t.Fatalf("待处理条目识别异常: %+v", pending)
	}
	if len(partners) != 1 || partners[0].ID != "old" {
		t.Fatalf("伙伴条目应一并进入整理: %+v", partners)
	}

	// 没有伙伴标记的待处理条目只进 pending，不产生伙伴
	orphan := []memoryItem{{ID: "solo", Content: "独条", Timestamp: 100, Pending: true}}
	pending, partners = pendingCandidateSet(orphan)
	if len(pending) != 1 || len(partners) != 0 {
		t.Fatalf("无伙伴的待处理条目不应产生伙伴: %+v / %+v", pending, partners)
	}
}

func TestPendingGroupItemsDedupes(t *testing.T) {
	pending := []memoryItem{
		{ID: "p1", Content: "天气很好", Pending: true, PendingPartner: "p2"},
		{ID: "p2", Content: "天气真好", Pending: true, PendingPartner: "p1"},
	}
	partners := []memoryItem{{ID: "p1", Content: "天气很好", Pending: true}}
	items := pendingGroupItems(pending, partners)
	if len(items) != 2 {
		t.Fatalf("同一 ID 只能出现一次，实得 %d 条: %+v", len(items), items)
	}
}

func TestBuildPendingBatchesGroupsAndSplits(t *testing.T) {
	items := []memoryItem{
		{ID: "old", Content: "天气不错", Timestamp: 100},
		{ID: "new", Content: "天气很好", Timestamp: 200, Pending: true, PendingPartner: "old"},
	}
	batches := buildPendingBatches(items, []memory.TextCluster{{IDs: []string{"old", "new"}}})
	if len(batches) != 1 {
		t.Fatalf("应生成 1 个待合并批次，实得 %d", len(batches))
	}
	if len(batches[0].Items) != 2 || batches[0].Items[0].ID != "old" || batches[0].Items[1].ID != "new" {
		t.Fatalf("批次内应按入库时间升序: %+v", batches[0].Items)
	}

	// 多个 pending 指向同一伙伴：即使没有聚类结果也要合并成一组
	multi := []memoryItem{
		{ID: "old", Content: "天气不错", Timestamp: 100},
		{ID: "p1", Content: "天气很好", Timestamp: 200, Pending: true, PendingPartner: "old"},
		{ID: "p2", Content: "天气真好", Timestamp: 300, Pending: true, PendingPartner: "old"},
	}
	batches = buildPendingBatches(multi, nil)
	if len(batches) != 1 || len(batches[0].Items) != 3 {
		t.Fatalf("同一伙伴的待处理条目应并成一批: %+v", batches)
	}

	// 组过大时按 pendingGroupMaxItems 切块，避免单批模型输入过大
	many := []memoryItem{{ID: "old", Content: "天气不错", Timestamp: 100}}
	for i := 0; i < 20; i++ {
		many = append(many, memoryItem{
			ID:             fmt.Sprintf("p-%02d", i),
			Content:        "天气很好",
			Timestamp:      int64(200 + i),
			Pending:        true,
			PendingPartner: "old",
		})
	}
	batches = buildPendingBatches(many, nil)
	if len(batches) != 3 {
		t.Fatalf("21 条应切成 3 批，实得 %d", len(batches))
	}
	seen := 0
	for _, batch := range batches {
		if len(batch.Items) > pendingGroupMaxItems {
			t.Fatalf("单批不应超过 %d 条，实得 %d", pendingGroupMaxItems, len(batch.Items))
		}
		seen += len(batch.Items)
	}
	if seen != len(many) {
		t.Fatalf("切块不应丢条目，期望 %d 条实得 %d", len(many), seen)
	}
}

// TestChunkPendingGroupKeepsEveryPendingMergeable 切块不能把待处理条目与伙伴切散后
// 产生「纯伙伴块」并被跳过：每个批次都必须含 Pending 条目，且条目总数不丢。
func TestChunkPendingGroupKeepsEveryPendingMergeable(t *testing.T) {
	items := make([]memoryItem, 0, 18)
	for i := 0; i < 9; i++ {
		items = append(items, memoryItem{
			ID:        fmt.Sprintf("old-%d", i),
			Content:   "旧记忆",
			Timestamp: int64(i + 1),
		})
	}
	for i := 0; i < 9; i++ {
		items = append(items, memoryItem{
			ID:             fmt.Sprintf("new-%d", i),
			Content:        "新记忆",
			Timestamp:      int64(100 + i),
			Pending:        true,
			PendingPartner: fmt.Sprintf("old-%d", i),
		})
	}
	// 用聚类把 9 对连成一个待合并组，强制触发切块路径
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	batches := buildPendingBatches(items, []memory.TextCluster{{IDs: ids}})
	if len(batches) != 2 {
		t.Fatalf("18 条应切成 2 批，实得 %d", len(batches))
	}
	total := 0
	for _, batch := range batches {
		if !groupHasPending(batch.Items) {
			t.Fatalf("切块后每批都必须含待处理条目: %+v", batch.Items)
		}
		total += len(batch.Items)
	}
	if total != len(items) {
		t.Fatalf("切块不应丢条目，期望 %d 条实得 %d", len(items), total)
	}
}

// TestBuildPendingBatchesProcessingOrder 多个待合并组按「组内最旧条目」升序进入计划，
// 保证逐组处理时旧记忆优先。
func TestBuildPendingBatchesProcessingOrder(t *testing.T) {
	items := []memoryItem{
		// 组 A：最旧 500
		{ID: "a-old", Content: "A 旧", Timestamp: 500},
		{ID: "a-new", Content: "A 新", Timestamp: 900, Pending: true, PendingPartner: "a-old"},
		// 组 B：最旧 100
		{ID: "b-old", Content: "B 旧", Timestamp: 100},
		{ID: "b-new", Content: "B 新", Timestamp: 700, Pending: true, PendingPartner: "b-old"},
		// 组 C：最旧 300
		{ID: "c-old", Content: "C 旧", Timestamp: 300},
		{ID: "c-new", Content: "C 新", Timestamp: 800, Pending: true, PendingPartner: "c-old"},
	}
	batches := buildPendingBatches(items, nil)
	if len(batches) != 3 {
		t.Fatalf("应得到 3 个待合并组，实得 %d", len(batches))
	}
	order := []string{batches[0].Items[0].ID, batches[1].Items[0].ID, batches[2].Items[0].ID}
	if order[0] != "b-old" || order[1] != "c-old" || order[2] != "a-old" {
		t.Fatalf("多组应按最旧条目升序处理，实得 %v", order)
	}
}

// TestPlanPendingBatchesNoWork 没有待合并组时不产生任何批次与原因。
func TestPlanPendingBatchesNoWork(t *testing.T) {
	batches, reason := planPendingBatches(nil, nil)
	if len(batches) != 0 || reason == "" {
		t.Fatalf("无待合并组时不应产生批次，实得 %d 批、原因 %q", len(batches), reason)
	}
	// 只有单条（不足以构成组）时同样不动作
	single := []memoryItem{{ID: "solo", Content: "独条", Pending: true}}
	batches, reason = planPendingBatches(single, nil)
	if len(batches) != 0 || reason == "" {
		t.Fatalf("单条待处理条目不应产生批次，实得 %d 批、原因 %q", len(batches), reason)
	}
}

func TestClearOrphanPendingReturnsToVisible(t *testing.T) {
	var cleared []string
	oldClear := pendingClearHook
	pendingClearHook = func(_ context.Context, id string) error {
		cleared = append(cleared, id)
		return nil
	}
	t.Cleanup(func() { pendingClearHook = oldClear })

	pending := []memoryItem{{ID: "p1", Content: "天气很好", Timestamp: 200, Pending: true, PendingPartner: "missing"}}
	kept := clearOrphanPending(context.Background(), pending, nil)
	if len(kept) != 0 {
		t.Fatalf("伙伴已不存在的待处理条目应解除待处理状态，实得 %+v", kept)
	}
	if len(cleared) != 1 || cleared[0] != "p1" {
		t.Fatalf("应清除孤儿条目的待处理标记，实得 %+v", cleared)
	}

	// 清除失败时保留原条目等待下轮重试
	pendingClearHook = func(_ context.Context, _ string) error { return context.Canceled }
	kept = clearOrphanPending(context.Background(), pending, nil)
	if len(kept) != 1 || kept[0].ID != "p1" {
		t.Fatalf("清除失败应保留待处理条目，实得 %+v", kept)
	}
}
