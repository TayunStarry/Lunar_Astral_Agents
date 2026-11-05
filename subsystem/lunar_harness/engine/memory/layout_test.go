package memory

// layout_test.go 簇目录布局：迁移对账、幂等、失败回滚、分裂/合并搬迁、读写与删除一致性。

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// writeLegacyDir 按旧布局写一个语义分组目录：documents_0001.json + embeddings_0001.json + metadata.json。
func writeLegacyDir(t *testing.T, baseDir, name string, docs []Document) {
	t.Helper()
	dir := filepath.Join(baseDir, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("创建旧分组目录失败: %v", err)
	}
	records := make([]embeddingRecord, len(docs))
	for i := range docs {
		records[i] = embeddingRecord{ID: docs[i].ID, Embedding: docs[i].Embedding}
	}
	if err := atomicWriteJSON(filepath.Join(dir, "documents_0001.json"), docs); err != nil {
		t.Fatalf("写入旧分组文档失败: %v", err)
	}
	if err := atomicWriteJSON(filepath.Join(dir, "embeddings_0001.json"), records); err != nil {
		t.Fatalf("写入旧分组向量失败: %v", err)
	}
	meta := collectionMeta{
		EmbeddingDimension:   len(docs[0].Embedding),
		Version:              CurrentVersion,
		DocumentsChunkCount:  1,
		EmbeddingsChunkCount: 1,
	}
	if err := atomicWriteJSON(filepath.Join(dir, "metadata.json"), meta); err != nil {
		t.Fatalf("写入旧分组元数据失败: %v", err)
	}
}

// writeIndexFile 写入手工构造的 clusters.json（用于模拟分裂/合并后的归属变化）。
func writeIndexFile(t *testing.T, baseDir string, nodes []ClusterNode) {
	t.Helper()
	index := &ClusterIndex{
		Version:     ClusterIndexVersion,
		Threshold:   ClusterAdmitThreshold,
		Dimension:   len(nodes[0].Centroid),
		Clusters:    nodes,
		Assignments: make(map[string]string),
	}
	for _, node := range nodes {
		for _, id := range node.DocIDs {
			index.Assignments[id] = node.ID
		}
	}
	if err := atomicWriteJSON(clusterIndexPath(baseDir), index); err != nil {
		t.Fatalf("写入 clusters.json 失败: %v", err)
	}
}

// clusterNode 构造索引节点（质心归一化，规模与成员保持一致）。
func clusterNode(id string, centroid []float32, ids ...string) ClusterNode {
	return ClusterNode{ID: id, Centroid: normalizeVector(centroid), DocIDs: ids, Size: len(ids)}
}

// layoutDocs 迁移用例的固定数据集：两条同簇天气 + 一条代码。
func layoutDocs() []Document {
	return []Document{
		{ID: "doc-weather-1", Type: TypeText, Role: "user", Content: "今天天气不错", Timestamp: 100, Embedding: []float32{1, 0, 0, 0}},
		{ID: "doc-weather-2", Type: TypeText, Role: "user", Content: "明天天气很好", Timestamp: 200, Embedding: []float32{0.98, 0, 0, 0}},
		{ID: "doc-code-1", Type: TypeText, Role: "user", Content: "写代码很顺利", Timestamp: 300, Embedding: []float32{0, 0, 1, 0}},
	}
}

func listClusterDirNames(t *testing.T, baseDir string) []string {
	t.Helper()
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		t.Fatalf("读取记忆库目录失败: %v", err)
	}
	names := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() && isClusterDirName(entry.Name()) {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names
}

func dirDocIDs(t *testing.T, dir string) []string {
	t.Helper()
	docs, _, _, err := readDocumentsFromDir(dir)
	if err != nil {
		t.Fatalf("读取目录 %s 失败: %v", dir, err)
	}
	ids := make([]string, 0, len(docs))
	for _, doc := range docs {
		ids = append(ids, doc.ID)
	}
	sort.Strings(ids)
	return ids
}

func assertDirDocIDs(t *testing.T, dir string, want ...string) {
	t.Helper()
	got := dirDocIDs(t, dir)
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("目录 %s 条目 %v ≠ 期望 %v", dir, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("目录 %s 条目 %v ≠ 期望 %v", dir, got, want)
		}
	}
}

// TestLayoutMigrationFromLegacyGroups 旧分组目录应迁移为"总库为主"的新布局：
// 少量条目（< ClusterUpgradeSize=16）全进总库，不建正式簇目录。
func TestLayoutMigrationFromLegacyGroups(t *testing.T) {
	base := t.TempDir()
	docs := layoutDocs()
	writeLegacyDir(t, base, "日常闲聊", docs[:2])
	writeLegacyDir(t, base, "工作", docs[2:])

	d := newStore(base)
	report, err := d.prepareLayout(false)
	if err != nil {
		t.Fatalf("迁移失败: %v", err)
	}

	// 对账数据：3 条全部有向量，< 16，全进总库，0 个正式簇
	if report.DocCount != 3 || report.VectorCount != 3 {
		t.Fatalf("迁移条目/向量数不正确: %+v", report)
	}
	if report.ClusterCount != 0 {
		t.Fatalf("3 条不足以建簇，实得 %d", report.ClusterCount)
	}
	if report.AssignedCount != 0 {
		t.Fatalf("不应有归属条目，实得 %d", report.AssignedCount)
	}
	if report.StoreSize != 3 {
		t.Fatalf("总库应有 3 条，实得 %d", report.StoreSize)
	}
	if report.LegacyDirs != 2 {
		t.Fatalf("应删除 2 个旧目录，实得 %d", report.LegacyDirs)
	}
	if report.BackupDir == "" {
		t.Fatal("迁移前应生成备份目录")
	}

	// 旧分组目录只有在全部成功后才会删除
	for _, name := range []string{"日常闲聊", "工作"} {
		if _, err := os.Stat(filepath.Join(base, name)); !os.IsNotExist(err) {
			t.Fatalf("旧分组目录 %s 未删除: %v", name, err)
		}
	}

	// 3 条都 < 16，全进 store-all
	storeDir := filepath.Join(base, StoreCollectionID)
	assertDirDocIDs(t, storeDir, "doc-weather-1", "doc-weather-2", "doc-code-1")
	if names := listClusterDirNames(t, base); len(names) != 0 {
		t.Fatalf("迁移后不应有正式簇目录，实得 %v", names)
	}

	// 备份清单记录旧目录与文件数
	manifest := ClusterBackupManifest{}
	if err := readJSONFile(filepath.Join(report.BackupDir, ClusterLayoutManifest), &manifest); err != nil {
		t.Fatalf("读取备份清单失败: %v", err)
	}
	if manifest.DocCount != 3 || manifest.VectorCount != 3 || manifest.HadClusterIndex {
		t.Fatalf("备份清单规模不正确: %+v", manifest)
	}
	for _, name := range []string{"日常闲聊", "工作"} {
		if _, ok := manifest.Dirs[name]; !ok {
			t.Fatalf("备份清单缺少旧目录 %s: %+v", name, manifest.Dirs)
		}
	}

	// 全部条目可读
	entries, total := d.List("", 0, 0)
	if total != 3 || len(entries) != 3 {
		t.Fatalf("迁移后应可读回 3 条记忆，实得 total=%d len=%d", total, len(entries))
	}
	for _, entry := range entries {
		if entry.Collection != StoreCollectionID {
			t.Fatalf("条目 %s 应在总库 store-all，实得 %s", entry.ID, entry.Collection)
		}
	}
}

// TestLayoutMigrationIdempotent 重复执行迁移不重复搬迁、不再生成备份。
func TestLayoutMigrationIdempotent(t *testing.T) {
	base := t.TempDir()
	docs := layoutDocs()
	writeLegacyDir(t, base, "日常闲聊", docs[:2])
	writeLegacyDir(t, base, "工作", docs[2:])

	d := newStore(base)
	first, err := d.prepareLayout(false)
	if err != nil {
		t.Fatalf("首次迁移失败: %v", err)
	}
	if first.LegacyDirs != 2 {
		t.Fatalf("首次迁移应删除 2 个旧目录: %+v", first)
	}

	backups, err := os.ReadDir(filepath.Join(base, ClusterMigrateBackupDir))
	if err != nil {
		t.Fatalf("读取备份目录失败: %v", err)
	}
	backupCount := len(backups)

	second, err := d.prepareLayout(false)
	if err != nil {
		t.Fatalf("重复迁移失败: %v", err)
	}
	if !second.ReusedIndex || second.MovedDocs != 0 || second.BackupDir != "" {
		t.Fatalf("布局已一致时不应再次搬迁或备份: %+v", second)
	}
	again, err := os.ReadDir(filepath.Join(base, ClusterMigrateBackupDir))
	if err != nil {
		t.Fatalf("读取备份目录失败: %v", err)
	}
	if len(again) != backupCount {
		t.Fatalf("重复迁移新增了备份: %d → %d", backupCount, len(again))
	}
	// 3 条 < 16，全进总库 store-all，无正式簇目录
	if names := listClusterDirNames(t, base); len(names) != 0 {
		t.Fatalf("重复迁移后仍不应有正式簇目录: %v", names)
	}
}

// TestLayoutMigrationRollbackKeepsLegacyDirs 索引写入失败时回滚：旧目录保留、新目录清理。
func TestLayoutMigrationRollbackKeepsLegacyDirs(t *testing.T) {
	base := t.TempDir()
	docs := layoutDocs()
	writeLegacyDir(t, base, "日常闲聊", docs[:2])
	writeLegacyDir(t, base, "工作", docs[2:])

	d := newStore(base)
	original := clusterPersistHook
	clusterPersistHook = func(path string) error {
		if filepath.Base(path) == ClusterIndexFile {
			return fmt.Errorf("模拟 clusters.json 写入失败")
		}
		return nil
	}
	defer func() { clusterPersistHook = original }()

	if _, err := d.prepareLayout(false); err == nil {
		t.Fatal("索引写入失败时应返回错误")
	}

	// 旧分组目录必须原样保留
	for _, name := range []string{"日常闲聊", "工作"} {
		dir := filepath.Join(base, name)
		if _, err := os.Stat(filepath.Join(dir, "documents_0001.json")); err != nil {
			t.Fatalf("回滚后旧分组目录 %s 丢失: %v", name, err)
		}
	}
	// 本次新建的簇目录与索引都应被清理
	if names := listClusterDirNames(t, base); len(names) != 0 {
		t.Fatalf("回滚后不应残留新建簇目录: %v", names)
	}
	if _, err := os.Stat(clusterIndexPath(base)); !os.IsNotExist(err) {
		t.Fatalf("回滚后不应保留新 clusters.json: %v", err)
	}

	// 失败恢复后重试可以成功完成迁移
	clusterPersistHook = original
	report, err := d.prepareLayout(false)
	if err != nil {
		t.Fatalf("重试迁移失败: %v", err)
	}
	if report.LegacyDirs != 2 || report.DocCount != 3 {
		t.Fatalf("重试迁移对账不正确: %+v", report)
	}
	for _, name := range []string{"日常闲聊", "工作"} {
		if _, err := os.Stat(filepath.Join(base, name)); !os.IsNotExist(err) {
			t.Fatalf("重试成功后旧目录 %s 未删除: %v", name, err)
		}
	}
}

// TestInvalidClusterIndexTriggersRebuild 手写 clusters.json 含 size < ClusterUpgradeSize 的小簇时，
// layoutMatchesIndex 拒绝复用，materializeLayout 强制重建，所有条目进 store-all。
func TestInvalidClusterIndexTriggersRebuild(t *testing.T) {
	base := t.TempDir()
	docs := layoutDocs()
	writeLegacyDir(t, base, "旧分组", docs)

	// 手写 clusters.json：两个小簇（各 1-2 条），都 < ClusterUpgradeSize
	writeIndexFile(t, base, []ClusterNode{
		clusterNode("cluster-weather", []float32{1, 0, 0, 0}, "doc-weather-1", "doc-weather-2"),
		clusterNode("cluster-code", []float32{0, 0, 1, 0}, "doc-code-1"),
	})
	d := newStore(base)
	report, err := d.prepareLayout(false)
	if err != nil {
		t.Fatalf("迁移失败: %v", err)
	}

	// 小簇被强制重建 → 0 个正式簇，全部进 store-all
	if report.ClusterCount != 0 {
		t.Fatalf("小簇应被强制重建为总库，实得 %d 个正式簇", report.ClusterCount)
	}
	if report.AssignedCount != 0 {
		t.Fatalf("不应有归属条目，实得 %d", report.AssignedCount)
	}
	if report.StoreSize != 3 {
		t.Fatalf("StoreSize 应为 3，实得 %d", report.StoreSize)
	}
	if report.LegacyDirs != 1 {
		t.Fatalf("应删除 1 个旧目录，实得 %d", report.LegacyDirs)
	}

	// 旧目录已删除
	if _, err := os.Stat(filepath.Join(base, "旧分组")); !os.IsNotExist(err) {
		t.Fatalf("旧目录未删除: %v", err)
	}
	// store-all 有全部 3 条
	storeDir := filepath.Join(base, StoreCollectionID)
	assertDirDocIDs(t, storeDir, "doc-weather-1", "doc-weather-2", "doc-code-1")
	// 没有 c-* 正式簇目录
	if names := listClusterDirNames(t, base); len(names) != 0 {
		t.Fatalf("不应有正式簇目录，实得 %v", names)
	}

	// 索引里 Assignments 为空（所有条目隐含在 StoreSize 里）
	idx := readIndexForTest(t, base)
	if len(idx.Clusters) != 0 || len(idx.Assignments) != 0 {
		t.Fatalf("重建后簇/归属应为空，实得 Clusters=%d Assignments=%d",
			len(idx.Clusters), len(idx.Assignments))
	}
	if idx.StoreSize != 3 {
		t.Fatalf("StoreSize 应为 3，实得 %d", idx.StoreSize)
	}
}

// TestClusterDirReadWriteAndDeleteConsistency 写入/更新/删除/ReplaceTexts 全部按簇目录定位，
// 删除后目录与 clusters.json 保持一致，空簇不保留空目录。
func TestClusterDirReadWriteAndDeleteConsistency(t *testing.T) {
	vectors := map[string][]float32{
		"今天天气不错":   {1, 0, 0, 0},
		"明天天气很好":   {0.9, 0.3, 0, 0},
		"写代码很顺利":   {0, 0, 1, 0},
		"后天天气更好":   {1, 0, 0, 0},
		"整理后的代码记录": {0, 0, 1, 0},
	}
	restore := setEmbedTextsHook(func(_ context.Context, _ string, texts []string) ([][]float32, error) {
		out := make([][]float32, len(texts))
		for i, text := range texts {
			if vec, ok := vectors[text]; ok {
				out[i] = vec
				continue
			}
			out[i] = []float32{0, 0, 0, 1}
		}
		return out, nil
	})
	defer restore()

	ctx := context.Background()
	d := newStore(t.TempDir())
	if err := d.EnsureReady(); err != nil {
		t.Fatalf("初始化失败: %v", err)
	}

	weather1, err := d.AddText(ctx, "user", "今天天气不错")
	if err != nil {
		t.Fatalf("写入第一条天气记忆失败: %v", err)
	}
	weather2, err := d.AddText(ctx, "user", "明天天气很好")
	if err != nil {
		t.Fatalf("写入第二条天气记忆失败: %v", err)
	}
	first, _ := d.Get(weather1)
	second, _ := d.Get(weather2)
	// 新语义：少量条目（< 16）统一进总库 store-all
	if first.Collection != StoreCollectionID || second.Collection != StoreCollectionID {
		t.Fatalf("少量条目应进总库 store-all，实得 first=%s second=%s", first.Collection, second.Collection)
	}
	storeDir := filepath.Join(d.baseDir, clusterDirName(StoreCollectionID))
	assertDirDocIDs(t, storeDir, weather1, weather2)

	code, err := d.AddText(ctx, "user", "写代码很顺利")
	if err != nil {
		t.Fatalf("写入代码记忆失败: %v", err)
	}
	codeEntry, _ := d.Get(code)
	if codeEntry.Collection != StoreCollectionID {
		t.Fatalf("不同语义条目同样应进 store-all，实得 %s", codeEntry.Collection)
	}
	assertDirDocIDs(t, storeDir, weather1, weather2, code)

	// 更新：正文与向量一起更新，仍落在同一簇目录（不存在“写新目录、读旧目录”）
	if err := d.Update(ctx, weather1, "后天天气更好"); err != nil {
		t.Fatalf("更新天气记忆失败: %v", err)
	}
	updated, _ := d.Get(weather1)
	if updated.Content != "后天天气更好" || updated.Collection != StoreCollectionID {
		t.Fatalf("更新后归属/正文不正确: %+v", updated)
	}
	assertDirDocIDs(t, storeDir, weather1, weather2, code)

	// 删除其中一条：store-all 条目数减少，索引 StoreSize 同步减少
	if err := d.Delete(ctx, weather2); err != nil {
		t.Fatalf("删除天气记忆失败: %v", err)
	}
	assertDirDocIDs(t, storeDir, weather1, code)
	idx := readIndexForTest(t, d.baseDir)
	if len(idx.Assignments) != 0 {
		t.Fatalf("少量条目不应有归属，实得 %d", len(idx.Assignments))
	}
	if idx.StoreSize != 2 {
		t.Fatalf("StoreSize 应为 2，实得 %d", idx.StoreSize)
	}

	// 删除最后一条天气：store-all 只剩 code
	if err := d.Delete(ctx, weather1); err != nil {
		t.Fatalf("删除最后一条天气记忆失败: %v", err)
	}
	assertDirDocIDs(t, storeDir, code)
	idx = readIndexForTest(t, d.baseDir)
	if idx.StoreSize != 1 {
		t.Fatalf("StoreSize 应为 1，实得 %d", idx.StoreSize)
	}

	// ReplaceTexts：在 store-all 里先写新、再删旧
	result, err := d.ReplaceTexts(ctx, []string{code}, []string{"整理后的代码记录"}, ReplaceOptions{})
	if err != nil {
		t.Fatalf("批量替换失败: %v", err)
	}
	if len(result.CreatedIDs) != 1 || len(result.DeletedIDs) != 1 {
		t.Fatalf("替换结果不正确: %+v", result)
	}
	if _, err := d.Get(code); err == nil {
		t.Fatal("替换后旧条目仍可读取")
	}
	newEntry, err := d.Get(result.CreatedIDs[0])
	if err != nil {
		t.Fatalf("替换后新条目不可读: %v", err)
	}
	if newEntry.Collection != StoreCollectionID {
		t.Fatalf("替换后新条目应仍在 store-all，实得 %s", newEntry.Collection)
	}
	// store-all 现在只有新条目
	assertDirDocIDs(t, storeDir, newEntry.ID)
}

// readIndexForTest 读取磁盘 clusters.json。
func readIndexForTest(t *testing.T, baseDir string) *ClusterIndex {
	t.Helper()
	var index ClusterIndex
	if err := readJSONFile(clusterIndexPath(baseDir), &index); err != nil {
		t.Fatalf("读取 clusters.json 失败: %v", err)
	}
	return &index
}
