package memory

// keyword_index_bench_test.go 提供倒排索引召回 vs 全库线性扫描的合成数据对比：
//   - 比较条目数（索引=扫描倒排记录数，全扫=比较文档数）；
//   - 单次查询耗时（基准测试）；
//   - topK 一致性（稀有精确串查询下两路首位/集合一致）。
//
// 运行基准：go test -run '^$' -bench '^BenchmarkKeyword' -benchmem ./engine/memory/

import (
	"fmt"
	"sort"
	"testing"
)

func keywordBenchDocs(n int) []Document {
	docs := make([]Document, 0, n)
	for i := 0; i < n; i++ {
		content := fmt.Sprintf("第%d条记忆 记录 通用说明 唯一编号 token%d 路径 /data/topic%04d/file.go",
			i, i, i%1000)
		docs = append(docs, Document{
			ID:        fmt.Sprintf("doc-%05d", i),
			Type:      TypeText,
			Content:   content,
			Timestamp: int64(i),
			Embedding: []float32{1, float32(i % 5)},
		})
	}
	return docs
}

func keywordBenchSources(n int) []keywordIndexSource {
	docs := keywordBenchDocs(n)
	sources := make([]keywordIndexSource, 0, n)
	for _, doc := range docs {
		sources = append(sources, keywordIndexSource{Doc: doc, Collection: StoreCollectionID})
	}
	return sources
}

func buildKeywordBenchStore(n int) *Store {
	d := scopeStore(map[string][]Document{"cluster-a": keywordBenchDocs(n)})
	// 建立 ID→下标映射，使召回后定位正文为 O(1)（与真实落盘集合一致）
	for _, c := range d.snapshotCollections() {
		c.mu.Lock()
		c.rebuildPositionsLocked()
		c.mu.Unlock()
	}
	d.RebuildKeywordIndex()
	return d
}

// keywordFullScanBaseline 复刻改造前的关键词通道：全库线性扫描 + 字面命中打分。
// 返回 topK ID 与比较文档数。
func keywordFullScanBaseline(d *Store, queries []string, mediaType string, limit int) ([]string, int) {
	score, err := hybridKeywordScorer(queries, false)
	if err != nil || score == nil {
		return nil, 0
	}
	compared := 0
	merged := make(map[string]Entry)
	for _, c := range d.snapshotCollections() {
		c.mu.RLock()
		for i := range c.Documents {
			doc := &c.Documents[i]
			if documentType(*doc) != mediaType || doc.Pending {
				continue
			}
			compared++
			s := score(doc.Content)
			if s <= 0 {
				continue
			}
			if existing, ok := merged[doc.ID]; ok && existing.KeywordScore >= s {
				continue
			}
			entry := resultOf(c, doc, 0)
			entry.KeywordScore = s
			merged[doc.ID] = entry
		}
		c.mu.RUnlock()
	}
	out := make([]Entry, 0, len(merged))
	for _, item := range merged {
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].KeywordScore != out[j].KeywordScore {
			return out[i].KeywordScore > out[j].KeywordScore
		}
		if out[i].Timestamp != out[j].Timestamp {
			return out[i].Timestamp > out[j].Timestamp
		}
		return out[i].ID < out[j].ID
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	ids := make([]string, 0, len(out))
	for _, item := range out {
		ids = append(ids, item.ID)
	}
	return ids, compared
}

// TestKeywordIndexCompareCountsAndTopK 10k 合成数据：
// 稀有精确串查询下索引只扫描 1 条倒排记录，而全扫比较 10000 篇文档；topK 一致。
func TestKeywordIndexCompareCountsAndTopK(t *testing.T) {
	const n = 10000
	d := buildKeywordBenchStore(n)
	queries := []string{"token9000"}

	indexIDs, stats := d.keywordIndexRecall(queries, TypeText, 5)
	fullIDs, compared := keywordFullScanBaseline(d, queries, TypeText, 5)

	if stats.PostingsScanned != 1 {
		t.Fatalf("稀有词应只扫描 1 条倒排记录，实得 %d", stats.PostingsScanned)
	}
	if compared != n {
		t.Fatalf("全扫应比较全部 %d 篇，实得 %d", n, compared)
	}
	if len(indexIDs) != 1 || indexIDs[0].ID != "doc-09000" {
		t.Fatalf("索引召回 top1 应为 doc-09000，实得 %+v", indexIDs)
	}
	if len(fullIDs) != 1 || fullIDs[0] != "doc-09000" {
		t.Fatalf("全扫 top1 应为 doc-09000，实得 %+v", fullIDs)
	}

	idxStats := d.KeywordIndexStats()
	t.Logf("10k: 索引比较 %d 条倒排 / 全扫比较 %d 篇；索引词表 %d、倒排 %d、估算内存 %d B、落盘 %d B",
		stats.PostingsScanned, compared, idxStats.Tokens, idxStats.Postings, idxStats.EstimatedBytes, idxStats.FileBytes)
	// 100k 量级估算：全扫比较条目随文档数线性增长，稀有精确串倒排记录数不变
	t.Logf("100k 估算：全扫约比较 %d 篇；索引约扫描 %d 条倒排（按当前词表密度，倒排总量约 %d）",
		compared*10, stats.PostingsScanned, idxStats.Postings*10)
}

// TestKeywordIndexTopKConsistencyExact 同一精确串命中 5 条时，倒排召回与全扫 topK 完全一致。
func TestKeywordIndexTopKConsistencyExact(t *testing.T) {
	docs := make([]Document, 0, 30)
	for i := 0; i < 5; i++ {
		docs = append(docs, Document{
			ID:        fmt.Sprintf("hit-%02d", i),
			Type:      TypeText,
			Content:   fmt.Sprintf("共享标记 REQ_9000_ALPHA 第%d条", i),
			Timestamp: int64(100 + i),
		})
	}
	for i := 5; i < 30; i++ {
		docs = append(docs, Document{
			ID:        fmt.Sprintf("miss-%02d", i),
			Type:      TypeText,
			Content:   fmt.Sprintf("普通记录 第%d条", i),
			Timestamp: int64(200 + i),
		})
	}
	d := scopeStore(map[string][]Document{"cluster-a": docs})
	for _, c := range d.snapshotCollections() {
		c.mu.Lock()
		c.rebuildPositionsLocked()
		c.mu.Unlock()
	}
	d.RebuildKeywordIndex()

	indexHits, _ := d.keywordIndexRecall([]string{"REQ_9000_ALPHA"}, TypeText, 5)
	indexIDs := make([]string, 0, len(indexHits))
	for _, hit := range indexHits {
		indexIDs = append(indexIDs, hit.ID)
	}
	fullIDs, _ := keywordFullScanBaseline(d, []string{"REQ_9000_ALPHA"}, TypeText, 5)
	if len(indexIDs) != len(fullIDs) {
		t.Fatalf("topK 数量不一致：index=%v full=%v", indexIDs, fullIDs)
	}
	for i := range indexIDs {
		if indexIDs[i] != fullIDs[i] {
			t.Fatalf("topK 顺序不一致：index=%v full=%v", indexIDs, fullIDs)
		}
	}
}

// TestKeywordIndexFileSize10k 落盘体积实测（10k 条、gzip 倒排快照）。
func TestKeywordIndexFileSize10k(t *testing.T) {
	const n = 10000
	d := newStore(t.TempDir())
	if err := d.EnsureReady(); err != nil {
		t.Fatalf("初始化失败: %v", err)
	}
	c, err := d.storeCollection()
	if err != nil || c == nil {
		t.Fatalf("总库集合缺失: %v", err)
	}
	// 直接批量写入内存集合（逐条 append 会触发每篇全量重写分片，不适合构造测试数据）
	c.mu.Lock()
	c.Documents = append(c.Documents, keywordBenchDocs(n)...)
	c.rebuildPositionsLocked()
	c.mu.Unlock()
	if err := d.RebuildKeywordIndex(); err != nil {
		t.Fatalf("重建索引失败: %v", err)
	}
	stats := d.KeywordIndexStats()
	t.Logf("10k 落盘：%d B（docs=%d tokens=%d postings=%d），内存估算 %d B",
		stats.FileBytes, stats.Docs, stats.Tokens, stats.Postings, stats.EstimatedBytes)
	if stats.FileBytes <= 0 {
		t.Fatalf("索引文件应已落盘")
	}
}

// BenchmarkKeywordRecallIndex10k 倒排索引召回（含 Entry 构建）。
func BenchmarkKeywordRecallIndex10k(b *testing.B) {
	d := buildKeywordBenchStore(10000)
	queries := []string{"token9000"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, stats := d.keywordIndexRecall(queries, TypeText, 5)
		b.ReportMetric(float64(stats.PostingsScanned), "postings/op")
	}
}

// BenchmarkKeywordFullScan10k 改造前的全库线性扫描口径。
func BenchmarkKeywordFullScan10k(b *testing.B) {
	d := buildKeywordBenchStore(10000)
	queries := []string{"token9000"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, compared := keywordFullScanBaseline(d, queries, TypeText, 5)
		b.ReportMetric(float64(compared), "docs/op")
	}
}

// BenchmarkKeywordRecallIndex100k 10 万量级实测。
func BenchmarkKeywordRecallIndex100k(b *testing.B) {
	d := buildKeywordBenchStore(100000)
	queries := []string{"token90000"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, stats := d.keywordIndexRecall(queries, TypeText, 5)
		b.ReportMetric(float64(stats.PostingsScanned), "postings/op")
	}
}

// BenchmarkKeywordFullScan100k 10 万量级全扫实测。
func BenchmarkKeywordFullScan100k(b *testing.B) {
	d := buildKeywordBenchStore(100000)
	queries := []string{"token90000"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, compared := keywordFullScanBaseline(d, queries, TypeText, 5)
		b.ReportMetric(float64(compared), "docs/op")
	}
}

// BenchmarkKeywordIndexBuild10k 启动/重建成本：仅分词 + 构建内存倒排（不含读盘）。
func BenchmarkKeywordIndexBuild10k(b *testing.B) {
	sources := keywordBenchSources(10000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx := buildKeywordIndexFromSources(sources)
		if idx == nil {
			b.Fatal("索引构建失败")
		}
	}
}
