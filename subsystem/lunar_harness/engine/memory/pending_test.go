package memory

// pending_test.go 待处理（待合并）队列的回归测试：
// 相似写入不再丢弃、检索隐藏、整理成功清标记、写入上限不丢数据。

import (
	"context"
	"testing"
)

func TestSimilarTextWritesPendingInsteadOfDropping(t *testing.T) {
	d, _ := setupReplaceStore(t)
	ctx := context.Background()

	first, err := d.AddText(ctx, "user", "今天天气不错")
	if err != nil {
		t.Fatalf("写入第一条失败: %v", err)
	}
	second, err := d.AddText(ctx, "user", "今天天气很好")
	if err != nil {
		t.Fatalf("写入相似条目失败: %v", err)
	}
	if second == first {
		t.Fatalf("相似写入不应复用旧 ID，否则新措辞会丢失")
	}

	firstEntry, err := d.Get(first)
	if err != nil {
		t.Fatalf("读取旧条目失败: %v", err)
	}
	if firstEntry.Pending {
		t.Fatalf("旧条目不应被标记待处理: %+v", firstEntry)
	}
	secondEntry, err := d.Get(second)
	if err != nil {
		t.Fatalf("读取新条目失败: %v", err)
	}
	if !secondEntry.Pending || secondEntry.PendingPartner != first {
		t.Fatalf("新条目应写入并标记待处理、伙伴为 %s，实得 %+v", first, secondEntry)
	}
	if got := d.Count(TypeText); got != 2 {
		t.Fatalf("两条文本都应保留，实得 %d", got)
	}
	if got := d.PendingCount(); got != 1 {
		t.Fatalf("待处理队列应为 1 条，实得 %d", got)
	}
}

func TestPendingHiddenFromRetrieval(t *testing.T) {
	d, _ := setupReplaceStore(t)
	ctx := context.Background()

	first, err := d.AddText(ctx, "user", "今天天气不错")
	if err != nil {
		t.Fatalf("写入第一条失败: %v", err)
	}
	second, err := d.AddText(ctx, "user", "今天天气很好")
	if err != nil {
		t.Fatalf("写入相似条目失败: %v", err)
	}

	hits, err := d.Query(ctx, "天气", TypeText, 5, QueryOptions{})
	if err != nil {
		t.Fatalf("向量检索失败: %v", err)
	}
	if len(hits) != 1 || hits[0].ID != first {
		t.Fatalf("向量检索只应返回非待处理条目 %s，实得 %+v", first, hits)
	}

	keywordHits, err := d.KeywordSearch(TypeText, []string{"天气"}, false, 10)
	if err != nil {
		t.Fatalf("关键词检索失败: %v", err)
	}
	if len(keywordHits) != 1 || keywordHits[0].ID != first {
		t.Fatalf("关键词检索只应返回非待处理条目 %s，实得 %+v", first, keywordHits)
	}

	hybridHits, err := d.HybridQuery(ctx, []string{"天气"}, TypeText, 5, HybridQueryOptions{})
	if err != nil {
		t.Fatalf("混合检索失败: %v", err)
	}
	for _, entry := range hybridHits {
		if entry.ID == second {
			t.Fatalf("混合检索不应返回待处理条目 %s: %+v", second, hybridHits)
		}
	}
	if len(hybridHits) == 0 || hybridHits[0].ID != first {
		t.Fatalf("混合检索应保留非待处理条目 %s，实得 %+v", first, hybridHits)
	}

	// 整理侧可见：List 仍返回待处理条目
	entries, total := d.List(TypeText, 0, 0)
	if total != 2 || len(entries) != 2 {
		t.Fatalf("整理侧列举应看到两条（含待处理），实得 total=%d len=%d", total, len(entries))
	}
}

func TestReplaceTextsClearsPendingAfterMerge(t *testing.T) {
	d, _ := setupReplaceStore(t)
	ctx := context.Background()

	first, err := d.AddText(ctx, "user", "今天天气不错")
	if err != nil {
		t.Fatalf("写入第一条失败: %v", err)
	}
	second, err := d.AddText(ctx, "user", "今天天气很好")
	if err != nil {
		t.Fatalf("写入相似条目失败: %v", err)
	}

	result, err := d.ReplaceTexts(ctx, []string{first, second}, []string{"今天天气不错，心情很好"}, ReplaceOptions{})
	if err != nil {
		t.Fatalf("整理替换失败: %v", err)
	}
	if got := d.PendingCount(); got != 0 {
		t.Fatalf("合并成功后不应残留待处理条目，实得 %d", got)
	}
	for _, id := range result.CreatedIDs {
		entry, err := d.Get(id)
		if err != nil {
			t.Fatalf("读取整理结果失败: %v", err)
		}
		if entry.Pending || entry.PendingPartner != "" {
			t.Fatalf("整理结果不应带待处理标记: %+v", entry)
		}
	}
	if got := d.Count(TypeText); got != 1 {
		t.Fatalf("两条合并后应剩 1 条，实得 %d", got)
	}
}

func TestPendingCapWritesWithoutQueue(t *testing.T) {
	d, _ := setupReplaceStore(t)
	ctx := context.Background()

	original := PendingMaxEntries
	PendingMaxEntries = 3
	t.Cleanup(func() { PendingMaxEntries = original })

	first, err := d.AddText(ctx, "user", "今天天气不错")
	if err != nil {
		t.Fatalf("写入第一条失败: %v", err)
	}
	ids := []string{first}
	for i := 0; i < 4; i++ {
		id, err := d.AddText(ctx, "user", "今天天气很好")
		if err != nil {
			t.Fatalf("第 %d 次相似写入失败: %v", i+1, err)
		}
		ids = append(ids, id)
	}
	if got := d.Count(TypeText); got != len(ids) {
		t.Fatalf("超限也必须全部写入，期望 %d 条，实得 %d", len(ids), got)
	}
	if got := d.PendingCount(); got != PendingMaxEntries {
		t.Fatalf("待处理队列应停在上限 %d 条，实得 %d", PendingMaxEntries, got)
	}

	// 第 4 次相似写入已超上限：仍写入但不入队，检索可见
	lastEntry, err := d.Get(ids[len(ids)-1])
	if err != nil {
		t.Fatalf("读取超限条目失败: %v", err)
	}
	if lastEntry.Pending {
		t.Fatalf("超过上限的条目不应当入队: %+v", lastEntry)
	}
	hits, err := d.Query(ctx, "天气", TypeText, 10, QueryOptions{})
	if err != nil {
		t.Fatalf("向量检索失败: %v", err)
	}
	foundLast := false
	for _, entry := range hits {
		if entry.ID == lastEntry.ID {
			foundLast = true
		}
	}
	if !foundLast {
		t.Fatalf("未入队条目应对检索可见: %+v", hits)
	}
}

func TestClearPendingRestoresRetrievalVisibility(t *testing.T) {
	d, _ := setupReplaceStore(t)
	ctx := context.Background()

	if _, err := d.AddText(ctx, "user", "今天天气不错"); err != nil {
		t.Fatalf("写入第一条失败: %v", err)
	}
	second, err := d.AddText(ctx, "user", "今天天气很好")
	if err != nil {
		t.Fatalf("写入相似条目失败: %v", err)
	}
	if err := d.ClearPending(ctx, second); err != nil {
		t.Fatalf("清除待处理标记失败: %v", err)
	}
	entry, err := d.Get(second)
	if err != nil {
		t.Fatalf("读取条目失败: %v", err)
	}
	if entry.Pending || entry.PendingPartner != "" {
		t.Fatalf("清除后不应残留标记: %+v", entry)
	}
	if got := d.PendingCount(); got != 0 {
		t.Fatalf("清除后队列应为空，实得 %d", got)
	}
	hits, err := d.Query(ctx, "天气", TypeText, 5, QueryOptions{})
	if err != nil {
		t.Fatalf("向量检索失败: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("清除标记后两条都应可检索，实得 %+v", hits)
	}
}
