package persistence

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
)

// fakeEmbeddingVector 把文本映射为 4 维语义向量，供路由/检索测试使用：
// 天气=[1,0,0,0]，开心=[0,1,0,0]，代码=[0,0,1,0]，其它=[0,0,0,1]。
func fakeEmbeddingVector(text string) []float32 {
	vec := []float32{0, 0, 0, 1}
	switch {
	case strings.Contains(text, "天气"):
		vec = []float32{1, 0, 0, 0}
	case strings.Contains(text, "开心"):
		vec = []float32{0, 1, 0, 0}
	case strings.Contains(text, "代码"):
		vec = []float32{0, 0, 1, 0}
	}
	return vec
}

// startFakeEmbeddingServer 启动 OpenAI 兼容的 /v1/embeddings + /v1/chat/completions 假服务
func startFakeEmbeddingServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/chat/completions":
			// 图片标签生成：固定返回两个标签，测试图片按 URL 去重而不是按标签去重
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"[\"可爱\",\"猫\"]"}}]}`))
			return
		case "/v1/embeddings":
			// 继续下面的嵌入逻辑
		default:
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
		type dataItem struct {
			Embedding []float32 `json:"embedding"`
		}
		resp := struct {
			Data []dataItem `json:"data"`
		}{}
		for _, input := range req.Input {
			resp.Data = append(resp.Data, dataItem{Embedding: fakeEmbeddingVector(input)})
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

// isolateStorage 把 LocalDir 与资源目录数据库指向临时目录，避免测试写入仓库数据。
func isolateStorage(t *testing.T) {
	t.Helper()
	origLocal := *config.LocalDir
	origArchive := *config.ArchiveDBPath
	tmp := t.TempDir()
	*config.LocalDir = tmp
	*config.ArchiveDBPath = filepath.Join(tmp, "database", "archive.db")
	t.Cleanup(func() {
		// 先关闭 SQLite 连接，否则 Windows 上临时目录因文件占用无法删除
		_ = CloseArchiveDB()
		*config.LocalDir = origLocal
		*config.ArchiveDBPath = origArchive
	})
}

func TestMemoryEndToEndTypeRouting(t *testing.T) {
	server := startFakeEmbeddingServer(t)
	defer server.Close()

	isolateStorage(t)
	origURL := *config.AgentEmbeddingURL
	origModel := *config.AgentEmbeddingModel
	*config.AgentEmbeddingURL = server.URL + "/v1"
	*config.AgentEmbeddingModel = "test-embedding"
	t.Cleanup(func() {
		*config.AgentEmbeddingURL = origURL
		*config.AgentEmbeddingModel = origModel
	})

	d := initMemoryDBInstance(t.TempDir())
	if err := d.MemoryInitInstance(); err != nil {
		t.Fatalf("记忆库初始化失败: %v", err)
	}
	if got := d.MemoryGetCollectionCount(); got < len(DefaultMemoryCollections) {
		t.Fatalf("默认分组未全部创建: %d < %d", got, len(DefaultMemoryCollections))
	}

	ctx := context.Background()
	textID, err := d.MemoryAddMessage(ctx, "user", "今天天气不错")
	if err != nil {
		t.Fatalf("添加文本记忆失败: %v", err)
	}
	entry, err := d.MemoryGetEntry(textID)
	if err != nil {
		t.Fatalf("按 ID 读取文本记忆失败: %v", err)
	}
	if entry.Type != MemoryTypeText || entry.Collection == "" {
		t.Fatalf("文本记忆类型/分组不正确: %+v", entry)
	}

	textHits, err := d.MemoryQuery(ctx, "天气", MemoryTypeText, 5)
	if err != nil {
		t.Fatalf("文本查询失败: %v", err)
	}
	if len(textHits) != 1 || textHits[0].ID != textID {
		t.Fatalf("文本查询应命中刚写入的条目: %+v", textHits)
	}

	imageHits, err := d.MemoryQuery(ctx, "天气", MemoryTypeImage, 5)
	if err != nil {
		t.Fatalf("图片查询失败: %v", err)
	}
	if len(imageHits) != 0 {
		t.Fatalf("按图片类型查询不应返回文本条目: %+v", imageHits)
	}

	audioID, err := d.MemoryAddAudio(ctx, "data:audio/mp3;base64,SUQzBA==", "开心的笑声")
	if err != nil {
		t.Fatalf("添加音频记忆失败: %v", err)
	}
	audioHits, err := d.MemoryQuery(ctx, "开心", MemoryTypeAudio, 5)
	if err != nil {
		t.Fatalf("音频查询失败: %v", err)
	}
	if len(audioHits) != 1 || audioHits[0].ID != audioID || audioHits[0].URL == "" {
		t.Fatalf("音频查询应命中刚写入的条目: %+v", audioHits)
	}

	entries, total := d.MemoryListEntries("", 0, 0)
	if total != 2 || len(entries) != 2 {
		t.Fatalf("逐条列举应返回两条记忆，实得 total=%d len=%d", total, len(entries))
	}
	if err := d.MemoryDeleteMessage(ctx, audioID); err != nil {
		t.Fatalf("删除音频记忆失败: %v", err)
	}
	if got := d.MemoryCountEntries(MemoryTypeAudio); got != 0 {
		t.Fatalf("删除后音频条目数应为 0，实得 %d", got)
	}
}

// TestMemoryAddImageDedupByContent 图片记忆应按实体内容去重，而不是按标签去重：
// 同一张图重复入库只保留一条；不同图片即使标签相同也各自保留。
func TestMemoryAddImageDedupByContent(t *testing.T) {
	server := startFakeEmbeddingServer(t)
	defer server.Close()

	isolateStorage(t)
	origURL := *config.AgentEmbeddingURL
	origModel := *config.AgentEmbeddingModel
	origMMURL := *config.AgentMultimodalURL
	origMMModel := *config.AgentMultimodalModel
	*config.AgentEmbeddingURL = server.URL + "/v1"
	*config.AgentEmbeddingModel = "test-embedding"
	*config.AgentMultimodalURL = server.URL + "/v1"
	*config.AgentMultimodalModel = "test-multimodal"
	t.Cleanup(func() {
		*config.AgentEmbeddingURL = origURL
		*config.AgentEmbeddingModel = origModel
		*config.AgentMultimodalURL = origMMURL
		*config.AgentMultimodalModel = origMMModel
	})

	d := initMemoryDBInstance(t.TempDir())
	if err := d.MemoryInitInstance(); err != nil {
		t.Fatalf("记忆库初始化失败: %v", err)
	}

	const pngOne = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="
	const pngTwo = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAIAAAACCAIAAAD91JpzAAAAEElEQVR4nGP8zwACTGCSAQANHQEDgslx/wAAAABJRU5ErkJggg=="

	ctx := context.Background()
	firstID, err := d.MemoryAddImage(ctx, pngOne, "auto", "")
	if err != nil {
		t.Fatalf("添加第一张图片失败: %v", err)
	}
	entry, err := d.MemoryGetEntry(firstID)
	if err != nil {
		t.Fatalf("读取图片记忆失败: %v", err)
	}
	if entry.Type != MemoryTypeImage || entry.URL == "" {
		t.Fatalf("图片记忆类型/URL 不正确: %+v", entry)
	}
	if !strings.Contains(entry.URL, "/multimedia/memory/") {
		t.Fatalf("新版图片应落在 multimedia/memory/<分组>/，实得 %s", entry.URL)
	}
	imageAbs := filepath.Join(*config.LocalDir, filepath.FromSlash(strings.TrimPrefix(entry.URL, "/file/read/")))
	if _, err := os.Stat(imageAbs); err != nil {
		t.Fatalf("图片实体未落盘: %v", err)
	}

	// 同一张图重复入库：按内容哈希去重，返回既有 ID
	againID, err := d.MemoryAddImage(ctx, pngOne, "auto", "")
	if err != nil {
		t.Fatalf("重复添加同一张图片失败: %v", err)
	}
	againEntry, _ := d.MemoryGetEntry(againID)
	if againID != firstID {
		t.Fatalf("同一张图片应复用既有记忆: %s != %s (%+v)", againID, firstID, againEntry)
	}

	// 不同图片、相同标签：必须各自保留
	secondID, err := d.MemoryAddImage(ctx, pngTwo, "auto", "")
	if err != nil {
		t.Fatalf("添加第二张图片失败: %v", err)
	}
	if secondID == firstID {
		t.Fatal("不同图片即使标签相同也不应被去重")
	}
	hits, err := d.MemoryQuery(ctx, "可爱", MemoryTypeImage, 5)
	if err != nil {
		t.Fatalf("图片类型查询失败: %v", err)
	}
	if len(hits) < 2 {
		t.Fatalf("图片查询应返回两张不同图片，实得 %d", len(hits))
	}

	// 删除条目时应顺带删除其图片实体（第二张仍在，不受影响）
	if err := d.MemoryDeleteMessage(ctx, firstID); err != nil {
		t.Fatalf("删除图片记忆失败: %v", err)
	}
	if _, err := os.Stat(imageAbs); !os.IsNotExist(err) {
		t.Fatalf("删除记忆条目后图片实体仍存在: %v", err)
	}

	// 清空分组时同样应清理剩余图片
	secondEntry, err := d.MemoryGetEntry(secondID)
	if err != nil {
		t.Fatalf("读取第二张图片失败: %v", err)
	}
	secondAbs := filepath.Join(*config.LocalDir, filepath.FromSlash(strings.TrimPrefix(secondEntry.URL, "/file/read/")))
	if _, err := os.Stat(secondAbs); err != nil {
		t.Fatalf("第二张图片实体不存在: %v", err)
	}
	if err := d.MemoryClearCollection(secondEntry.Collection); err != nil {
		t.Fatalf("清空分组失败: %v", err)
	}
	if _, err := os.Stat(secondAbs); !os.IsNotExist(err) {
		t.Fatalf("清空分组后图片实体仍存在: %v", err)
	}
}

// TestMemoryNameEmbeddingPersisted 集合创建后应把集合名向量写入 metadata.json，
// 重启加载时无需再次请求嵌入服务即可参与归组。
func TestMemoryNameEmbeddingPersisted(t *testing.T) {
	server := startFakeEmbeddingServer(t)
	defer server.Close()

	isolateStorage(t)
	origURL := *config.AgentEmbeddingURL
	origModel := *config.AgentEmbeddingModel
	*config.AgentEmbeddingURL = server.URL + "/v1"
	*config.AgentEmbeddingModel = "test-embedding"
	t.Cleanup(func() {
		*config.AgentEmbeddingURL = origURL
		*config.AgentEmbeddingModel = origModel
	})

	base := t.TempDir()
	first := initMemoryDBInstance(base)
	if err := first.MemoryInitInstance(); err != nil {
		t.Fatalf("首次初始化失败: %v", err)
	}

	// 模拟重启：新的实例从同一目录加载，集合名向量应从 metadata 恢复
	server.Close()
	second := initMemoryDBInstance(base)
	if err := second.MemoryInitInstance(); err != nil {
		t.Fatalf("重启初始化失败: %v", err)
	}
	info := second.MemoryGetCollectionInfo(DefaultMemoryCollections[0])
	if info == nil {
		t.Fatal("重启后集合信息为空")
	}
	hasEmbedding, _ := info["has_name_embedding"].(bool)
	if !hasEmbedding {
		t.Fatalf("重启后集合名向量未恢复: %+v", info)
	}
}
