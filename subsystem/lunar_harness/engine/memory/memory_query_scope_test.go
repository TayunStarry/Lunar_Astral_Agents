package memory

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
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

// scopeStore 构造一个只有 6 个簇的内存记忆库：
// a~e 的簇质心与查询 [1,0] 的相似度依次为 1、.95、.9、.8、.6，f 为 .2。
func scopeStore(docsByCluster map[string][]Document) *Store {
	specs := []struct {
		id       string
		centroid []float32
	}{
		{"cluster-a", []float32{1, 0}},
		{"cluster-b", []float32{0.95, 0.31}},
		{"cluster-c", []float32{0.9, 0.44}},
		{"cluster-d", []float32{0.8, 0.6}},
		{"cluster-e", []float32{0.6, 0.8}},
		{"cluster-f", []float32{0.2, 0.98}},
	}
	collections := make(map[string]*Collection, len(specs))
	clusters := make([]ClusterNode, 0, len(specs))
	assignments := make(map[string]string)
	for _, spec := range specs {
		docs := docsByCluster[spec.id]
		collections[spec.id] = &Collection{
			Name:      spec.id,
			Dimension: 2,
			Documents: docs,
		}
		docIDs := make([]string, 0, len(docs))
		for _, doc := range docs {
			docIDs = append(docIDs, doc.ID)
			assignments[doc.ID] = spec.id
		}
		clusters = append(clusters, ClusterNode{
			ID:       spec.id,
			Centroid: spec.centroid,
			DocIDs:   docIDs,
			Size:     len(docs),
		})
	}
	return &Store{
		initialized: true,
		baseDir:     "",
		collections: collections,
		clusterIndex: &ClusterIndex{
			Version:     ClusterIndexVersion,
			Dimension:   2,
			Clusters:    clusters,
			Assignments: assignments,
		},
	}
}

func scopeTestDocs() (map[string][]Document, string, string) {
	const idNear = "doc-near"
	const idFar = "doc-far"
	docs := map[string][]Document{
		"cluster-a": {{ID: idNear, Type: TypeText, Role: "user", Content: "目标很近", Timestamp: 100, Embedding: []float32{1, 0}}},
		"cluster-f": {{ID: idFar, Type: TypeText, Role: "user", Content: "目标很远", Timestamp: 200, Embedding: []float32{1, 0}}},
	}
	return docs, idNear, idFar
}

// TestQueryFastScopeSkipsFarCollections 覆盖路由收敛：质心相似度 0.2 的远处簇
// （cluster-f）过不了 ClusterCentroidGate(0.70)，路由阶段不会扫描它的成员；
// 请求 topK=1 且首簇已取满时，不再回落，远处簇条目不会被比较。
func TestQueryFastScopeSkipsFarCollections(t *testing.T) {
	server := scopeEmbeddingServer(t)
	defer server.Close()
	origURL, origModel := config.AgentEmbeddingURL, config.AgentEmbeddingModel
	config.AgentEmbeddingURL = server.URL + "/v1"
	config.AgentEmbeddingModel = "test-embedding"
	t.Cleanup(func() {
		config.AgentEmbeddingURL, config.AgentEmbeddingModel = origURL, origModel
	})

	docs, idNear, idFar := scopeTestDocs()
	d := scopeStore(docs)
	d.httpClient = server.Client()

	fast, stats, err := d.QueryWithStats(context.Background(), "目标", TypeText, 1, QueryOptions{})
	if err != nil {
		t.Fatalf("快速检索失败: %v", err)
	}
	if len(fast) != 1 || fast[0].ID != idNear {
		t.Fatalf("快速检索应只命中首簇条目 %s，实得 %+v", idNear, fast)
	}
	// a/b/c/d 质心相似度 1/.95/.9/.8 通过门限，e(.6)、f(.2) 被跳过
	if stats.ClustersCompared != 6 || stats.ClustersHit != 4 {
		t.Fatalf("质心路由异常: compared=%d hit=%d", stats.ClustersCompared, stats.ClustersHit)
	}
	if stats.UsedFallback || stats.DocsCompared != 1 {
		t.Fatalf("取满 topK 后不应回落且只比较簇内 1 条，实得 %+v", stats)
	}

	global, err := d.Query(context.Background(), "目标", TypeText, 10, QueryOptions{ForceGlobal: true})
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

// TestQueryFastScopeFallsBackWhenFirstMisses 覆盖回退：最相近的簇没有
// 达到门槛的结果时，必须扫描其余全部簇，不能因为快速路由而漏掉远处簇的命中。
func TestQueryFastScopeFallsBackWhenFirstMisses(t *testing.T) {
	server := scopeEmbeddingServer(t)
	defer server.Close()
	origURL, origModel := config.AgentEmbeddingURL, config.AgentEmbeddingModel
	config.AgentEmbeddingURL = server.URL + "/v1"
	config.AgentEmbeddingModel = "test-embedding"
	t.Cleanup(func() {
		config.AgentEmbeddingURL, config.AgentEmbeddingModel = origURL, origModel
	})

	_, _, idFar := scopeTestDocs()
	d := scopeStore(map[string][]Document{
		// 首簇为空：快速检索必须回退到其余簇
		"cluster-f": {{ID: idFar, Type: TypeText, Role: "user", Content: "目标很远", Timestamp: 200, Embedding: []float32{1, 0}}},
	})
	d.httpClient = server.Client()

	hits, err := d.Query(context.Background(), "目标", TypeText, 10, QueryOptions{})
	if err != nil {
		t.Fatalf("回退检索失败: %v", err)
	}
	if len(hits) != 1 || hits[0].ID != idFar {
		t.Fatalf("首簇未命中时应回退扫描全部簇并命中 %s，实得 %+v", idFar, hits)
	}
}
