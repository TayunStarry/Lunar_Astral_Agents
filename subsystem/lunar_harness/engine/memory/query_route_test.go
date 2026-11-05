package memory

// query_route_test.go 覆盖"先簇后库"的查询路由：
//  1. 命中簇时只比较簇内成员，不扫全库；
//  2. 质心门限过滤：相似度不足的簇不进入簇内检索；
//  3. 成员门限过滤：簇内与查询不够相似的成员被跳过；
//  4. 无簇命中时回落全库扫描；
//  5. 回落补齐 topK，且不重复返回；
//  6. 关键词通道仍覆盖总库 + 簇内成员。

import (
	"context"
	"fmt"
	"testing"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
)

// routeDim 路由测试向量维度
const routeDim = 8

// routeBasis 返回第 i 个标准基向量。
func routeBasis(i int) []float32 {
	vec := make([]float32, routeDim)
	vec[i] = 1
	return vec
}

// routeMixed 返回 0.5*e0 + 0.866*e_g（与 e0 余弦 0.5、与 e_g 余弦 0.866）。
func routeMixed(g int) []float32 {
	vec := make([]float32, routeDim)
	vec[0] = 0.5
	vec[g] = 0.866
	return vec
}

// newRouteStore 构造不依赖真实嵌入服务的记忆库；查询文本 "qN" 映射到第 N 个基向量。
func newRouteStore(t *testing.T) *Store {
	return newRouteStoreDim(t, routeDim)
}

// newRouteStoreDim 同 newRouteStore，但可指定向量维度（合成数据基准用）。
func newRouteStoreDim(t testing.TB, dim int) *Store {
	t.Helper()
	origURL := config.AgentEmbeddingURL
	config.AgentEmbeddingURL = ""
	restore := setEmbedTextsHook(func(ctx context.Context, model string, texts []string) ([][]float32, error) {
		vectors := make([][]float32, len(texts))
		for i, text := range texts {
			vectors[i] = routeQueryVectorDim(text, dim)
		}
		return vectors, nil
	})
	t.Cleanup(func() {
		restore()
		config.AgentEmbeddingURL = origURL
	})

	d := newStore(t.TempDir())
	if err := d.EnsureReady(); err != nil {
		t.Fatalf("记忆库初始化失败: %v", err)
	}
	return d
}

// routeQueryVectorDim 从 "Instruct: …\nQuery:qN" 中解析 N，映射到 dim 维基向量。
func routeQueryVectorDim(text string, dim int) []float32 {
	idx := 0
	for i := 0; i < len(text); i++ {
		if text[i] == 'q' {
			idx = i
		}
	}
	var n int
	if _, err := fmt.Sscanf(text[idx:], "q%d", &n); err == nil && n >= 0 && n < dim {
		return routeBasisDim(n, dim)
	}
	return routeBasisDim(0, dim)
}

// appendDocs 直接向集合追加文档并落盘（绕过嵌入与判重，便于构造可控向量）。
func appendDocs(t testing.TB, c *Collection, docs []Document) {
	t.Helper()
	c.mu.Lock()
	c.Documents = append(c.Documents, docs...)
	c.mu.Unlock()
	if err := c.saveDocumentsToFile(); err != nil {
		t.Fatalf("写入集合 %s 文档失败: %v", c.Name, err)
	}
	c.updateLastModTime()
}

// routeDataset 基础数据集：20 条 e0（簇 A）、20 条 e1（簇 B）、105 条总库（15 条/组）。
//
// 总库向量 0.5*e0 + 0.866*e_g 与各簇质心余弦最高 0.866 < ClusterAdmitThreshold(0.90)，
// 不会误入簇；每组恰好 15 条 < ClusterUpgradeSize(16)，也不会升级成簇。
func buildRouteDataset(t *testing.T, d *Store) {
	t.Helper()
	docs := make([]Document, 0, 145)
	ts := int64(1)

	for i := 0; i < 20; i++ {
		docs = append(docs, Document{
			ID: fmt.Sprintf("a-%02d", i), Type: TypeText,
			Content: fmt.Sprintf("天气记录A%02d", i), Timestamp: ts, Embedding: routeBasis(0),
		})
		ts++
	}
	for i := 0; i < 20; i++ {
		docs = append(docs, Document{
			ID: fmt.Sprintf("b-%02d", i), Type: TypeText,
			Content: fmt.Sprintf("心情记录B%02d", i), Timestamp: ts, Embedding: routeBasis(1),
		})
		ts++
	}
	for g := 1; g <= 7; g++ {
		vec := routeMixed(g)
		for i := 0; i < 15; i++ {
			docs = append(docs, Document{
				ID: fmt.Sprintf("s-%d-%02d", g, i), Type: TypeText,
				Content: fmt.Sprintf("备注记录%d-%02d", g, i), Timestamp: ts, Embedding: vec,
			})
			ts++
		}
	}

	store, err := d.storeCollection()
	if err != nil {
		t.Fatalf("获取总库失败: %v", err)
	}
	appendDocs(t, store, docs)

	report, err := d.MigrateClusters()
	if err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	if report.ClusterCount != 2 {
		t.Fatalf("应升级 2 个正式簇，实得 %d", report.ClusterCount)
	}
	if report.AssignedCount != 40 {
		t.Fatalf("簇内条目应为 40，实得 %d", report.AssignedCount)
	}
	if report.StoreSize != 105 {
		t.Fatalf("总库条目应为 105，实得 %d", report.StoreSize)
	}
}

// TestClusterRouteHitsWithoutFullScan 覆盖"命中簇不扫全库"与"质心门限过滤"。
func TestClusterRouteHitsWithoutFullScan(t *testing.T) {
	d := newRouteStore(t)
	buildRouteDataset(t, d)

	results, stats, err := d.QueryWithStats(context.Background(), "q0", TypeText, 10, QueryOptions{})
	if err != nil {
		t.Fatalf("检索失败: %v", err)
	}
	if len(results) != 10 {
		t.Fatalf("应返回 10 条，实得 %d", len(results))
	}
	for _, r := range results {
		if len(r.ID) < 2 || r.ID[:2] != "a-" {
			t.Fatalf("命中应全部来自簇 A，实得 %s", r.ID)
		}
	}

	// 两个簇都比过质心，但簇 B 相似度 0 < 0.70 被门限跳过
	if stats.ClustersCompared != 2 || stats.ClustersHit != 1 {
		t.Fatalf("质心门限路由异常: compared=%d hit=%d", stats.ClustersCompared, stats.ClustersHit)
	}
	// 只比较簇 A 的 20 条成员，远小于全库 145 条
	if stats.ClusterDocs != 20 || stats.DocsCompared != 20 {
		t.Fatalf("应只比较簇 A 的 20 条，实得 cluster=%d total=%d", stats.ClusterDocs, stats.DocsCompared)
	}
	if stats.UsedFallback || stats.FallbackDocs != 0 {
		t.Fatalf("命中簇且取满 topK 时不应回落: %+v", stats)
	}
	if stats.DocsCompared >= 145 {
		t.Fatalf("比较条目数未减少: %d", stats.DocsCompared)
	}
}

// TestClusterRouteMissFallsBackToStore 覆盖"无簇命中 → 全库扫描"。
func TestClusterRouteMissFallsBackToStore(t *testing.T) {
	d := newRouteStore(t)
	buildRouteDataset(t, d)

	// q2 = e2：与簇 A/B 质心余弦均为 0，全部簇被门限过滤
	results, stats, err := d.QueryWithStats(context.Background(), "q2", TypeText, 10, QueryOptions{})
	if err != nil {
		t.Fatalf("检索失败: %v", err)
	}
	if len(results) != 10 {
		t.Fatalf("应返回 10 条，实得 %d", len(results))
	}
	for _, r := range results {
		if len(r.ID) < 2 || r.ID[:2] != "s-" {
			t.Fatalf("无簇命中时应命中总库条目，实得 %s", r.ID)
		}
	}
	if stats.ClustersHit != 0 {
		t.Fatalf("不应有簇通过质心门限，实得 %d", stats.ClustersHit)
	}
	if !stats.UsedFallback {
		t.Fatal("无簇命中时必须回落")
	}
	if stats.DocsCompared != 145 {
		t.Fatalf("回落应全库扫描 145 条，实得 %d", stats.DocsCompared)
	}
}

// TestClusterRouteFallbackFillsTopK 覆盖"回落补齐 topK 且不重复"。
func TestClusterRouteFallbackFillsTopK(t *testing.T) {
	d := newRouteStore(t)
	buildRouteDataset(t, d)

	results, stats, err := d.QueryWithStats(context.Background(), "q0", TypeText, 25, QueryOptions{})
	if err != nil {
		t.Fatalf("检索失败: %v", err)
	}
	if len(results) != 25 {
		t.Fatalf("应补齐到 25 条，实得 %d", len(results))
	}
	ids := make(map[string]bool, len(results))
	for _, r := range results {
		if ids[r.ID] {
			t.Fatalf("回落补齐出现重复 ID: %s", r.ID)
		}
		ids[r.ID] = true
	}
	if stats.ClusterDocs != 20 {
		t.Fatalf("簇内应比较 20 条，实得 %d", stats.ClusterDocs)
	}
	if !stats.UsedFallback {
		t.Fatal("结果不足 topK 时必须回落补齐")
	}
	// 回落阶段排除已返回的 20 条，比较 145-20=125 条
	if stats.FallbackDocs != 125 {
		t.Fatalf("回落应排除已命中 ID 后比较 125 条，实得 %d", stats.FallbackDocs)
	}
	if stats.DocsCompared != 20+125 {
		t.Fatalf("比较总数应为 145，实得 %d", stats.DocsCompared)
	}
}

// TestClusterRouteMemberGateFilters 覆盖"簇内成员门限过滤"：
// 手写索引让簇内同时存在与查询余弦 1.0 和 0.6 的成员，后者应被 ClusterMemberGate(0.80) 跳过。
func TestClusterRouteMemberGateFilters(t *testing.T) {
	d := newRouteStore(t)

	// 总库：6 条 e3（与查询余弦 0）
	store, err := d.storeCollection()
	if err != nil {
		t.Fatalf("获取总库失败: %v", err)
	}
	storeDocs := make([]Document, 0, 6)
	for i := 0; i < 6; i++ {
		storeDocs = append(storeDocs, Document{
			ID: fmt.Sprintf("s3-%02d", i), Type: TypeText,
			Content: fmt.Sprintf("总库备注%02d", i), Timestamp: int64(i + 1), Embedding: routeBasis(3),
		})
	}
	appendDocs(t, store, storeDocs)

	// 正式簇：10 条 e0（余弦 1.0）+ 8 条 0.6*e0+0.8*e1（余弦 0.6，低于成员门限）
	cluster, err := d.clusterForCollection("cluster-gate")
	if err != nil {
		t.Fatalf("创建簇集合失败: %v", err)
	}
	clusterDocs := make([]Document, 0, 18)
	clusterIDs := make([]string, 0, 18)
	mixed := make([]float32, routeDim)
	mixed[0], mixed[1] = 0.6, 0.8
	for i := 0; i < 10; i++ {
		id := fmt.Sprintf("g-close-%02d", i)
		clusterIDs = append(clusterIDs, id)
		clusterDocs = append(clusterDocs, Document{
			ID: id, Type: TypeText, Content: fmt.Sprintf("簇内近邻%02d", i),
			Timestamp: int64(20 + i), Embedding: routeBasis(0),
		})
	}
	for i := 0; i < 8; i++ {
		id := fmt.Sprintf("g-far-%02d", i)
		clusterIDs = append(clusterIDs, id)
		clusterDocs = append(clusterDocs, Document{
			ID: id, Type: TypeText, Content: fmt.Sprintf("簇内远邻%02d", i),
			Timestamp: int64(40 + i), Embedding: mixed,
		})
	}
	appendDocs(t, cluster, clusterDocs)

	// 手写索引：一个 18 条成员的正式簇（总库 6 条，合计 24）
	index := newClusterIndex(currentEmbeddingModel())
	index.Dimension = routeDim
	index.StoreSize = 6
	index.Clusters = []ClusterNode{{
		ID: "cluster-gate", Centroid: routeBasis(0), DocIDs: clusterIDs, Size: len(clusterIDs),
	}}
	for _, id := range clusterIDs {
		index.Assignments[id] = "cluster-gate"
	}
	index.UpdatedAt = time.Now().Unix()
	d.setClusterIndex(index, time.Now())
	if err := d.saveClusterIndexFile(index); err != nil {
		t.Fatalf("保存索引失败: %v", err)
	}

	results, stats, err := d.QueryWithStats(context.Background(), "q0", TypeText, 10, QueryOptions{})
	if err != nil {
		t.Fatalf("检索失败: %v", err)
	}
	if len(results) != 10 {
		t.Fatalf("应返回 10 条近邻条目，实得 %d", len(results))
	}
	for _, r := range results {
		if len(r.ID) < 8 || r.ID[:8] != "g-close-" {
			t.Fatalf("低于成员门限的条目不应返回，实得 %s", r.ID)
		}
	}
	// 18 条成员都参与比较，但只有 10 条通过门限
	if stats.ClusterDocs != 18 {
		t.Fatalf("簇内应比较 18 条，实得 %d", stats.ClusterDocs)
	}
	if stats.ClustersHit != 1 || stats.UsedFallback {
		t.Fatalf("路由观测异常: %+v", stats)
	}
}

// TestKeywordCoversWholeLibrary 关键词通道必须覆盖总库 + 簇内成员。
func TestKeywordCoversWholeLibrary(t *testing.T) {
	d := newRouteStore(t)
	buildRouteDataset(t, d)

	inCluster, err := d.KeywordSearch(TypeText, []string{"天气记录A00"}, false, 5)
	if err != nil {
		t.Fatalf("关键词检索失败: %v", err)
	}
	if len(inCluster) == 0 || inCluster[0].ID != "a-00" {
		t.Fatalf("应命中簇内成员 a-00，实得 %+v", inCluster)
	}

	inStore, err := d.KeywordSearch(TypeText, []string{"备注记录3-00"}, false, 5)
	if err != nil {
		t.Fatalf("关键词检索失败: %v", err)
	}
	if len(inStore) == 0 || inStore[0].ID != "s-3-00" {
		t.Fatalf("应命中总库条目 s-3-00，实得 %+v", inStore)
	}
}

// TestHybridBudgetUnchanged 混合检索仍共享同一个 topK 预算。
func TestHybridBudgetUnchanged(t *testing.T) {
	d := newRouteStore(t)
	buildRouteDataset(t, d)

	for _, topK := range []int{3, 10} {
		results, err := d.HybridQuery(context.Background(), []string{"天气记录A", "q0"}, TypeText, topK, HybridQueryOptions{})
		if err != nil {
			t.Fatalf("混合检索失败: %v", err)
		}
		if len(results) > topK {
			t.Fatalf("topK=%d 时返回 %d 条，超出预算", topK, len(results))
		}
		if len(results) == 0 {
			t.Fatalf("topK=%d 时不应为空", topK)
		}
	}
}

// TestMigrateSweepsUndersizedClusters 迁移扫尾把不足 ClusterDowngradeSize 的簇解散回总库。
func TestMigrateSweepsUndersizedClusters(t *testing.T) {
	d := newRouteStore(t)

	// 总库 6 条
	store, err := d.storeCollection()
	if err != nil {
		t.Fatalf("获取总库失败: %v", err)
	}
	storeDocs := make([]Document, 0, 6)
	for i := 0; i < 6; i++ {
		storeDocs = append(storeDocs, Document{
			ID: fmt.Sprintf("s-%02d", i), Type: TypeText,
			Content: fmt.Sprintf("总库条目%02d", i), Timestamp: int64(i + 1), Embedding: routeBasis(2),
		})
	}
	appendDocs(t, store, storeDocs)

	// 构造一个只有 4 条成员的历史簇（正常路径不会产生，模拟异常/复用索引残留）
	cluster, err := d.clusterForCollection("cluster-tiny")
	if err != nil {
		t.Fatalf("创建簇集合失败: %v", err)
	}
	tinyIDs := make([]string, 0, 4)
	tinyDocs := make([]Document, 0, 4)
	for i := 0; i < 4; i++ {
		id := fmt.Sprintf("t-%02d", i)
		tinyIDs = append(tinyIDs, id)
		tinyDocs = append(tinyDocs, Document{
			ID: id, Type: TypeText, Content: fmt.Sprintf("小簇条目%02d", i),
			Timestamp: int64(10 + i), Embedding: routeBasis(0),
		})
	}
	appendDocs(t, cluster, tinyDocs)

	index := newClusterIndex(currentEmbeddingModel())
	index.Dimension = routeDim
	index.StoreSize = 6
	index.Clusters = []ClusterNode{{
		ID: "cluster-tiny", Centroid: routeBasis(0), DocIDs: tinyIDs, Size: len(tinyIDs),
	}}
	for _, id := range tinyIDs {
		index.Assignments[id] = "cluster-tiny"
	}
	d.setClusterIndex(index, time.Now())
	if err := d.saveClusterIndexFile(index); err != nil {
		t.Fatalf("保存索引失败: %v", err)
	}

	if dissolved := d.sweepUndersizedClusters(); dissolved != 1 {
		t.Fatalf("应解散 1 个不足下限的簇，实得 %d", dissolved)
	}

	after := d.cloneClusterIndex()
	if len(after.Clusters) != 0 || len(after.Assignments) != 0 {
		t.Fatalf("簇与归属应清空: clusters=%d assignments=%d", len(after.Clusters), len(after.Assignments))
	}
	if after.StoreSize != 10 {
		t.Fatalf("解散后总库应为 10 条，实得 %d", after.StoreSize)
	}

	// 总库集合现在包含 10 条（6 + 4 搬迁）
	store, err = d.storeCollection()
	if err != nil {
		t.Fatalf("获取总库失败: %v", err)
	}
	store.mu.RLock()
	got := len(store.Documents)
	store.mu.RUnlock()
	if got != 10 {
		t.Fatalf("总库应有 10 条，实得 %d", got)
	}
}
