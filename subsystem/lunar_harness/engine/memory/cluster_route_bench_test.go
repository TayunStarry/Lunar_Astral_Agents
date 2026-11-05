package memory

// cluster_route_bench_test.go 用 10,000 条合成向量验证「簇路由」的实际收益：
//
//   - 5 组、每组 20 条高相似（余弦 ≥ 0.99）→ 迁移后升级为 5 个正式簇；
//   - 其余 9,900 条为稀疏背景向量，任意两条共享分量不足（相似度 ≤ 0.667），
//     不会凑够 ClusterUpgradeSize(16) 条，稳定留在总库；
//   - 对比「簇路由」与「强制全局全扫」的比较条目数、topK 一致性与耗时。
//
// 数字全部用 t.Logf 输出，不做易抖动的耗时硬断言。

import (
	"context"
	"fmt"
	"testing"
	"time"
)

const (
	benchDim        = 32   // 向量维度
	benchGroups     = 5    // 高相似组数量
	benchGroupSize  = 20   // 每组条数（≥ ClusterUpgradeSize）
	benchBackground = 9900 // 背景条数，总条目 = 5*20 + 9900 = 10,000
	benchTopK       = 10
)

// benchSparseVector 返回三个非零分量构造的稀疏单位向量（位置互不相同即相似度 ≤ 0.667）。
func benchSparseVector(p1, p2, p3 int) []float32 {
	vec := make([]float32, benchDim)
	const w = 0.577350269
	vec[p1], vec[p2], vec[p3] = w, w, w
	return vec
}

// benchTriples 枚举 [5, benchDim) 内全部三元组（保证每条背景向量组合唯一可复现）。
func benchTriples() [][3]int {
	triples := make([][3]int, 0, 3000)
	for i := 5; i < benchDim; i++ {
		for j := i + 1; j < benchDim; j++ {
			for k := j + 1; k < benchDim; k++ {
				triples = append(triples, [3]int{i, j, k})
			}
		}
	}
	return triples
}

// seedSyntheticDataset 写入 10,000 条合成文档到总库（绕过嵌入与判重）。
func seedSyntheticDataset(t testing.TB, d *Store) {
	t.Helper()
	docs := make([]Document, 0, benchGroups*benchGroupSize+benchBackground)
	ts := int64(1)

	// 5 组高相似：同组向量完全相同，组间正交
	for g := 0; g < benchGroups; g++ {
		for i := 0; i < benchGroupSize; i++ {
			docs = append(docs, Document{
				ID: fmt.Sprintf("g%d-%02d", g, i), Type: TypeText,
				Content: fmt.Sprintf("高相似主题%d第%02d条", g, i), Timestamp: ts, Embedding: routeBasisDim(g, benchDim),
			})
			ts++
		}
	}

	// 9,900 条分散背景：三元组轮转分配，同组合最多 ~4 条，稳定留在总库
	triples := benchTriples()
	for i := 0; i < benchBackground; i++ {
		tp := triples[i%len(triples)]
		docs = append(docs, Document{
			ID: fmt.Sprintf("s-%04d", i), Type: TypeText,
			Content: fmt.Sprintf("背景记录%04d", i), Timestamp: ts, Embedding: benchSparseVector(tp[0], tp[1], tp[2]),
		})
		ts++
	}

	store, err := d.storeCollection()
	if err != nil {
		t.Fatalf("获取总库失败: %v", err)
	}
	appendDocs(t, store, docs)
}

// routeBasisDim 返回 dim 维的第 i 个标准基向量。
func routeBasisDim(i, dim int) []float32 {
	vec := make([]float32, dim)
	if i < dim {
		vec[i] = 1
	}
	return vec
}

// idSet 把检索结果转成 ID 集合。
func idSet(entries []Entry) map[string]bool {
	out := make(map[string]bool, len(entries))
	for _, e := range entries {
		out[e.ID] = true
	}
	return out
}

// TestClusterRouteSyntheticBenchmark 10,000 条合成数据的路由收益与一致性验证。
func TestClusterRouteSyntheticBenchmark(t *testing.T) {
	if testing.Short() {
		t.Skip("合成数据基准在 -short 下跳过")
	}

	d := newRouteStoreDim(t, benchDim)
	seedSyntheticDataset(t, d)

	storeCollection, err := d.storeCollection()
	if err != nil {
		t.Fatalf("获取总库失败: %v", err)
	}
	storeCollection.mu.RLock()
	storeDocsBefore := len(storeCollection.Documents)
	storeCollection.mu.RUnlock()

	// 建簇前：全部条目在总库，没有任何正式簇
	t.Logf("建簇前：总条目=%d，正式簇=0，总库条目=%d", storeDocsBefore, storeDocsBefore)

	migrateStart := time.Now()
	report, err := d.MigrateClusters()
	if err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	t.Logf("迁移耗时: %v", time.Since(migrateStart))

	if report.ClusterCount < benchGroups {
		t.Fatalf("应至少升级 %d 个正式簇，实得 %d", benchGroups, report.ClusterCount)
	}
	if report.DocCount != benchGroups*benchGroupSize+benchBackground {
		t.Fatalf("迁移条目数 = %d，期望 10000", report.DocCount)
	}
	if report.StoreSize+report.AssignedCount != report.DocCount {
		t.Fatalf("对账失败：StoreSize(%d)+AssignedCount(%d)≠DocCount(%d)",
			report.StoreSize, report.AssignedCount, report.DocCount)
	}
	t.Logf("建簇后：正式簇=%d，簇内条目=%d，总库条目=%d，条目合计=%d",
		report.ClusterCount, report.AssignedCount, report.StoreSize, report.DocCount)

	ctx := context.Background()
	queries := make([]string, 0, benchGroups)
	for g := 0; g < benchGroups; g++ {
		queries = append(queries, fmt.Sprintf("q%d", g))
	}

	// 一致性 + 比较条目数对比
	consistent := 0
	var routeCompared, globalCompared int
	for _, query := range queries {
		routed, routeStats, routeErr := d.QueryWithStats(ctx, query, TypeText, benchTopK, QueryOptions{})
		if routeErr != nil {
			t.Fatalf("路由检索 %q 失败: %v", query, routeErr)
		}
		global, globalStats, globalErr := d.QueryWithStats(ctx, query, TypeText, benchTopK, QueryOptions{ForceGlobal: true})
		if globalErr != nil {
			t.Fatalf("全扫检索 %q 失败: %v", query, globalErr)
		}
		if len(routed) != len(global) {
			t.Fatalf("查询 %q 结果数不一致：路由 %d，全扫 %d", query, len(routed), len(global))
		}
		if fmt.Sprint(idSet(routed)) == fmt.Sprint(idSet(global)) {
			consistent++
		} else {
			t.Errorf("查询 %q topK 命中集合不一致：路由 %v，全扫 %v",
				query, idSet(routed), idSet(global))
		}
		routeCompared += routeStats.DocsCompared
		globalCompared += globalStats.DocsCompared
		t.Logf("查询 %q：路由比较 %d 条（簇内 %d，回落 %d），全扫比较 %d 条",
			query, routeStats.DocsCompared, routeStats.ClusterDocs, routeStats.FallbackDocs, globalStats.DocsCompared)
	}
	t.Logf("比较条目数：路由合计 %d，全扫合计 %d，比例 %.2f%%",
		routeCompared, globalCompared,
		100*float64(routeCompared)/float64(globalCompared))
	t.Logf("topK 一致性：%d/%d 组查询命中 ID 集合完全相同", consistent, len(queries))

	// 耗时对比交给 BenchmarkClusterRouteQuery（testing.B 以 b.N 累积计时，避免
	// Windows time.Now 亚毫秒粒度把读数抹成 0）：
	//   go test -run '^$' -bench BenchmarkClusterRouteQuery -count=5 ./engine/memory/

	// 数据完整性：建簇后条目数不变
	total := 0
	for _, c := range d.snapshotCollections() {
		c.mu.RLock()
		total += len(c.Documents)
		c.mu.RUnlock()
	}
	if total != benchGroups*benchGroupSize+benchBackground {
		t.Fatalf("建簇后条目总数 = %d，期望 10000", total)
	}
	t.Logf("建簇后落盘条目合计=%d（总库+簇目录一致）", total)
}

// BenchmarkClusterRouteQuery 同一数据集下对比「簇路由」与「强制全库扫描」的单次查询耗时。
//
// setup（建 10,000 条 + 迁移）在计时外完成；两个子基准都以 b.N 循环累积计时，
// 结果即 ns/op（均值）。配合 -count=5 可得到 5 组样本，用于报均值与中位数。
func BenchmarkClusterRouteQuery(b *testing.B) {
	if testing.Short() {
		b.Skip("合成数据基准在 -short 下跳过")
	}
	d := newRouteStoreDim(b, benchDim)
	seedSyntheticDataset(b, d)
	if _, err := d.MigrateClusters(); err != nil {
		b.Fatalf("迁移失败: %v", err)
	}

	ctx := context.Background()
	queries := make([]string, 0, benchGroups)
	for g := 0; g < benchGroups; g++ {
		queries = append(queries, fmt.Sprintf("q%d", g))
	}

	run := func(b *testing.B, opts QueryOptions) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			query := queries[i%len(queries)]
			if _, _, err := d.QueryWithStats(ctx, query, TypeText, benchTopK, opts); err != nil {
				b.Fatalf("检索失败: %v", err)
			}
		}
	}

	b.Run("route", func(b *testing.B) { run(b, QueryOptions{}) })
	b.Run("global", func(b *testing.B) { run(b, QueryOptions{ForceGlobal: true}) })
}
