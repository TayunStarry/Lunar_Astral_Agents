package memory

// cluster_test.go 记忆整理候选聚类与整理结果向量校验的回归测试。

import (
	"context"
	"testing"
	"time"
)

// TestClusterTextDocsMergesSimilarAcrossTime 相似条目即使入库时间相隔很远也应聚成一簇。
func TestClusterTextDocsMergesSimilarAcrossTime(t *testing.T) {
	docs := []Document{
		{ID: "old", Content: "今天天气不错", Embedding: []float32{1, 0, 0, 0}, Timestamp: 100},
		{ID: "mid", Content: "今天天气很好", Embedding: []float32{1, 0, 0, 0}, Timestamp: 2000},
		{ID: "new", Content: "今天天气晴朗", Embedding: []float32{1, 0, 0, 0}, Timestamp: 500000},
		{ID: "other", Content: "项目上线了", Embedding: []float32{0, 1, 0, 0}, Timestamp: 3000},
	}
	clusters := clusterTextDocs(docs, 0.90, ClusterMaxItems, ClusterMaxRunes)
	if len(clusters) != 2 {
		t.Fatalf("应得到 1 个相似簇 + 1 个独立条目，实际 %d 个簇", len(clusters))
	}
	if len(clusters[0].indices) != 3 {
		t.Fatalf("跨时间相似条目应合并，实际 %d 条", len(clusters[0].indices))
	}
	if clusters[0].indices[0] != 0 || clusters[0].indices[2] != 2 {
		t.Fatalf("簇内应保持最久条目为代表，实际索引 %v", clusters[0].indices)
	}
	if len(clusters[1].indices) != 1 {
		t.Fatalf("不相似条目不应并入，实际 %d 条", len(clusters[1].indices))
	}
}

// TestClusterTextDocsSkipsSingletons 没有相似伙伴的条目不应被硬凑成簇。
func TestClusterTextDocsSkipsSingletons(t *testing.T) {
	docs := []Document{
		{ID: "a", Content: "天气", Embedding: []float32{1, 0}},
		{ID: "b", Content: "代码", Embedding: []float32{0, 1}},
	}
	clusters := clusterTextDocs(docs, 0.90, ClusterMaxItems, ClusterMaxRunes)
	if len(clusters) != 2 {
		t.Fatalf("两条不相似条目应各自成单例簇，实际 %d", len(clusters))
	}
	for _, cluster := range clusters {
		if len(cluster.indices) != 1 {
			t.Fatalf("单例簇不应包含多条，实际 %d", len(cluster.indices))
		}
	}
}

// TestClusterTextCandidatesOnlyReturnsMergeableGroups 对外只返回 ≥2 条的簇。
func TestClusterTextCandidatesOnlyReturnsMergeableGroups(t *testing.T) {
	d, _ := setupReplaceStore(t)
	ctx := context.Background()

	ids := addTexts(t, d, "旧天气记忆", "新天气记忆", "代码记忆")
	now := time.Now().Unix()
	// 把第一条调整为更久以前：跨时间相似条目仍应聚成一簇
	for _, c := range d.snapshotCollections() {
		c.mu.Lock()
		for i := range c.Documents {
			switch c.Documents[i].ID {
			case ids[0]:
				c.Documents[i].Timestamp = now - 60*24*3600
			case ids[1], ids[2]:
				c.Documents[i].Timestamp = now
			}
		}
		c.mu.Unlock()
		if err := c.saveDocumentsToFile(); err != nil {
			t.Fatalf("写入测试时间戳失败: %v", err)
		}
	}

	clusters, candidates, err := d.ClusterTextCandidates(ctx, "", ids, 0.90)
	if err != nil {
		t.Fatalf("聚类失败: %v", err)
	}
	if candidates != 3 {
		t.Fatalf("候选数应为 3，实际 %d", candidates)
	}
	if len(clusters) != 1 || len(clusters[0].IDs) != 2 {
		t.Fatalf("应聚成 1 个双条簇，实际 %+v", clusters)
	}
	if clusters[0].IDs[0] != ids[0] || clusters[0].IDs[1] != ids[1] {
		t.Fatalf("簇内应按时间升序，实际 %v", clusters[0].IDs)
	}
	if clusters[0].OldestTimestamp >= clusters[0].NewestTimestamp {
		t.Fatalf("跨时间簇的时间范围异常: %+v", clusters[0])
	}
}

// TestVerifyReplacementSimilarityRejectsDriftAndLoss 结果跑偏或丢事实都应被拒绝。
func TestVerifyReplacementSimilarityRejectsDriftAndLoss(t *testing.T) {
	d, _ := setupReplaceStore(t)
	ctx := context.Background()

	weatherIDs := addTexts(t, d, "今天天气不错")
	if err := d.VerifyReplacementSimilarity(ctx, weatherIDs, []string{"整理后天气不错"}, ClusterVerifyThreshold); err != nil {
		t.Fatalf("同义整理结果应通过校验: %v", err)
	}
	if err := d.VerifyReplacementSimilarity(ctx, weatherIDs, []string{"写代码很顺利"}, ClusterVerifyThreshold); err == nil {
		t.Fatal("语义跑偏的整理结果应被拒绝")
	}

	moodIDs, err := d.AddText(ctx, "user", "今天心情很好")
	if err != nil {
		t.Fatalf("写入心情记忆失败: %v", err)
	}
	if err := d.VerifyReplacementSimilarity(ctx, []string{moodIDs}, []string{"整理后天气不错"}, ClusterVerifyThreshold); err == nil {
		t.Fatal("丢事实的整理结果应被拒绝")
	}
}
