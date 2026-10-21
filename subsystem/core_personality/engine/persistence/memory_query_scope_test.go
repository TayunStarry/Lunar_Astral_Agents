package persistence

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"LunarSubsystem/GeneralConfig"
)

// scopeEmbeddingServer 假嵌入服务：查询文本含“目标”时映射为 [1,0]，否则 [0,1]。
func scopeEmbeddingServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		type item struct {
			Embedding []float32 `json:"embedding"`
		}
		resp := struct {
			Data []item `json:"data"`
		}{}
		for _, input := range req.Input {
			vec := []float32{0, 1}
			if strings.Contains(input, "目标") {
				vec = []float32{1, 0}
			}
			resp.Data = append(resp.Data, item{Embedding: vec})
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

// scopeMemoryDB 构造一个只有 6 个集合的内存记忆库：
// 01~05 的集合名与查询 [1,0] 的相似度依次为 1、.95、.9、.8、.6，06 为 .2。
func scopeMemoryDB(docsByCollection map[string][]Document) *MemoryDB {
	names := []struct {
		name string
		vec  []float32
	}{
		{"01-相近", []float32{1, 0}},
		{"02-次近", []float32{0.95, 0.31}},
		{"03-第三", []float32{0.9, 0.44}},
		{"04-第四", []float32{0.8, 0.6}},
		{"05-第五", []float32{0.6, 0.8}},
		{"06-较远", []float32{0.2, 0.98}},
	}
	collections := make(map[string]*Collection, len(names))
	for _, entry := range names {
		collections[entry.name] = &Collection{
			Name:          entry.name,
			Model:         "test-embedding",
			Dimension:     2,
			NameEmbedding: entry.vec,
			Documents:     docsByCollection[entry.name],
		}
	}
	return &MemoryDB{
		memoryInitialized: true,
		baseDir:           "",
		collections:       collections,
	}
}

func scopeTestDocs() (map[string][]Document, string, string) {
	const idNear = "doc-near"
	const idFar = "doc-far"
	docs := map[string][]Document{
		"01-相近": {{ID: idNear, Type: MemoryTypeText, Role: "user", Content: "目标很近", Timestamp: 100, Embedding: []float32{1, 0}}},
		"06-较远": {{ID: idFar, Type: MemoryTypeText, Role: "user", Content: "目标很远", Timestamp: 200, Embedding: []float32{1, 0}}},
	}
	return docs, idNear, idFar
}

// TestMemoryQueryFastScopeSkipsFarCollections 覆盖快速检索：首集合命中时只扫描
// 最相近的 5 个集合，不会去扫第 6 个（未来的大数据量就是靠这层收敛查询成本）。
func TestMemoryQueryFastScopeSkipsFarCollections(t *testing.T) {
	server := scopeEmbeddingServer(t)
	defer server.Close()
	origURL, origModel := *GeneralConfig.AgentEmbeddingURL, *GeneralConfig.AgentEmbeddingModel
	*GeneralConfig.AgentEmbeddingURL = server.URL + "/v1"
	*GeneralConfig.AgentEmbeddingModel = "test-embedding"
	t.Cleanup(func() {
		*GeneralConfig.AgentEmbeddingURL, *GeneralConfig.AgentEmbeddingModel = origURL, origModel
	})

	docs, idNear, idFar := scopeTestDocs()
	d := scopeMemoryDB(docs)
	d.httpClient = server.Client()

	fast, err := d.MemoryQuery(context.Background(), "目标", MemoryTypeText, 10)
	if err != nil {
		t.Fatalf("快速检索失败: %v", err)
	}
	if len(fast) != 1 || fast[0].ID != idNear {
		t.Fatalf("快速检索应只命中首集合条目 %s，实得 %+v", idNear, fast)
	}

	global, err := d.MemoryQueryWithOptions(context.Background(), "目标", MemoryTypeText, 10, MemoryQueryOptions{ForceGlobal: true})
	if err != nil {
		t.Fatalf("全局检索失败: %v", err)
	}
	seen := map[string]bool{}
	for _, hit := range global {
		seen[hit.ID] = true
	}
	if !seen[idNear] || !seen[idFar] {
		t.Fatalf("全局检索应同时命中 %s 与 %s，实得 %+v", idNear, idFar, global)
	}
}

// TestMemoryQueryFastScopeFallsBackWhenFirstMisses 覆盖回退：最相近的集合没有
// 达到门槛的结果时，必须扫描其余全部集合，不能因为快速路由而漏掉远处集合的命中。
func TestMemoryQueryFastScopeFallsBackWhenFirstMisses(t *testing.T) {
	server := scopeEmbeddingServer(t)
	defer server.Close()
	origURL, origModel := *GeneralConfig.AgentEmbeddingURL, *GeneralConfig.AgentEmbeddingModel
	*GeneralConfig.AgentEmbeddingURL = server.URL + "/v1"
	*GeneralConfig.AgentEmbeddingModel = "test-embedding"
	t.Cleanup(func() {
		*GeneralConfig.AgentEmbeddingURL, *GeneralConfig.AgentEmbeddingModel = origURL, origModel
	})

	_, idNear, idFar := scopeTestDocs()
	d := scopeMemoryDB(map[string][]Document{
		// 首集合为空：快速检索必须回退到其余集合
		"06-较远": {{ID: idFar, Type: MemoryTypeText, Role: "user", Content: "目标很远", Timestamp: 200, Embedding: []float32{1, 0}}},
	})
	_ = idNear
	d.httpClient = server.Client()

	hits, err := d.MemoryQuery(context.Background(), "目标", MemoryTypeText, 10)
	if err != nil {
		t.Fatalf("回退检索失败: %v", err)
	}
	if len(hits) != 1 || hits[0].ID != idFar {
		t.Fatalf("首集合未命中时应回退扫描全部集合并命中 %s，实得 %+v", idFar, hits)
	}
}
