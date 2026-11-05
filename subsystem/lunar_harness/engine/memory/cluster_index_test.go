package memory

// cluster_index_test.go 覆盖"总库为主 + 正式簇加速"架构的关键性质：
//  1. 少量条目（< ClusterUpgradeSize=16）全留总库，不建正式簇；
//  2. 迁移/重建时，高相似组 ≥16 条才升级为正式簇；
//  3. 重建聚类幂等，同一批数据重复重建得到相同归属；
//  4. 迁移前备份、迁移后 StoreSize + AssignedCount == DocCount 对账；
//  5. 嵌入服务不可用时迁移仍可完成（只读已持久化向量）；
//  6. 迁移前后抽样检索的 topK 命中一致。

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
)

// clusterTestVector 确定性测试向量：按主题映射到不同正交方向。
func clusterTestVector(text string) []float32 {
	switch {
	case strings.Contains(text, "天气"):
		return []float32{1, 0, 0, 0}
	case strings.Contains(text, "心情"):
		return []float32{0, 1, 0, 0}
	case strings.Contains(text, "代码"):
		return []float32{0, 0, 1, 0}
	case strings.Contains(text, "吃饭"):
		return []float32{0, 0, 0, 1}
	}
	return []float32{0.5, 0.5, 0.5, 0.5}
}

// newClusterTestStore 构造不依赖真实嵌入服务的测试记忆库（通过注入点提供向量）。
func newClusterTestStore(t *testing.T) *Store {
	t.Helper()
	origURL := config.AgentEmbeddingURL
	config.AgentEmbeddingURL = ""
	restore := setEmbedTextsHook(func(ctx context.Context, model string, texts []string) ([][]float32, error) {
		vectors := make([][]float32, len(texts))
		for i, text := range texts {
			vectors[i] = clusterTestVector(text)
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

// resetClusterIndex 模拟"旧库尚无聚类索引"：删除磁盘索引并清空内存状态。
func resetClusterIndex(t *testing.T, d *Store) {
	t.Helper()
	if err := os.Remove(clusterIndexPath(d.baseDir)); err != nil && !os.IsNotExist(err) {
		t.Fatalf("删除聚类索引失败: %v", err)
	}
	d.clusterMu.Lock()
	d.clusterIndex = nil
	d.clusterMu.Unlock()
}

// TestSmallGroupsStayInStore 少量条目（< 16）全留总库，不建正式簇。
func TestSmallGroupsStayInStore(t *testing.T) {
	d := newClusterTestStore(t)
	ctx := context.Background()

	// 2 条天气 + 1 条心情 → 3 条，都 < 16
	idWeather1, err := d.AddText(ctx, "user", "今天天气不错")
	if err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	idWeather2, err := d.AddText(ctx, "user", "今天天气很好")
	if err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	idMood, err := d.AddText(ctx, "user", "心情很开心")
	if err != nil {
		t.Fatalf("写入失败: %v", err)
	}

	// 新语义：写入时不自动升簇，全部在总库
	for _, id := range []string{idWeather1, idWeather2, idMood} {
		entry, err := d.Get(id)
		if err != nil {
			t.Fatalf("读取 %s 失败: %v", id, err)
		}
		if entry.Collection != StoreCollectionID {
			t.Fatalf("条目 %s 应在总库 store-all，实得 %q", id, entry.Collection)
		}
	}

	// 迁移触发扫描
	resetClusterIndex(t, d)
	report, err := d.MigrateClusters()
	if err != nil {
		t.Fatalf("迁移失败: %v", err)
	}

	// 3 条都 < 16，没有达标的组
	if report.ClusterCount != 0 {
		t.Fatalf("3 条不足以建簇，实得 %d 个簇", report.ClusterCount)
	}
	if report.AssignedCount != 0 {
		t.Fatalf("归属应全部为空，实得 %d", report.AssignedCount)
	}
	if report.StoreSize != 3 {
		t.Fatalf("总库应有 3 条，实得 %d", report.StoreSize)
	}

	index := d.cloneClusterIndex()
	if index == nil {
		t.Fatal("聚类索引未建立")
	}
	if len(index.Clusters) != 0 {
		t.Fatalf("迁移后不应有正式簇，实得 %d", len(index.Clusters))
	}
	if len(index.Assignments) != 0 {
		t.Fatalf("不应有归属条目，实得 %d", len(index.Assignments))
	}
}

// TestClusterUpgradesAt16 高相似条目达到 16 条时升级为正式簇。
func TestClusterUpgradesAt16(t *testing.T) {
	d := newClusterTestStore(t)
	ctx := context.Background()

	// 先写 15 条含"天气"的文本 → 向量全相同 {1,0,0,0}，但 < 16
	for i := 0; i < ClusterUpgradeSize-1; i++ {
		if _, err := d.AddText(ctx, "user", fmt.Sprintf("天气第%d条", i)); err != nil {
			t.Fatalf("写入第 %d 条失败: %v", i, err)
		}
	}

	resetClusterIndex(t, d)
	report, err := d.MigrateClusters()
	if err != nil {
		t.Fatalf("15 条迁移失败: %v", err)
	}
	if report.ClusterCount != 0 || report.AssignedCount != 0 {
		t.Fatalf("15 条不应升簇：ClusterCount=%d AssignedCount=%d",
			report.ClusterCount, report.AssignedCount)
	}

	// 加入第 16 条 → 触发升簇
	if _, err := d.AddText(ctx, "user", "天气第15条"); err != nil {
		t.Fatalf("写入第 16 条失败: %v", err)
	}
	resetClusterIndex(t, d)
	report, err = d.MigrateClusters()
	if err != nil {
		t.Fatalf("16 条迁移失败: %v", err)
	}
	if report.ClusterCount != 1 {
		t.Fatalf("16 条高相似应升 1 个簇，实得 %d", report.ClusterCount)
	}
	if report.AssignedCount != ClusterUpgradeSize {
		t.Fatalf("正式簇应有 %d 条，实得 %d", ClusterUpgradeSize, report.AssignedCount)
	}
	if report.StoreSize != 0 {
		t.Fatalf("升级后总库应为 0（全部迁出），实得 %d", report.StoreSize)
	}
}

// TestClusterRebuildIdempotent 覆盖重建幂等：同一批条目重复重建，归属与簇 ID 完全一致。
func TestClusterRebuildIdempotent(t *testing.T) {
	// 构造一批包含 ≥16 条同主题的数据，验证重建幂等
	docs := make([]Document, 0, ClusterUpgradeSize*2)
	for i := 0; i < ClusterUpgradeSize; i++ {
		docs = append(docs, Document{
			ID:        fmt.Sprintf("w%d", i),
			Type:      TypeText,
			Content:   fmt.Sprintf("天气%d", i),
			Timestamp: int64(i + 1),
			Embedding: []float32{1, 0, 0, 0},
		})
	}
	for i := 0; i < 5; i++ {
		docs = append(docs, Document{
			ID:        fmt.Sprintf("m%d", i),
			Type:      TypeText,
			Content:   fmt.Sprintf("心情%d", i),
			Timestamp: int64(ClusterUpgradeSize + i + 1),
			Embedding: []float32{0, 1, 0, 0},
		})
	}

	first, _ := buildClusterIndexFromDocs(docs, "test-model")
	second, _ := buildClusterIndexFromDocs(docs, "test-model")

	if !reflect.DeepEqual(first.Assignments, second.Assignments) {
		t.Fatalf("重建归属发生漂移：\n第一次 %v\n第二次 %v", first.Assignments, second.Assignments)
	}
	if !reflect.DeepEqual(first.Clusters, second.Clusters) {
		t.Fatalf("重建簇结构发生漂移：\n第一次 %+v\n第二次 %+v", first.Clusters, second.Clusters)
	}

	// 天气组 16 条 → 1 个正式簇；心情组 5 条 → 留总库
	if len(first.Clusters) != 1 {
		t.Fatalf("应只有 1 个正式簇（天气组 16 条），实得 %d", len(first.Clusters))
	}
	if len(first.Assignments) != ClusterUpgradeSize {
		t.Fatalf("归属应 %d 条，实得 %d", ClusterUpgradeSize, len(first.Assignments))
	}
	if first.Clusters[0].Size != ClusterUpgradeSize {
		t.Fatalf("正式簇应有 %d 条，实得 %d", ClusterUpgradeSize, first.Clusters[0].Size)
	}
	if first.StoreSize != 5 {
		t.Fatalf("总库应为 5 条（心情组），实得 %d", first.StoreSize)
	}
}

// TestClusterMigrationReconcileAndIdempotent 覆盖迁移对账与幂等：
// StoreSize + AssignedCount == DocCount；重复迁移结果一致。
func TestClusterMigrationReconcileAndIdempotent(t *testing.T) {
	d := newClusterTestStore(t)

	// 构造 16 条天气 + 4 条心情 = 20 条
	ids := make([]string, 0, 20)
	ctx := context.Background()
	for i := 0; i < 16; i++ {
		id, err := d.AddText(ctx, "user", fmt.Sprintf("天气第%d条", i))
		if err != nil {
			t.Fatalf("写入失败: %v", err)
		}
		ids = append(ids, id)
	}
	for i := 0; i < 4; i++ {
		id, err := d.AddText(ctx, "user", fmt.Sprintf("心情第%d条", i))
		if err != nil {
			t.Fatalf("写入失败: %v", err)
		}
		ids = append(ids, id)
	}

	resetClusterIndex(t, d)
	report, err := d.MigrateClusters()
	if err != nil {
		t.Fatalf("迁移失败: %v", err)
	}

	if report.DocCount != 20 {
		t.Fatalf("迁移条目数 = %d，期望 20", report.DocCount)
	}
	if report.VectorCount != 20 {
		t.Fatalf("向量数应为 20，实得 %d", report.VectorCount)
	}
	// 16 天气 → 升 1 簇；4 心情 → 留总库
	if report.ClusterCount != 1 {
		t.Fatalf("应 1 个正式簇，实得 %d", report.ClusterCount)
	}
	if report.AssignedCount != 16 {
		t.Fatalf("归属应为 16，实得 %d", report.AssignedCount)
	}
	if report.StoreSize != 4 {
		t.Fatalf("总库应为 4，实得 %d", report.StoreSize)
	}
	if report.StoreSize+report.AssignedCount != report.DocCount {
		t.Fatalf("对账失败：StoreSize(%d) + AssignedCount(%d) ≠ DocCount(%d)",
			report.StoreSize, report.AssignedCount, report.DocCount)
	}
	if report.BackupDir == "" {
		t.Fatal("迁移必须生成备份目录")
	}
	if _, err := os.Stat(report.BackupDir); err != nil {
		t.Fatalf("备份目录不存在: %v", err)
	}
	if _, err := os.Stat(clusterIndexPath(d.baseDir)); err != nil {
		t.Fatalf("迁移后聚类索引不存在: %v", err)
	}

	// 重复迁移幂等
	first := d.cloneClusterIndex()
	secondReport, err := d.MigrateClusters()
	if err != nil {
		t.Fatalf("重复迁移失败: %v", err)
	}
	if secondReport.VectorCount != report.VectorCount {
		t.Fatalf("重复迁移向量数变化：%d → %d", report.VectorCount, secondReport.VectorCount)
	}
	second := d.cloneClusterIndex()
	if !reflect.DeepEqual(first.Assignments, second.Assignments) {
		t.Fatalf("迁移非幂等，归属发生漂移：\n第一次 %v\n第二次 %v", first.Assignments, second.Assignments)
	}
}

// TestClusterMigrationRollback 覆盖写入失败回滚：旧索引保留、内存索引不切换。
func TestClusterMigrationRollback(t *testing.T) {
	d := newClusterTestStore(t)
	// 3 条（< 16），迁移后 AssignedCount=0
	addTexts(t, d, "今天天气不错", "心情很开心", "写代码很顺利")

	before := d.cloneClusterIndex()
	clusterPersistHook = func(string) error { return errors.New("模拟写盘失败") }
	_, err := d.MigrateClusters()
	clusterPersistHook = nil
	if err == nil {
		t.Fatal("写盘失败时迁移应返回错误")
	}

	after := d.cloneClusterIndex()
	if !reflect.DeepEqual(before.Assignments, after.Assignments) {
		t.Fatalf("回滚后内存索引发生变化：\n之前 %v\n之后 %v", before.Assignments, after.Assignments)
	}
	var onDisk ClusterIndex
	if readErr := readJSONFile(clusterIndexPath(d.baseDir), &onDisk); readErr != nil {
		t.Fatalf("回滚后磁盘索引不可读: %v", readErr)
	}
	if len(onDisk.Assignments) != len(before.Assignments) {
		t.Fatalf("回滚后磁盘归属数 = %d，期望 %d", len(onDisk.Assignments), len(before.Assignments))
	}
}

// TestClusterMigrationWithoutEmbeddingService 覆盖嵌入服务不可用：
// 迁移只读取已持久化向量，不依赖嵌入服务。
func TestClusterMigrationWithoutEmbeddingService(t *testing.T) {
	d := newClusterTestStore(t)
	// 2 条不同主题 → 各 1 条，< 16，全留总库
	addTexts(t, d, "今天天气不错", "心情很开心")
	resetClusterIndex(t, d)

	failing := setEmbedTextsHook(func(ctx context.Context, model string, texts []string) ([][]float32, error) {
		return nil, errors.New("嵌入服务不可用")
	})
	defer failing()

	report, err := d.MigrateClusters()
	if err != nil {
		t.Fatalf("嵌入服务不可用时迁移仍应成功: %v", err)
	}
	if report.DocCount != 2 || report.VectorCount != 2 {
		t.Fatalf("条目/向量数异常: %+v", report)
	}
	if report.ClusterCount != 0 {
		t.Fatalf("2 条不应建簇，实得 %d", report.ClusterCount)
	}
	if report.StoreSize != 2 || report.AssignedCount != 0 {
		t.Fatalf("应全留总库（StoreSize=2, AssignedCount=0），实得 %+v", report)
	}
}

// TestClusterMigrationKeepsTopKStable 覆盖迁移前后抽样检索对比：
// 迁移不改动文档与向量，topK 命中顺序应完全一致。
func TestClusterMigrationKeepsTopKStable(t *testing.T) {
	d := newClusterTestStore(t)
	addTexts(t, d, "今天天气不错", "心情很开心", "写代码很顺利", "想吃饭了")
	ctx := context.Background()
	queries := []string{"天气", "心情", "代码"}

	sample := func() map[string][]string {
		out := make(map[string][]string, len(queries))
		for _, query := range queries {
			hits, err := d.Query(ctx, query, TypeText, 3, QueryOptions{ForceGlobal: true})
			if err != nil {
				t.Fatalf("抽样检索 %q 失败: %v", query, err)
			}
			ids := make([]string, 0, len(hits))
			for _, hit := range hits {
				ids = append(ids, hit.ID)
			}
			out[query] = ids
		}
		return out
	}

	before := sample()
	resetClusterIndex(t, d)
	if _, err := d.MigrateClusters(); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	after := sample()

	matched, total := 0, 0
	for _, query := range queries {
		total += len(before[query])
		if reflect.DeepEqual(before[query], after[query]) {
			matched += len(before[query])
		}
		t.Logf("抽样 %q：迁移前 %v，迁移后 %v", query, before[query], after[query])
	}
	t.Logf("抽样检索对比：%d 组查询，topK 命中一致 %d/%d 条", len(queries), matched, total)
	if matched != total {
		t.Fatalf("迁移前后检索命中不一致：%d/%d", matched, total)
	}
}
