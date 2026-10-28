package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/memory"
	"LunarSubsystem/LunarHarness/engine/resource"
)

// memoryHTTPFakeVector 4 维语义向量：天气=[1,0,0,0]，开心=[0,1,0,0]，其它=[0,0,0,1]
func memoryHTTPFakeVector(text string) []float32 {
	switch {
	case strings.Contains(text, "天气"):
		return []float32{1, 0, 0, 0}
	case strings.Contains(text, "开心"):
		return []float32{0, 1, 0, 0}
	default:
		return []float32{0, 0, 0, 1}
	}
}

func TestMemoryHandlerGlobalTypeAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			http.NotFound(w, r)
			return
		}
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
			resp.Data = append(resp.Data, item{Embedding: memoryHTTPFakeVector(input)})
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	origLocal := config.LocalDir
	origArchive := config.ArchiveDBPath
	origURL := config.AgentEmbeddingURL
	origModel := config.AgentEmbeddingModel
	tmp := t.TempDir()
	config.LocalDir = tmp
	config.ArchiveDBPath = filepath.Join(tmp, "database", "archive.db")
	config.AgentEmbeddingURL = server.URL + "/v1"
	config.AgentEmbeddingModel = "test-embedding"
	t.Cleanup(func() {
		_ = resource.Close()
		config.LocalDir = origLocal
		config.ArchiveDBPath = origArchive
		config.AgentEmbeddingURL = origURL
		config.AgentEmbeddingModel = origModel
	})

	memory.Init(filepath.Join(tmp, "memory"))
	if err := memory.EnsureReady(); err != nil {
		t.Fatalf("记忆库初始化失败: %v", err)
	}

	call := func(method, target string, body string) map[string]any {
		t.Helper()
		req := httptest.NewRequest(method, target, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		MemoryHandler(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s %s 返回 %d: %s", method, target, rec.Code, rec.Body.String())
		}
		var payload map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("解析响应失败: %v", err)
		}
		return payload
	}

	// 分组创建（新版前端用 POST /memory/collections + {"name"}）
	created := call(http.MethodPost, "/memory/collections", `{"name":"测试分组"}`)
	createData, _ := created["data"].(map[string]any)
	if createData["name"] != "测试分组" || createData["has_name_embedding"] != true {
		t.Fatalf("分组创建结果不正确: %v", created)
	}
	collectionsPayload := call(http.MethodGet, "/memory/collections", "")
	collectionsData, _ := collectionsPayload["data"].(map[string]any)
	collectionList, _ := collectionsData["collections"].([]any)
	if len(collectionList) == 0 {
		t.Fatalf("分组列表为空: %v", collectionsPayload)
	}

	// 写入文本记忆（不指定集合，内容自动归组）
	payload := call(http.MethodPost, "/memory/messages", `{"role":"user","content":"今天天气不错"}`)
	data, _ := payload["data"].(map[string]any)
	id, _ := data["id"].(string)
	if id == "" {
		t.Fatalf("添加记忆未返回 ID: %v", payload)
	}

	// 文本查询命中；图像查询为空（返回结果只含指定类型）
	textPayload := call(http.MethodGet, "/memory/messages?type=text&query="+urlEncode("天气"), "")
	textData, _ := textPayload["data"].(map[string]any)
	results, _ := textData["results"].([]any)
	if len(results) != 1 {
		t.Fatalf("文本查询应返回 1 条，实得 %v", textData)
	}
	imagePayload := call(http.MethodGet, "/memory/messages?type=image&query="+urlEncode("天气"), "")
	imageData, _ := imagePayload["data"].(map[string]any)
	imageResults, _ := imageData["results"].([]any)
	if len(imageResults) != 0 {
		t.Fatalf("图像查询不应返回文本记忆，实得 %v", imageData)
	}

	// 按 ID 读取与逐条列举
	entryPayload := call(http.MethodGet, "/memory/entries?id="+urlEncode(id), "")
	entry, _ := entryPayload["data"].(map[string]any)
	if entry["id"] != id || entry["type"] != "text" {
		t.Fatalf("按 ID 读取结果不正确: %v", entry)
	}
	listPayload := call(http.MethodGet, "/memory/entries?limit=0", "")
	listData, _ := listPayload["data"].(map[string]any)
	if total, _ := listData["total"].(float64); total != 1 {
		t.Fatalf("逐条列举总数应为 1，实得 %v", listData)
	}

	// 删除（query 参数形式，允许空 body）
	call(http.MethodDelete, "/memory/messages?id="+urlEncode(id), "")
	afterPayload := call(http.MethodGet, "/memory/entries?limit=0", "")
	afterData, _ := afterPayload["data"].(map[string]any)
	if total, _ := afterData["total"].(float64); total != 0 {
		t.Fatalf("删除后条目数应为 0，实得 %v", afterData)
	}
}

// urlEncode 仅用于测试路径参数，避免引入额外依赖
func urlEncode(text string) string {
	return strings.ReplaceAll(strings.ReplaceAll(text, "%", "%25"), " ", "%20")
}
