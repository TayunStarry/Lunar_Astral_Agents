package memory

// keyword_index_test.go 覆盖关键词倒排索引的关键行为：
//  1. 中文双字切分与精确串（昵称/路径/错误码/编号）命中；
//  2. 覆盖率打分：词频不放大、常见词降权、精确串优先；
//  3. 写入/更新/删除/簇搬迁与索引同步；
//  4. 旧库首次启动建索引、索引损坏后幂等重建、持久化复用；
//  5. 关键词独立召回（向量不可用）与共享 topK 裁剪。

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// keywordWriteServeHook 注入确定性的文档嵌入：不同标记映射到不同基向量，避免判重。
func keywordWriteServeHook() func() {
	return setEmbedTextsHook(func(_ context.Context, _ string, texts []string) ([][]float32, error) {
		out := make([][]float32, len(texts))
		for i, text := range texts {
			vec := []float32{0, 0, 0, 1}
			switch {
			case strings.Contains(text, "alpha"):
				vec = []float32{1, 0, 0, 0}
			case strings.Contains(text, "beta"):
				vec = []float32{0, 1, 0, 0}
			case strings.Contains(text, "gamma"):
				vec = []float32{0, 0, 1, 0}
			}
			out[i] = vec
		}
		return out, nil
	})
}

func keywordTestRecallIDs(t *testing.T, d *Store, queries []string, limit int) []string {
	t.Helper()
	hits, stats := d.keywordIndexRecall(queries, TypeText, limit)
	if stats.QueryTokens == 0 && len(hits) != 0 {
		t.Fatalf("无查询词元却返回了结果: %+v", hits)
	}
	ids := make([]string, 0, len(hits))
	for _, hit := range hits {
		ids = append(ids, hit.ID)
	}
	return ids
}

// TestKeywordTokenizeChineseAndExact 验证中文双字切分与精确串整体切分。
func TestKeywordTokenizeChineseAndExact(t *testing.T) {
	terms := keywordTokenize("目标记录：路径 /usr/local/bin/lunar 错误 ERR_CONN_RESET 编号 20241105")
	for _, want := range []string{"目标", "标记", "记录", "/usr/local/bin/lunar", "err_conn_reset", "20241105"} {
		if terms[want] == 0 {
			t.Fatalf("分词缺少词元 %q：%#v", want, terms)
		}
	}
	if !keywordTokenExact("/usr/local/bin/lunar") || !keywordTokenExact("err_conn_reset") {
		t.Fatalf("路径/错误码应判定为精确串")
	}
	if keywordTokenExact("记录") {
		t.Fatalf("中文双字词不应判定为精确串")
	}
}

// TestKeywordRecallExactStrings 覆盖昵称、路径、错误码、编号的独立命中。
func TestKeywordRecallExactStrings(t *testing.T) {
	docs := map[string][]Document{
		"cluster-a": {
			{ID: "nick", Type: TypeText, Content: "用户昵称 NightOwl 正在调试", Timestamp: 100},
			{ID: "path", Type: TypeText, Content: "安装路径 /usr/local/bin/lunar --verbose", Timestamp: 200},
			{ID: "err", Type: TypeText, Content: "启动失败 ERR_CONN_RESET 0x80070005", Timestamp: 300},
			{ID: "code", Type: TypeText, Content: "工单编号 20241105 待处理", Timestamp: 400},
			{ID: "other", Type: TypeText, Content: "完全无关的正文", Timestamp: 500},
		},
	}
	d := scopeStore(docs)

	cases := []struct {
		query string
		want  string
	}{
		{"NightOwl", "nick"},
		{"/usr/local/bin/lunar", "path"},
		{"ERR_CONN_RESET", "err"},
		{"0x80070005", "err"},
		{"20241105", "code"},
	}
	for _, tc := range cases {
		ids := keywordTestRecallIDs(t, d, []string{tc.query}, 5)
		if len(ids) == 0 || ids[0] != tc.want {
			t.Fatalf("查询 %q 首位应为 %s，实得 %v", tc.query, tc.want, ids)
		}
	}
}

// TestKeywordCoverageIgnoresTermFrequencyAndDownweightsCommon 覆盖覆盖率打分：
// 词频不参与打分；常见词相对精确串被显著降权。
func TestKeywordCoverageIgnoresTermFrequencyAndDownweightsCommon(t *testing.T) {
	docs := make([]Document, 0, 23)
	docs = append(docs, Document{ID: "common-once", Type: TypeText, Content: "记录 第一条", Timestamp: 100})
	docs = append(docs, Document{ID: "common-many", Type: TypeText, Content: "记录记录记录记录记录 第二条", Timestamp: 200})
	for i := 0; i < 20; i++ {
		docs = append(docs, Document{
			ID:        fmt.Sprintf("common-%02d", i),
			Type:      TypeText,
			Content:   fmt.Sprintf("记录 背景条目 %d", i),
			Timestamp: int64(300 + i),
		})
	}
	docs = append(docs, Document{ID: "rare", Type: TypeText, Content: "稀有标记 X9000 记录", Timestamp: 1000})
	d := scopeStore(map[string][]Document{"cluster-a": docs})

	hits, _ := d.keywordIndexRecall([]string{"记录", "X9000"}, TypeText, 100)
	if len(hits) < 2 || hits[0].ID != "rare" {
		t.Fatalf("精确串条目应排在首位，实得 %v", hitIDs(hits))
	}
	var onceScore, manyScore, commonScore float32
	for _, hit := range hits {
		switch hit.ID {
		case "common-once":
			onceScore = hit.KeywordScore
		case "common-many":
			manyScore = hit.KeywordScore
		default:
			if strings.HasPrefix(hit.ID, "common-") {
				commonScore = hit.KeywordScore
			}
		}
	}
	if onceScore != manyScore {
		t.Fatalf("词频不应影响覆盖率得分：一次 %.3f，多次 %.3f", onceScore, manyScore)
	}
	if hits[0].KeywordScore <= commonScore*3 {
		t.Fatalf("精确串应显著高于常见词：rare=%.3f common=%.3f", hits[0].KeywordScore, commonScore)
	}
}

// TestKeywordIndexSyncsWithWriteUpdateDelete 覆盖写路径同步维护。
func TestKeywordIndexSyncsWithWriteUpdateDelete(t *testing.T) {
	restore := keywordWriteServeHook()
	defer restore()

	d := newStore(t.TempDir())
	if err := d.EnsureReady(); err != nil {
		t.Fatalf("初始化失败: %v", err)
	}
	ctx := context.Background()

	id, err := d.AddText(ctx, "user", "alpha 目标标记 AlphaTag")
	if err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	if ids := keywordTestRecallIDs(t, d, []string{"AlphaTag"}, 5); len(ids) != 1 || ids[0] != id {
		t.Fatalf("新增后索引应命中，实得 %v", ids)
	}

	if err := d.Update(ctx, id, "beta 更新标记 BetaTag"); err != nil {
		t.Fatalf("更新失败: %v", err)
	}
	if ids := keywordTestRecallIDs(t, d, []string{"AlphaTag"}, 5); len(ids) != 0 {
		t.Fatalf("更新后旧词元不应召回，实得 %v", ids)
	}
	if ids := keywordTestRecallIDs(t, d, []string{"BetaTag"}, 5); len(ids) != 1 || ids[0] != id {
		t.Fatalf("更新后新词元应命中，实得 %v", ids)
	}

	if err := d.Delete(ctx, id); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	if ids := keywordTestRecallIDs(t, d, []string{"BetaTag"}, 5); len(ids) != 0 {
		t.Fatalf("删除后索引不应召回，实得 %v", ids)
	}
}

// TestKeywordIndexSyncsWithClusterRelocation 覆盖簇搬迁：正文不变、集合归属刷新、仍可召回。
func TestKeywordIndexSyncsWithClusterRelocation(t *testing.T) {
	d := newStore(t.TempDir())
	if err := d.EnsureReady(); err != nil {
		t.Fatalf("初始化失败: %v", err)
	}
	cluster, err := d.clusterForCollection("cluster-move")
	if err != nil {
		t.Fatalf("创建簇失败: %v", err)
	}
	doc := Document{ID: "move-1", Type: TypeText, Content: "搬迁标记 MoveTag /tmp/move.bin", Timestamp: 100}
	if err := cluster.appendDocument(doc); err != nil {
		t.Fatalf("写入簇失败: %v", err)
	}
	if err := cluster.saveDocumentsToFile(); err != nil {
		t.Fatalf("保存簇失败: %v", err)
	}
	if err := d.RebuildKeywordIndex(); err != nil {
		t.Fatalf("重建索引失败: %v", err)
	}
	if ids := keywordTestRecallIDs(t, d, []string{"MoveTag"}, 5); len(ids) != 1 || ids[0] != doc.ID {
		t.Fatalf("搬迁前应命中，实得 %v", ids)
	}

	if err := d.moveClusterDocsToStore("cluster-move"); err != nil {
		t.Fatalf("搬迁失败: %v", err)
	}
	hits, _ := d.keywordIndexRecall([]string{"MoveTag"}, TypeText, 5)
	if len(hits) != 1 || hits[0].ID != doc.ID {
		t.Fatalf("搬迁后应仍命中，实得 %+v", hits)
	}
	if hits[0].Collection != StoreCollectionID {
		t.Fatalf("搬迁后集合归属应刷新为 %s，实得 %s", StoreCollectionID, hits[0].Collection)
	}
}

// TestKeywordIndexPersistenceLegacyAndCorruption 覆盖旧库首次建索引、持久化复用与损坏重建。
func TestKeywordIndexPersistenceLegacyAndCorruption(t *testing.T) {
	restore := keywordWriteServeHook()
	defer restore()

	dir := t.TempDir()
	d := newStore(dir)
	if err := d.EnsureReady(); err != nil {
		t.Fatalf("初始化失败: %v", err)
	}
	id, err := d.AddText(context.Background(), "user", "alpha 持久化标记 PersistTag")
	if err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	path := filepath.Join(dir, KeywordIndexFile)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("写入后索引文件应存在: %v", err)
	}

	// 模拟旧库：删除索引文件（documents/embeddings 分片保留），首次启动应自动建索引
	if err := os.Remove(path); err != nil {
		t.Fatalf("删除索引文件失败: %v", err)
	}
	d2 := newStore(dir)
	if err := d2.EnsureReady(); err != nil {
		t.Fatalf("旧库初始化失败: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("旧库首次启动应建立索引: %v", err)
	}
	if ids := keywordTestRecallIDs(t, d2, []string{"PersistTag"}, 5); len(ids) != 1 || ids[0] != id {
		t.Fatalf("旧库建索引后应命中，实得 %v", ids)
	}

	// 索引损坏：启动时应幂等重建，不影响召回
	if err := os.WriteFile(path, []byte("not-a-gzip-index"), 0o644); err != nil {
		t.Fatalf("损坏索引失败: %v", err)
	}
	d3 := newStore(dir)
	if err := d3.EnsureReady(); err != nil {
		t.Fatalf("损坏索引后初始化失败: %v", err)
	}
	if _, err := loadKeywordIndexFile(path); err != nil {
		t.Fatalf("损坏索引应被重建为合法文件: %v", err)
	}
	if ids := keywordTestRecallIDs(t, d3, []string{"PersistTag"}, 5); len(ids) != 1 || ids[0] != id {
		t.Fatalf("重建后应命中，实得 %v", ids)
	}
}

// TestKeywordIndexSharedTopKBudget 覆盖关键词候选与向量通道共享 topK 预算，最终统一裁剪。
func TestKeywordIndexSharedTopKBudget(t *testing.T) {
	docs := make([]Document, 0, 10)
	for i := 0; i < 10; i++ {
		docs = append(docs, Document{
			ID:        fmt.Sprintf("kw-%02d", i),
			Type:      TypeText,
			Content:   fmt.Sprintf("目标 共同词 %d", i),
			Timestamp: int64(100 + i),
			Embedding: []float32{0.05, 0.998}, // 与查询 [1,0] 的相似度过不了向量门槛
		})
	}
	d := scopeStore(map[string][]Document{"cluster-a": docs})

	hits, stats := d.keywordIndexRecall([]string{"目标"}, TypeText, 3)
	if stats.Candidates != 10 {
		t.Fatalf("候选数应覆盖全部 10 条，实得 %d", stats.Candidates)
	}
	if len(hits) != 3 || stats.Hits != 3 {
		t.Fatalf("应严格裁剪到共享预算 3 条，实得 %d/%d", len(hits), stats.Hits)
	}
}

// TestKeywordRecallWorksWhenVectorFails 覆盖向量不可用时关键词通道独立返回。
func TestKeywordRecallWorksWhenVectorFails(t *testing.T) {
	docs := map[string][]Document{
		"cluster-a": {
			{ID: "only-keyword", Type: TypeText, Content: "目标 关键词独立召回", Timestamp: 100, Embedding: []float32{1, 0}},
			{ID: "no-match", Type: TypeText, Content: "完全无关", Timestamp: 200, Embedding: []float32{1, 0}},
		},
	}
	server := scopeEmbeddingServer(t)
	defer server.Close()
	d := newHybridTestStore(server, docs)
	// 让向量通道必然失败（关闭测试服务）
	server.Close()

	hits, err := d.HybridQuery(context.Background(), []string{"目标"}, TypeText, 3, HybridQueryOptions{})
	if err != nil {
		t.Fatalf("向量失败时整条检索不应报错: %v", err)
	}
	if len(hits) != 1 || hits[0].ID != "only-keyword" {
		t.Fatalf("关键词通道应独立返回命中，实得 %+v", hits)
	}
	if hits[0].MatchedBy != MatchedByKeyword {
		t.Fatalf("命中来源应为关键词，实得 %q", hits[0].MatchedBy)
	}
}
