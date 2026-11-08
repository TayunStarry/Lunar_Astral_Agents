package memory

// pending_test.go 待处理（待合并）队列的回归测试：
// 相似写入不再丢弃、检索隐藏、整理成功清标记、写入上限不丢数据。

import (
	"context"
	"strings"
	"testing"
	"time"
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

// TestRecordPendingFailureReleasesAfterRetryLimit 连续失败达到上限后解除待处理标记，
// 条目重新对三路检索可见；未达上限时仍保持待处理。
func TestRecordPendingFailureReleasesAfterRetryLimit(t *testing.T) {
	d, _ := setupReplaceStore(t)
	ctx := context.Background()

	if _, err := d.AddText(ctx, "user", "今天天气不错"); err != nil {
		t.Fatalf("写入第一条失败: %v", err)
	}
	second, err := d.AddText(ctx, "user", "今天天气很好")
	if err != nil {
		t.Fatalf("写入相似条目失败: %v", err)
	}

	first, err := d.RecordPendingFailure(ctx, []string{second}, 2, 0)
	if err != nil {
		t.Fatalf("记录失败状态出错: %v", err)
	}
	if len(first.Released) != 0 || len(first.Retained) != 1 {
		t.Fatalf("第一次失败应保留待处理: %+v", first)
	}
	entry, err := d.Get(second)
	if err != nil {
		t.Fatalf("读取条目失败: %v", err)
	}
	if !entry.Pending {
		t.Fatal("未达重试上限前不应解除待处理标记")
	}

	secondRes, err := d.RecordPendingFailure(ctx, []string{second}, 2, 0)
	if err != nil {
		t.Fatalf("记录失败状态出错: %v", err)
	}
	if len(secondRes.Released) != 1 || len(secondRes.Retained) != 0 {
		t.Fatalf("达到上限应放行: %+v", secondRes)
	}
	entry, err = d.Get(second)
	if err != nil {
		t.Fatalf("读取条目失败: %v", err)
	}
	if entry.Pending || entry.PendingPartner != "" {
		t.Fatalf("放行后不应残留待处理标记: %+v", entry)
	}
	if got := d.PendingCount(); got != 0 {
		t.Fatalf("放行后队列应为空，实得 %d", got)
	}
	hits, err := d.Query(ctx, "天气", TypeText, 5, QueryOptions{})
	if err != nil {
		t.Fatalf("向量检索失败: %v", err)
	}
	found := false
	for _, hit := range hits {
		if hit.ID == second {
			found = true
		}
	}
	if !found {
		t.Fatalf("放行后条目应重新对检索可见: %+v", hits)
	}
}

// TestRecordPendingFailureReleasesExpiredPending 隐身超过时长上限时立即放行，
// 即使失败次数尚未达到上限。
func TestRecordPendingFailureReleasesExpiredPending(t *testing.T) {
	d, _ := setupReplaceStore(t)
	ctx := context.Background()

	if _, err := d.AddText(ctx, "user", "今天天气不错"); err != nil {
		t.Fatalf("写入第一条失败: %v", err)
	}
	second, err := d.AddText(ctx, "user", "今天天气很好")
	if err != nil {
		t.Fatalf("写入相似条目失败: %v", err)
	}

	// 时长下限压到 1ns：超过 0 秒即视为隐身超时（不依赖内部字段手工改写）
	res, err := d.RecordPendingFailure(ctx, []string{second}, 10, time.Nanosecond)
	if err != nil {
		t.Fatalf("记录失败状态出错: %v", err)
	}
	if len(res.Released) != 1 {
		t.Fatalf("超时后应放行: %+v", res)
	}
	entry, err := d.Get(second)
	if err != nil {
		t.Fatalf("读取条目失败: %v", err)
	}
	if entry.Pending {
		t.Fatal("超时放行后不应仍为待处理")
	}
}

// TestTaggedWriteUsesContentVectorForDedupAndCluster 标签向量不同但正文相同，
// 判重与聚类仍应命中正文，不能被标签向量拆散。
func TestTaggedWriteUsesContentVectorForDedupAndCluster(t *testing.T) {
	d, _ := setupReplaceStore(t)
	ctx := context.Background()

	// 两条正文同为「天气」向量，标签分别命中「开心」与「代码」两个不同向量
	first, err := d.AddTextWithTags(ctx, "user", "今天天气不错", []string{"开心"}, "")
	if err != nil {
		t.Fatalf("写入第一条失败: %v", err)
	}
	second, err := d.AddTextWithTags(ctx, "user", "今天天气很好", []string{"代码"}, "")
	if err != nil {
		t.Fatalf("写入第二条失败: %v", err)
	}

	entry, err := d.Get(second)
	if err != nil {
		t.Fatalf("读取第二条失败: %v", err)
	}
	if !entry.Pending {
		t.Fatalf("正文相同应判重为待处理，实得 %+v", entry)
	}

	clusters, _, err := d.ClusterTextCandidates(ctx, "", []string{first, second}, ClusterDefaultThreshold)
	if err != nil {
		t.Fatalf("聚类失败: %v", err)
	}
	if len(clusters) != 1 || len(clusters[0].IDs) != 2 {
		t.Fatalf("正文相同应聚为一簇，实得 %+v", clusters)
	}

	// 覆盖校验同样基于正文向量，应能通过
	if err := d.VerifyReplacementSimilarity(ctx, []string{first, second}, []string{"今天天气很好，心情不错"}, 0.80); err != nil {
		t.Fatalf("基于正文向量的覆盖校验应通过: %v", err)
	}
}

// TestVerifyReplacementSimilarityRejectsDimensionMismatch 来源向量维度不一致时必须报错，
// 不能静默跳过——被跳过的来源仍会被替换流程删除，等于「删了但没校验」。
func TestVerifyReplacementSimilarityRejectsDimensionMismatch(t *testing.T) {
	d, _ := setupReplaceStore(t)
	ctx := context.Background()

	first, err := d.AddText(ctx, "user", "今天天气不错")
	if err != nil {
		t.Fatalf("写入第一条失败: %v", err)
	}
	second, err := d.AddText(ctx, "user", "心情很开心")
	if err != nil {
		t.Fatalf("写入第二条失败: %v", err)
	}

	mutated := false
	for _, c := range d.snapshotCollections() {
		c.mu.Lock()
		for i := range c.Documents {
			if c.Documents[i].ID == second {
				// 故意改成 3 维，与其余条目的 4 维不一致
				c.Documents[i].ContentEmbedding = []float32{1, 0, 0}
				mutated = true
			}
		}
		c.mu.Unlock()
		if mutated {
			if err := c.saveDocumentsToFile(); err != nil {
				t.Fatalf("保存维度异常数据失败: %v", err)
			}
			break
		}
	}
	if !mutated {
		t.Fatal("未找到目标条目")
	}

	err = d.VerifyReplacementSimilarity(ctx, []string{first, second}, []string{"今天天气不错"}, 0.80)
	if err == nil {
		t.Fatal("来源向量维度不一致时必须拒绝校验，不能静默跳过")
	}
	if !strings.Contains(err.Error(), "维度不一致") {
		t.Fatalf("错误信息应说明维度不一致，实得: %v", err)
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
