package memory

// hybrid_test.go 覆盖混合检索的三类关键行为：
//  1. 两路召回的融合排序（向量强 / 关键词强 / 双路命中）；
//  2. 同一条记忆被两路命中时按 ID 去重，只返回一条；
//  3. 嵌入服务不可用或超时时，关键词通道独立返回可用结果。

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
)

// configureHybridEmbedding 临时指向测试嵌入服务，测试结束后恢复。
func configureHybridEmbedding(t *testing.T, url string) {
	t.Helper()
	origURL, origModel := config.AgentEmbeddingURL, config.AgentEmbeddingModel
	config.AgentEmbeddingURL = url
	config.AgentEmbeddingModel = "test-embedding"
	t.Cleanup(func() {
		config.AgentEmbeddingURL, config.AgentEmbeddingModel = origURL, origModel
	})
}

// hybridTestDocs 构造覆盖不同命中形态的文档：
//   - doc-both：向量相似度 0.8，关键词命中 2 次（双路命中）；
//   - doc-vector：向量相似度约 0.95，正文不含关键词（仅向量）；
//   - doc-keyword：向量相似度低于门槛，关键词命中 3 次（仅关键词，强度最高）；
//   - doc-keyword-weak：仅关键词命中 1 次（强度较低）；
//   - doc-weak-vector：向量相似度 0.35（刚过门槛），正文不含关键词（弱向量）。
func hybridTestDocs() map[string][]Document {
	return map[string][]Document{
		"cluster-a": {
			{ID: "doc-both", Type: TypeText, Role: "user", Content: "目标记录：目标很重要", Timestamp: 100, Embedding: []float32{0.8, 0.6}},
			{ID: "doc-vector", Type: TypeText, Role: "user", Content: "完全无关的正文", Timestamp: 200, Embedding: []float32{0.95, 0.313}},
			{ID: "doc-keyword", Type: TypeText, Role: "user", Content: "目标目标目标：原文", Timestamp: 300, Embedding: []float32{0.1, 0.995}},
			{ID: "doc-keyword-weak", Type: TypeText, Role: "user", Content: "只出现一次目标", Timestamp: 400, Embedding: []float32{0.05, 0.999}},
			{ID: "doc-weak-vector", Type: TypeText, Role: "user", Content: "无关正文", Timestamp: 500, Embedding: []float32{0.35, 0.937}},
		},
	}
}

func newHybridTestStore(server *httptest.Server, docs map[string][]Document) *Store {
	d := scopeStore(docs)
	d.httpClient = server.Client()
	return d
}

func findHybridHit(hits []Entry, id string) (Entry, bool) {
	for _, hit := range hits {
		if hit.ID == id {
			return hit, true
		}
	}
	return Entry{}, false
}

// TestHybridQueryFusesAndDedupes 覆盖融合排序与 ID 去重。
//
// 默认权重 0.6/0.4 下预期排序（关键词按覆盖率打分，同覆盖率按时间倒序）：
// doc-both > doc-vector > doc-keyword-weak > doc-keyword > doc-weak-vector。
func TestHybridQueryFusesAndDedupes(t *testing.T) {
	server := scopeEmbeddingServer(t)
	defer server.Close()
	configureHybridEmbedding(t, server.URL+"/v1")

	d := newHybridTestStore(server, hybridTestDocs())

	hits, err := d.HybridQuery(context.Background(), []string{"目标"}, TypeText, 5, HybridQueryOptions{})
	if err != nil {
		t.Fatalf("混合检索失败: %v", err)
	}
	wantOrder := []string{"doc-both", "doc-vector", "doc-keyword-weak", "doc-keyword", "doc-weak-vector"}
	if len(hits) != len(wantOrder) {
		t.Fatalf("返回条数 = %d，期望 %d：%+v", len(hits), len(wantOrder), hits)
	}
	for i, want := range wantOrder {
		if hits[i].ID != want {
			t.Fatalf("第 %d 条 = %s，期望 %s（完整顺序：%v）", i+1, hits[i].ID, want, hitIDs(hits))
		}
	}

	// 同一条记忆被两路命中：只保留一条，且分量都保留
	both, ok := findHybridHit(hits, "doc-both")
	if !ok {
		t.Fatalf("doc-both 应被返回")
	}
	if both.MatchedBy != MatchedByBoth {
		t.Fatalf("doc-both 命中来源 = %q，期望 %q", both.MatchedBy, MatchedByBoth)
	}
	if both.VectorScore <= 0 || both.KeywordScore <= 0 {
		t.Fatalf("doc-both 两路得分应都大于 0：向量 %.3f，关键词 %.3f", both.VectorScore, both.KeywordScore)
	}
	count := 0
	for _, hit := range hits {
		if hit.ID == "doc-both" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("doc-both 出现 %d 次，期望仅 1 次", count)
	}

	// 单一来源标记正确
	vectorHit, _ := findHybridHit(hits, "doc-vector")
	if vectorHit.MatchedBy != MatchedByVector {
		t.Fatalf("doc-vector 命中来源 = %q，期望 %q", vectorHit.MatchedBy, MatchedByVector)
	}
	keywordHit, _ := findHybridHit(hits, "doc-keyword")
	if keywordHit.MatchedBy != MatchedByKeyword {
		t.Fatalf("doc-keyword 命中来源 = %q，期望 %q", keywordHit.MatchedBy, MatchedByKeyword)
	}
	if keywordHit.KeywordScore <= 0 || keywordHit.VectorScore != 0 {
		t.Fatalf("doc-keyword 应仅有关键词分：%+v", keywordHit)
	}
}

// TestHybridQueryRespectsWeights 覆盖权重可配置：提高向量权重后，
// 向量分更高的 doc-vector 应反超双路命中的 doc-both。
func TestHybridQueryRespectsWeights(t *testing.T) {
	server := scopeEmbeddingServer(t)
	defer server.Close()
	configureHybridEmbedding(t, server.URL+"/v1")

	d := newHybridTestStore(server, hybridTestDocs())

	hits, err := d.HybridQuery(context.Background(), []string{"目标"}, TypeText, 2, HybridQueryOptions{
		VectorWeight:  0.9,
		KeywordWeight: 0.1,
	})
	if err != nil {
		t.Fatalf("混合检索失败: %v", err)
	}
	if len(hits) < 2 || hits[0].ID != "doc-vector" || hits[1].ID != "doc-both" {
		t.Fatalf("高向量权重下期望 doc-vector 领先 doc-both，实得 %v", hitIDs(hits))
	}
}

// TestHybridQueryKeywordOnlyWhenEmbeddingFails 覆盖嵌入服务不可用时的降级：
// 向量通道整路失败，但关键词通道仍按覆盖率返回结果（同覆盖率按时间倒序），整条检索不报错。
func TestHybridQueryKeywordOnlyWhenEmbeddingFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "embedding unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	configureHybridEmbedding(t, server.URL+"/v1")

	docs := map[string][]Document{
		"cluster-a": {
			{ID: "kw-strong", Type: TypeText, Role: "user", Content: "目标目标目标", Timestamp: 100, Embedding: []float32{1, 0}},
			{ID: "kw-weak", Type: TypeText, Role: "user", Content: "目标只出现一次", Timestamp: 200, Embedding: []float32{1, 0}},
			{ID: "no-match", Type: TypeText, Role: "user", Content: "完全无关", Timestamp: 300, Embedding: []float32{1, 0}},
		},
	}
	d := newHybridTestStore(server, docs)

	hits, err := d.HybridQuery(context.Background(), []string{"目标"}, TypeText, 3, HybridQueryOptions{})
	if err != nil {
		t.Fatalf("嵌入失败不应让整条检索失败: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("应只返回 2 条关键词命中，实得 %d：%+v", len(hits), hits)
	}
	if hits[0].ID != "kw-weak" || hits[1].ID != "kw-strong" {
		t.Fatalf("关键词应同覆盖率按时间倒序，实得 %v", hitIDs(hits))
	}
	for _, hit := range hits {
		if hit.MatchedBy != MatchedByKeyword {
			t.Fatalf("%s 命中来源 = %q，期望 %q", hit.ID, hit.MatchedBy, MatchedByKeyword)
		}
		if hit.VectorScore != 0 || hit.KeywordScore <= 0 {
			t.Fatalf("%s 应仅有关键词分：%+v", hit.ID, hit)
		}
		if hit.Similarity != hit.KeywordScore*HybridKeywordWeight {
			t.Fatalf("%s 融合分 = %.4f，期望关键词分 × %.1f = %.4f", hit.ID, hit.Similarity, HybridKeywordWeight, hit.KeywordScore*HybridKeywordWeight)
		}
	}
}

// TestHybridQueryKeywordOnlyWhenVectorTimesOut 覆盖向量通道超时：
// 父上下文超时会取消嵌入请求，关键词通道仍独立返回结果。
func TestHybridQueryKeywordOnlyWhenVectorTimesOut(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(300 * time.Millisecond):
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []any{map[string]any{"embedding": []float32{1, 0}}},
		})
	}))
	defer server.Close()
	configureHybridEmbedding(t, server.URL+"/v1")

	docs := map[string][]Document{
		"cluster-a": {
			{ID: "kw-only", Type: TypeText, Role: "user", Content: "目标原文", Timestamp: 100, Embedding: []float32{1, 0}},
		},
	}
	d := newHybridTestStore(server, docs)

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	hits, err := d.HybridQuery(ctx, []string{"目标"}, TypeText, 3, HybridQueryOptions{})
	if err != nil {
		t.Fatalf("向量超时不应让整条检索失败: %v", err)
	}
	if len(hits) != 1 || hits[0].ID != "kw-only" || hits[0].MatchedBy != MatchedByKeyword {
		t.Fatalf("超时后应仅返回关键词结果，实得 %+v", hits)
	}
}

// TestHybridSharedBudgetVectorSurplus 覆盖共享额度：关键词只召回 2 条、
// 向量召回 10 条时，向量补齐剩余额度（topK=5 时返回 2 条双路 + 3 条仅向量）。
func TestHybridSharedBudgetVectorSurplus(t *testing.T) {
	server := scopeEmbeddingServer(t)
	defer server.Close()
	configureHybridEmbedding(t, server.URL+"/v1")

	docs := map[string][]Document{"cluster-a": {}}
	for i := 0; i < 2; i++ {
		docs["cluster-a"] = append(docs["cluster-a"], Document{
			ID:        "both-" + string(rune('a'+i)),
			Type:      TypeText,
			Role:      "user",
			Content:   "目标记录",
			Timestamp: int64(100 + i),
			Embedding: []float32{1, 0},
		})
	}
	for i := 0; i < 8; i++ {
		docs["cluster-a"] = append(docs["cluster-a"], Document{
			ID:        "vector-" + string(rune('a'+i)),
			Type:      TypeText,
			Role:      "user",
			Content:   "无关正文",
			Timestamp: int64(200 + i),
			Embedding: []float32{1, 0},
		})
	}
	d := newHybridTestStore(server, docs)

	hits, err := d.HybridQuery(context.Background(), []string{"目标"}, TypeText, 5, HybridQueryOptions{})
	if err != nil {
		t.Fatalf("混合检索失败: %v", err)
	}
	if len(hits) != 5 {
		t.Fatalf("topK=5 时应返回 5 条（关键词不足由向量补足），实得 %d：%v", len(hits), hitIDs(hits))
	}
	vectorOnly, keywordOnly, both := hybridBudgetUsage(hits)
	if both != 2 || keywordOnly != 0 || vectorOnly != 3 {
		t.Fatalf("额度调配异常：双路 %d，仅关键词 %d，仅向量 %d（期望 2/0/3）", both, keywordOnly, vectorOnly)
	}
}

// TestHybridSharedBudgetKeywordSurplus 覆盖反向调配：向量只有 2 条、
// 关键词召回 10 条时，关键词补齐剩余额度（topK=5 时 2 条双路 + 3 条仅关键词）。
func TestHybridSharedBudgetKeywordSurplus(t *testing.T) {
	server := scopeEmbeddingServer(t)
	defer server.Close()
	configureHybridEmbedding(t, server.URL+"/v1")

	docs := map[string][]Document{"cluster-a": {}}
	for i := 0; i < 2; i++ {
		docs["cluster-a"] = append(docs["cluster-a"], Document{
			ID:        "both-" + string(rune('a'+i)),
			Type:      TypeText,
			Role:      "user",
			Content:   "目标记录",
			Timestamp: int64(100 + i),
			Embedding: []float32{1, 0},
		})
	}
	for i := 0; i < 8; i++ {
		docs["cluster-a"] = append(docs["cluster-a"], Document{
			ID:        "keyword-" + string(rune('a'+i)),
			Type:      TypeText,
			Role:      "user",
			Content:   "目标无关向量",
			Timestamp: int64(200 + i),
			Embedding: []float32{0, 1},
		})
	}
	d := newHybridTestStore(server, docs)

	hits, err := d.HybridQuery(context.Background(), []string{"目标"}, TypeText, 5, HybridQueryOptions{})
	if err != nil {
		t.Fatalf("混合检索失败: %v", err)
	}
	if len(hits) != 5 {
		t.Fatalf("topK=5 时应返回 5 条（向量不足由关键词补足），实得 %d：%v", len(hits), hitIDs(hits))
	}
	vectorOnly, keywordOnly, both := hybridBudgetUsage(hits)
	if both != 2 || keywordOnly != 3 || vectorOnly != 0 {
		t.Fatalf("额度调配异常：双路 %d，仅关键词 %d，仅向量 %d（期望 2/3/0）", both, keywordOnly, vectorOnly)
	}
}

// TestHybridSharedBudgetNeverExceedsTopK 覆盖总返回条数硬约束。
func TestHybridSharedBudgetNeverExceedsTopK(t *testing.T) {
	server := scopeEmbeddingServer(t)
	defer server.Close()
	configureHybridEmbedding(t, server.URL+"/v1")

	docs := map[string][]Document{"cluster-a": {}}
	for i := 0; i < 10; i++ {
		docs["cluster-a"] = append(docs["cluster-a"], Document{
			ID:        "doc-" + string(rune('a'+i)),
			Type:      TypeText,
			Role:      "user",
			Content:   "目标记录",
			Timestamp: int64(100 + i),
			Embedding: []float32{1, 0},
		})
	}
	d := newHybridTestStore(server, docs)

	hits, err := d.HybridQuery(context.Background(), []string{"目标"}, TypeText, 3, HybridQueryOptions{})
	if err != nil {
		t.Fatalf("混合检索失败: %v", err)
	}
	if len(hits) != 3 {
		t.Fatalf("topK=3 时返回 %d 条，超出预算：%v", len(hits), hitIDs(hits))
	}
}

// TestHybridQueryVectorOnlyWhenKeywordRegexInvalid 覆盖关键词正则非法时的降级：
// 关键词通道报错，向量通道单独返回，整条检索不失败。
func TestHybridQueryVectorOnlyWhenKeywordRegexInvalid(t *testing.T) {
	server := scopeEmbeddingServer(t)
	defer server.Close()
	configureHybridEmbedding(t, server.URL+"/v1")

	docs := map[string][]Document{
		"cluster-a": {
			{ID: "vec-only", Type: TypeText, Role: "user", Content: "目标记录", Timestamp: 100, Embedding: []float32{1, 0}},
		},
	}
	d := newHybridTestStore(server, docs)

	hits, err := d.HybridQuery(context.Background(), []string{"目标("}, TypeText, 3, HybridQueryOptions{Regex: true})
	if err != nil {
		t.Fatalf("关键词正则非法时不应让整条检索失败: %v", err)
	}
	if len(hits) != 1 || hits[0].ID != "vec-only" || hits[0].MatchedBy != MatchedByVector {
		t.Fatalf("应仅返回向量结果，实得 %+v", hits)
	}
	if hits[0].KeywordScore != 0 || hits[0].VectorScore <= 0 {
		t.Fatalf("降级结果应只含向量分：%+v", hits[0])
	}
}

// TestHybridQueryFailsOnlyWhenBothChannelsFail 覆盖两路都失败才报错。
func TestHybridQueryFailsOnlyWhenBothChannelsFail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "embedding unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	configureHybridEmbedding(t, server.URL+"/v1")

	docs := map[string][]Document{
		"cluster-a": {
			{ID: "doc", Type: TypeText, Role: "user", Content: "目标记录", Timestamp: 100, Embedding: []float32{1, 0}},
		},
	}
	d := newHybridTestStore(server, docs)

	if _, err := d.HybridQuery(context.Background(), []string{"目标("}, TypeText, 3, HybridQueryOptions{Regex: true}); err == nil {
		t.Fatalf("两路均失败时应返回错误")
	}
}

func hitIDs(hits []Entry) []string {
	ids := make([]string, 0, len(hits))
	for _, hit := range hits {
		ids = append(ids, hit.ID)
	}
	return ids
}
