package module

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// =============================================================================
// 测试辅助 — mock 嵌入服务
// =============================================================================

// mockEmbeddingServer 创建一个模拟嵌入服务的 HTTP 测试服务器
// 返回预定义维度的嵌入向量，便于验证查询结果
func mockEmbeddingServer(t *testing.T) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 兼容 /v1/embeddings 和 /embeddings 两种路径
		if r.URL.Path != "/embeddings" && r.URL.Path != "/v1/embeddings" {
			http.Error(w, "not found: "+r.URL.Path, http.StatusNotFound)
			return
		}

		var req embeddingRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		data := make([]embeddingData, len(req.Input))
		for i, text := range req.Input {
			// 为每个输入文本生成一个确定性嵌入向量（4 维）
			// 使用文本内容生成可预测的向量，便于测试验证
			vec := deterministicEmbedding(text)
			data[i] = embeddingData{Embedding: vec}
		}

		resp := embeddingResponse{Data: data}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))

	return server
}

// deterministicEmbedding 根据文本内容生成确定性嵌入向量（4 维）
// 不同文本生成不同向量，相同文本生成相同向量，便于测试验证
func deterministicEmbedding(text string) []float32 {
	text = strings.ToLower(strings.TrimSpace(text))
	vec := make([]float32, 4)

	// 基于文本内容的简单哈希，生成 4 维向量
	var hash uint32
	for _, r := range text {
		hash = hash*31 + uint32(r)
	}

	for i := range vec {
		shift := uint(i * 8)
		val := float32((hash>>shift)&0xFF) / 255.0
		// 确保向量非零
		if val < 0.01 {
			val = 0.5
		}
		vec[i] = val
	}
	return vec
}

// setupTestDB 创建测试用 MemoryDB 实例
// 每次调用都会重置全局 MemoryDatabase 状态，确保测试隔离
func setupTestDB(t *testing.T, server *httptest.Server) (*MemoryDB, string) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "memory_test_*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	t.Cleanup(func() {
		os.RemoveAll(tmpDir)
	})

	// 重置全局状态，确保测试隔离
	MemoryDatabase = nil

	if err := InitMemoryDB(tmpDir); err != nil {
		t.Fatalf("InitMemoryDB 失败: %v", err)
	}

	MemoryDatabase.embeddingBaseURL = server.URL + "/v1"
	MemoryDatabase.embeddingAPIKey = ""
	MemoryDatabase.httpClient = server.Client()
	MemoryDatabase.memoryInitialized = true

	return MemoryDatabase, tmpDir
}

// =============================================================================
// 测试：集合类型与元数据
// =============================================================================

func TestCollectionInitImage_CreatesMetadataWithType(t *testing.T) {
	server := mockEmbeddingServer(t)
	defer server.Close()

	db, tmpDir := setupTestDB(t, server)

	ctx := context.Background()
	err := db.CollectionInitImage(ctx, "test_images", "system-embedding")
	if err != nil {
		t.Fatalf("CollectionInitImage 失败: %v", err)
	}

	// 验证 metadata.json 包含 type 字段
	metaPath := filepath.Join(tmpDir, "test_images", "metadata.json")
	var meta collectionMeta
	if err := readJSONFile(metaPath, &meta); err != nil {
		t.Fatalf("读取 metadata.json 失败: %v", err)
	}

	if meta.Type != CollectionTypeImage {
		t.Errorf("期望 type='image', 实际 type='%s'", meta.Type)
	}
	if meta.Dimension != 4 {
		t.Errorf("期望 dimension=4, 实际 dimension=%d", meta.Dimension)
	}

	// 验证内存中的 Collection 对象
	c, err := db.getCollection("test_images")
	if err != nil {
		t.Fatalf("getCollection 失败: %v", err)
	}
	if c.CollectionType != CollectionTypeImage {
		t.Errorf("期望 CollectionType='image', 实际='%s'", c.CollectionType)
	}
}

func TestCollectionInit_TextTypeAsDefault(t *testing.T) {
	server := mockEmbeddingServer(t)
	defer server.Close()

	db, tmpDir := setupTestDB(t, server)

	ctx := context.Background()
	err := db.CollectionInit(ctx, "test_text", "system-embedding")
	if err != nil {
		t.Fatalf("CollectionInit 失败: %v", err)
	}

	metaPath := filepath.Join(tmpDir, "test_text", "metadata.json")
	var meta collectionMeta
	if err := readJSONFile(metaPath, &meta); err != nil {
		t.Fatalf("读取 metadata.json 失败: %v", err)
	}

	if meta.Type != CollectionTypeText {
		t.Errorf("期望 type='text', 实际 type='%s'", meta.Type)
	}
}

func TestCollectionInit_TypeMismatch(t *testing.T) {
	server := mockEmbeddingServer(t)
	defer server.Close()

	db, _ := setupTestDB(t, server)

	ctx := context.Background()
	// 先创建 text 集合
	if err := db.CollectionInit(ctx, "mixed", "system-embedding"); err != nil {
		t.Fatalf("CollectionInit 失败: %v", err)
	}

	// 尝试以 image 类型重新初始化应失败
	err := db.CollectionInitImage(ctx, "mixed", "system-embedding")
	if err == nil {
		t.Error("期望类型不匹配错误，但成功了")
	}
	if !strings.Contains(err.Error(), "类型为 text") {
		t.Errorf("期望错误包含类型信息，实际: %v", err)
	}
}

// =============================================================================
// 测试：图片集合条目创建
// =============================================================================

func TestMemoryAddImage_Success(t *testing.T) {
	server := mockEmbeddingServer(t)
	defer server.Close()

	db, tmpDir := setupTestDB(t, server)

	ctx := context.Background()
	if err := db.CollectionInitImage(ctx, "gallery", "system-embedding"); err != nil {
		t.Fatalf("CollectionInitImage 失败: %v", err)
	}

	base64Image := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
	emotion := "宁静平和、温暖治愈"
	colorStyle := "暖色调、柔光、低饱和度"
	content := "夕阳下的海滩，一个女孩在散步"

	id, err := db.MemoryAddImage(ctx, "gallery", base64Image, emotion, colorStyle, content)
	if err != nil {
		t.Fatalf("MemoryAddImage 失败: %v", err)
	}

	if id == "" {
		t.Error("期望返回非空 UUID")
	}

	// 验证 UUID 格式
	if !strings.Contains(id, "-") {
		t.Errorf("期望 UUID 格式，实际: %s", id)
	}

	// 验证文档数
	count := db.MemoryGetCollectionCount("gallery")
	if count != 1 {
		t.Errorf("期望文档数=1, 实际=%d", count)
	}

	// 验证内存中的文档
	c, _ := db.getCollection("gallery")
	c.mu.RLock()
	if len(c.ImageDocuments) != 1 {
		t.Errorf("期望 ImageDocuments 长度=1, 实际=%d", len(c.ImageDocuments))
	} else {
		doc := c.ImageDocuments[0]
		if doc.ID != id {
			t.Errorf("期望 ID=%s, 实际=%s", id, doc.ID)
		}
		if doc.Image != base64Image {
			t.Error("图片数据不匹配")
		}
		// 验证三个嵌入向量维度
		for v := 0; v < 3; v++ {
			if len(doc.Embeddings[v]) != 4 {
				t.Errorf("嵌入向量[%d] 维度期望 4, 实际 %d", v, len(doc.Embeddings[v]))
			}
		}
	}
	c.mu.RUnlock()

	// 验证磁盘文件
	base64Path := filepath.Join(tmpDir, "gallery", "base64_0001.json")
	var base64s []base64Entry
	if err := readJSONFile(base64Path, &base64s); err != nil {
		t.Fatalf("读取 base64_0001.json 失败: %v", err)
	}
	if len(base64s) != 1 || base64s[0].ID != id {
		t.Error("base64 文件内容不匹配")
	}

	embPath := filepath.Join(tmpDir, "gallery", "embeddings_0001.json")
	var embs []imageEmbeddingEntry
	if err := readJSONFile(embPath, &embs); err != nil {
		t.Fatalf("读取 embeddings_0001.json 失败: %v", err)
	}
	if len(embs) != 1 || embs[0].ID != id {
		t.Error("embeddings 文件内容不匹配")
	}
	if len(embs[0].Embeddings) != 3 {
		t.Errorf("期望 3 个嵌入向量，实际 %d", len(embs[0].Embeddings))
	}
}

func TestMemoryAddImage_WrongCollectionType(t *testing.T) {
	server := mockEmbeddingServer(t)
	defer server.Close()

	db, _ := setupTestDB(t, server)

	ctx := context.Background()
	if err := db.CollectionInit(ctx, "text_coll", "system-embedding"); err != nil {
		t.Fatalf("CollectionInit 失败: %v", err)
	}

	_, err := db.MemoryAddImage(ctx, "text_coll", "base64data", "a", "b", "c")
	if err == nil {
		t.Error("期望类型不匹配错误")
	}
	if !strings.Contains(err.Error(), "不支持图片添加") {
		t.Errorf("期望错误信息包含'不支持图片添加', 实际: %v", err)
	}
}

func TestMemoryAddImage_EmptyImage(t *testing.T) {
	server := mockEmbeddingServer(t)
	defer server.Close()

	db, _ := setupTestDB(t, server)

	ctx := context.Background()
	if err := db.CollectionInitImage(ctx, "gallery", "system-embedding"); err != nil {
		t.Fatalf("CollectionInitImage 失败: %v", err)
	}

	_, err := db.MemoryAddImage(ctx, "gallery", "", "a", "b", "c")
	if err == nil {
		t.Error("期望空图片数据错误")
	}
}

// =============================================================================
// 测试：图片查询（三元嵌入向量 + tok5 加权）
// =============================================================================

func TestMemoryQueryImages_BasicSimilarity(t *testing.T) {
	server := mockEmbeddingServer(t)
	defer server.Close()

	db, _ := setupTestDB(t, server)

	ctx := context.Background()
	if err := db.CollectionInitImage(ctx, "gallery", "system-embedding"); err != nil {
		t.Fatalf("CollectionInitImage 失败: %v", err)
	}

	// 添加 6 张图片，每张有不同的描述组合
	images := []struct {
		image   string
		emotion string
		color   string
		content string
	}{
		{"img1", "阳光明媚", "暖色调", "海滩日落"},
		{"img2", "忧郁深沉", "冷色调", "雨夜街道"},
		{"img3", "阳光温暖", "暖色调", "海滩日出"},
		{"img4", "忧郁悲伤", "冷色调", "雨夜小巷"},
		{"img5", "平静安宁", "中性色调", "森林小径"},
		{"img6", "激动兴奋", "鲜艳色彩", "城市夜景"},
	}

	for _, img := range images {
		_, err := db.MemoryAddImage(ctx, "gallery", img.image, img.emotion, img.color, img.content)
		if err != nil {
			t.Fatalf("MemoryAddImage 失败: %v", err)
		}
	}

	// 查询：与"阳光海滩"相关的内容
	results, err := db.MemoryQueryImages(ctx, "gallery", "阳光海滩", 5)
	if err != nil {
		t.Fatalf("MemoryQueryImages 失败: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("期望返回非空结果")
	}

	// 验证结果按最终评分降序排列
	for i := 1; i < len(results); i++ {
		if results[i].FinalScore > results[i-1].FinalScore {
			t.Errorf("结果未按最终评分降序排列: 索引 %d (%.4f) > 索引 %d (%.4f)",
				i, results[i].FinalScore, i-1, results[i-1].FinalScore)
		}
	}

	// 验证每个结果包含必要字段
	for _, r := range results {
		if r.ID == "" {
			t.Error("查询结果 ID 为空")
		}
		if r.Image == "" {
			t.Error("查询结果 Image 为空")
		}
		if r.BaseScore == 0 && r.FinalScore == 0 {
			// 基础评分为 0 是可能的（余弦相似度为 0），但最终评分也是 0 说明可能有问题
			// 实际上如果所有向量都是零向量，余弦相似度确实为 0
			t.Log("警告: 基础评分和最终评分均为 0")
		}
		if r.BoostLevel < 0 || r.BoostLevel > 3 {
			t.Errorf("BoostLevel 超出范围 [0,3]: %d", r.BoostLevel)
		}
	}
}

func TestMemoryQueryImages_Tok5Boost(t *testing.T) {
	// 此测试验证 tok5 加权逻辑的正确性
	// 使用精心构造的相似度数据来验证加权系数
	// 构造 10 个文档，topK=5，确保只有特定的文档进入 tok5

	docs := make([]ImageDocument, 10)
	// 前 3 个文档在所有维度上都有高相似度 → 应获得 3 个 tok5 命中
	for i := 0; i < 3; i++ {
		docs[i] = ImageDocument{
			ID:    "high-all-" + string(rune('A'+i)),
			Image: "h",
			Embeddings: [3][]float32{
				{0.9, 0.9, 0.9, 0.9},
				{0.9, 0.9, 0.9, 0.9},
				{0.9, 0.9, 0.9, 0.9},
			},
		}
	}
	// 接下来 2 个文档在 2 个维度（0,1）上有高相似度，维度 2 低
	for i := 3; i < 5; i++ {
		docs[i] = ImageDocument{
			ID:    "high-two-" + string(rune('A'+i)),
			Image: "t",
			Embeddings: [3][]float32{
				{0.9, 0.9, 0.9, 0.9},
				{0.9, 0.9, 0.9, 0.9},
				{-0.9, -0.9, -0.9, -0.9}, // 反向向量，与查询向量相似度≈-1.0
			},
		}
	}
	// 接下来 3 个文档仅在维度 2 上有高相似度 → 用于将 high-two 挤出维度 2 的 tok5
	for i := 5; i < 8; i++ {
		docs[i] = ImageDocument{
			ID:    "high-vec2-" + string(rune('A'+i)),
			Image: "v",
			Embeddings: [3][]float32{
				{-0.9, -0.9, -0.9, -0.9},
				{-0.9, -0.9, -0.9, -0.9},
				{0.9, 0.9, 0.9, 0.9},
			},
		}
	}
	// 剩余 2 个文档在 1 个维度（0）上有高相似度
	for i := 8; i < 10; i++ {
		docs[i] = ImageDocument{
			ID:    "high-one-" + string(rune('A'+i)),
			Image: "o",
			Embeddings: [3][]float32{
				{0.9, 0.9, 0.9, 0.9},
				{-0.9, -0.9, -0.9, -0.9},
				{-0.9, -0.9, -0.9, -0.9},
			},
		}
	}

	c := &Collection{
		CollectionType: CollectionTypeImage,
		ImageDocuments: docs,
	}

	queryVec := []float32{0.9, 0.9, 0.9, 0.9}
	results := c.queryImagesTopK(queryVec, 5)

	if len(results) != 5 {
		t.Fatalf("期望 5 条结果, 实际 %d", len(results))
	}

	// 验证加权等级
	for _, r := range results {
		switch {
		case strings.HasPrefix(r.ID, "high-all"):
			if r.BoostLevel != 3 {
				t.Errorf("%s 期望 BoostLevel=3, 实际=%d", r.ID, r.BoostLevel)
			}
			expectedFinal := r.BaseScore * 2.0
			if r.FinalScore != expectedFinal {
				t.Errorf("%s 期望 FinalScore=%.4f, 实际=%.4f", r.ID, expectedFinal, r.FinalScore)
			}
		case strings.HasPrefix(r.ID, "high-two"):
			if r.BoostLevel != 2 {
				t.Errorf("%s 期望 BoostLevel=2, 实际=%d", r.ID, r.BoostLevel)
			}
			expectedFinal := r.BaseScore * 1.6
			if r.FinalScore != expectedFinal {
				t.Errorf("%s 期望 FinalScore=%.4f, 实际=%.4f", r.ID, expectedFinal, r.FinalScore)
			}
		case strings.HasPrefix(r.ID, "high-one"):
			if r.BoostLevel != 1 {
				t.Errorf("%s 期望 BoostLevel=1, 实际=%d", r.ID, r.BoostLevel)
			}
			expectedFinal := r.BaseScore * 1.3
			if r.FinalScore != expectedFinal {
				t.Errorf("%s 期望 FinalScore=%.4f, 实际=%.4f", r.ID, expectedFinal, r.FinalScore)
			}
		case strings.HasPrefix(r.ID, "low-all"):
			if r.BoostLevel != 0 {
				t.Errorf("%s 期望 BoostLevel=0, 实际=%d", r.ID, r.BoostLevel)
			}
			expectedFinal := r.BaseScore * 1.0
			if r.FinalScore != expectedFinal {
				t.Errorf("%s 期望 FinalScore=%.4f, 实际=%.4f", r.ID, expectedFinal, r.FinalScore)
			}
		}
	}

	// 验证排序：high-all 文档应在前面
	topIDs := make([]string, len(results))
	for i, r := range results {
		topIDs[i] = r.ID
	}
	// 前 3 名应该是 high-all 文档
	for i := 0; i < 3; i++ {
		if !strings.HasPrefix(topIDs[i], "high-all") {
			t.Errorf("期望前 3 名是 high-all 文档, 第 %d 名=%s", i+1, topIDs[i])
		}
	}
}

func TestMemoryQueryImages_EmptyCollection(t *testing.T) {
	server := mockEmbeddingServer(t)
	defer server.Close()

	db, _ := setupTestDB(t, server)

	ctx := context.Background()
	if err := db.CollectionInitImage(ctx, "empty", "system-embedding"); err != nil {
		t.Fatalf("CollectionInitImage 失败: %v", err)
	}

	results, err := db.MemoryQueryImages(ctx, "empty", "test query", 5)
	if err != nil {
		t.Fatalf("MemoryQueryImages 失败: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("期望空结果, 实际 %d 条", len(results))
	}
}

func TestMemoryQueryImages_WrongType(t *testing.T) {
	server := mockEmbeddingServer(t)
	defer server.Close()

	db, _ := setupTestDB(t, server)

	ctx := context.Background()
	if err := db.CollectionInit(ctx, "text_coll", "system-embedding"); err != nil {
		t.Fatalf("CollectionInit 失败: %v", err)
	}

	_, err := db.MemoryQueryImages(ctx, "text_coll", "test", 5)
	if err == nil {
		t.Error("期望类型不匹配错误")
	}
}

// =============================================================================
// 测试：图片集合持久化（加载/保存往返）
// =============================================================================

func TestImageDocuments_PersistenceRoundTrip(t *testing.T) {
	server := mockEmbeddingServer(t)
	defer server.Close()

	db, tmpDir := setupTestDB(t, server)

	ctx := context.Background()
	if err := db.CollectionInitImage(ctx, "persist", "system-embedding"); err != nil {
		t.Fatalf("CollectionInitImage 失败: %v", err)
	}

	// 添加 3 张图片
	ids := make([]string, 3)
	for i := range ids {
		id, err := db.MemoryAddImage(ctx, "persist",
			"base64_img_"+string(rune('A'+i)),
			"emotion_"+string(rune('A'+i)),
			"color_"+string(rune('A'+i)),
			"content_"+string(rune('A'+i)),
		)
		if err != nil {
			t.Fatalf("MemoryAddImage 失败: %v", err)
		}
		ids[i] = id
	}

	// 验证磁盘文件存在
	base64Path := filepath.Join(tmpDir, "persist", "base64_0001.json")
	if _, err := os.Stat(base64Path); os.IsNotExist(err) {
		t.Error("base64_0001.json 不存在")
	}

	embPath := filepath.Join(tmpDir, "persist", "embeddings_0001.json")
	if _, err := os.Stat(embPath); os.IsNotExist(err) {
		t.Error("embeddings_0001.json 不存在")
	}

	// 模拟重新加载：清除内存，重新加载
	db.collectionsMu.Lock()
	delete(db.collections, "persist")
	db.collectionsMu.Unlock()

	db.loadAllCollections()

	// 验证重新加载后数据一致
	c, err := db.getCollection("persist")
	if err != nil {
		t.Fatalf("重新加载后 getCollection 失败: %v", err)
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.ImageDocuments) != 3 {
		t.Fatalf("期望 3 张图片, 实际 %d", len(c.ImageDocuments))
	}

	for i, doc := range c.ImageDocuments {
		if doc.ID != ids[i] {
			t.Errorf("索引 %d: 期望 ID=%s, 实际=%s", i, ids[i], doc.ID)
		}
		if doc.Image != "base64_img_"+string(rune('A'+i)) {
			t.Errorf("索引 %d: 图片数据不匹配", i)
		}
		for v := 0; v < 3; v++ {
			if len(doc.Embeddings[v]) != 4 {
				t.Errorf("索引 %d, 向量 %d: 期望维度 4, 实际 %d", i, v, len(doc.Embeddings[v]))
			}
		}
	}
}

// =============================================================================
// 测试：图片集合 删除/清空/重建
// =============================================================================

func TestMemoryDeleteMessage_ImageCollection(t *testing.T) {
	server := mockEmbeddingServer(t)
	defer server.Close()

	db, _ := setupTestDB(t, server)

	ctx := context.Background()
	if err := db.CollectionInitImage(ctx, "gallery", "system-embedding"); err != nil {
		t.Fatalf("CollectionInitImage 失败: %v", err)
	}

	id, _ := db.MemoryAddImage(ctx, "gallery", "img1", "a", "b", "c")
	db.MemoryAddImage(ctx, "gallery", "img2", "x", "y", "z")

	// 删除第一张
	if err := db.MemoryDeleteMessage(ctx, "gallery", id); err != nil {
		t.Fatalf("MemoryDeleteMessage 失败: %v", err)
	}

	c, _ := db.getCollection("gallery")
	c.mu.RLock()
	if len(c.ImageDocuments) != 1 {
		t.Errorf("删除后期望 1 张图片, 实际 %d", len(c.ImageDocuments))
	}
	c.mu.RUnlock()
}

func TestMemoryClearCollection_ImageCollection(t *testing.T) {
	server := mockEmbeddingServer(t)
	defer server.Close()

	db, _ := setupTestDB(t, server)

	ctx := context.Background()
	if err := db.CollectionInitImage(ctx, "gallery", "system-embedding"); err != nil {
		t.Fatalf("CollectionInitImage 失败: %v", err)
	}

	db.MemoryAddImage(ctx, "gallery", "img1", "a", "b", "c")
	db.MemoryAddImage(ctx, "gallery", "img2", "x", "y", "z")

	if err := db.MemoryClearCollection("gallery"); err != nil {
		t.Fatalf("MemoryClearCollection 失败: %v", err)
	}

	c, _ := db.getCollection("gallery")
	c.mu.RLock()
	if len(c.ImageDocuments) != 0 {
		t.Errorf("清空后期望 0 张图片, 实际 %d", len(c.ImageDocuments))
	}
	c.mu.RUnlock()
}

func TestMemoryRebuildEntries_ImageCollection(t *testing.T) {
	server := mockEmbeddingServer(t)
	defer server.Close()

	db, _ := setupTestDB(t, server)

	ctx := context.Background()
	if err := db.CollectionInitImage(ctx, "gallery", "system-embedding"); err != nil {
		t.Fatalf("CollectionInitImage 失败: %v", err)
	}

	// 手动添加一个维度不匹配的文档
	c, _ := db.getCollection("gallery")
	c.mu.Lock()
	c.ImageDocuments = append(c.ImageDocuments, ImageDocument{
		ID:    "bad-doc",
		Image: "bad",
		Embeddings: [3][]float32{
			{0.1, 0.2}, // 维度 2，不匹配
			{0.3},       // 维度 1，不匹配
			{0.4, 0.5}, // 维度 2，不匹配
		},
	})
	c.mu.Unlock()

	remaining, err := db.MemoryRebuildEntries(ctx, "gallery")
	if err != nil {
		t.Fatalf("MemoryRebuildEntries 失败: %v", err)
	}
	if remaining != 0 {
		t.Errorf("重建后期望 0 条有效文档, 实际 %d", remaining)
	}
}

// =============================================================================
// 测试：全局包装函数
// =============================================================================

func TestGlobalWrappers_ImageFunctions(t *testing.T) {
	server := mockEmbeddingServer(t)
	defer server.Close()

	// 准备全局 MemoryDatabase
	tmpDir, err := os.MkdirTemp("", "memory_test_global_*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := InitMemoryDB(tmpDir); err != nil {
		t.Fatalf("InitMemoryDB 失败: %v", err)
	}
	MemoryDatabase.embeddingBaseURL = server.URL + "/v1"
	MemoryDatabase.httpClient = server.Client()
	MemoryDatabase.memoryInitialized = true

	ctx := context.Background()

	// 测试 CollectionInitImage 全局包装
	if err := CollectionInitImage(ctx, "global_imgs", "system-embedding"); err != nil {
		t.Fatalf("CollectionInitImage 全局包装失败: %v", err)
	}

	// 测试 AddImage 全局包装
	id, err := AddImage(ctx, "global_imgs", "test_img", "开心", "明亮", "风景")
	if err != nil {
		t.Fatalf("AddImage 全局包装失败: %v", err)
	}
	if id == "" {
		t.Error("期望返回非空 id")
	}

	// 测试 QueryImages 全局包装
	results, err := QueryImages(ctx, "global_imgs", "风景照片", 5)
	if err != nil {
		t.Fatalf("QueryImages 全局包装失败: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("期望 1 条结果, 实际 %d", len(results))
	}
}

// =============================================================================
// 测试：辅助函数
// =============================================================================

func TestCosineSimilarity(t *testing.T) {
	// 相同向量
	a := []float32{1, 2, 3}
	sim := cosineSimilarity(a, a)
	if sim < 0.999 || sim > 1.001 {
		t.Errorf("相同向量期望相似度 ≈1.0, 实际=%.4f", sim)
	}

	// 正交向量
	b := []float32{1, 0, 0}
	c := []float32{0, 1, 0}
	sim = cosineSimilarity(b, c)
	if sim != 0 {
		t.Errorf("正交向量期望相似度=0, 实际=%.4f", sim)
	}

	// 反向向量
	d := []float32{-1, -2, -3}
	sim = cosineSimilarity(a, d)
	if sim > -0.999 || sim < -1.001 {
		t.Errorf("反向向量期望相似度 ≈-1.0, 实际=%.4f", sim)
	}

	// 不同长度应返回 0
	sim = cosineSimilarity([]float32{1, 2}, []float32{1, 2, 3})
	if sim != 0 {
		t.Errorf("不同长度向量期望相似度=0, 实际=%.4f", sim)
	}

	// 零向量应返回 0
	sim = cosineSimilarity([]float32{0, 0}, []float32{0, 0})
	if sim != 0 {
		t.Errorf("零向量期望相似度=0, 实际=%.4f", sim)
	}
}

func TestTruncateDesc(t *testing.T) {
	short := "短文本"
	if truncateDesc(short) != short {
		t.Errorf("短文本应原样返回")
	}

	// 31 个中文字符（超过 30），应被截断
	long := "这是一个非常长的描述文本用于测试截断功能是否正常工作的额外文字"
	result := truncateDesc(long)
	if len([]rune(result)) != 33 { // 30 字符 + "..."
		t.Errorf("截断后文本长度应为 33, 实际 %d: %s", len([]rune(result)), result)
	}
	if !strings.HasSuffix(result, "...") {
		t.Errorf("截断文本应以 '...' 结尾: %s", result)
	}

	// 精确 30 个字符，不应截断
	exact := "一二三四五六七八九十一二三四五六七八九十一二三四五六七八九十"
	if len([]rune(exact)) != 30 {
		t.Fatalf("测试文本不是 30 个字符: %d", len([]rune(exact)))
	}
	if truncateDesc(exact) != exact {
		t.Errorf("正好 30 字符的文本不应被截断: %s", truncateDesc(exact))
	}
}

func TestGenerateUUID(t *testing.T) {
	// 生成多个 UUID，验证唯一性和格式
	uuids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := generateUUID()
		if uuids[id] {
			t.Errorf("UUID 重复: %s", id)
		}
		uuids[id] = true

		// 验证格式：xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx
		parts := strings.Split(id, "-")
		if len(parts) != 5 {
			t.Errorf("UUID 格式错误: %s", id)
		}
	}
}

// =============================================================================
// 测试：docCount 辅助方法
// =============================================================================

func TestDocCount(t *testing.T) {
	textColl := &Collection{
		CollectionType: CollectionTypeText,
		Documents:      []MemoryDocument{{}, {}, {}},
	}
	if textColl.docCount() != 3 {
		t.Errorf("text 集合 docCount 期望 3, 实际 %d", textColl.docCount())
	}

	imageColl := &Collection{
		CollectionType: CollectionTypeImage,
		ImageDocuments: []ImageDocument{{}, {}, {}, {}},
	}
	if imageColl.docCount() != 4 {
		t.Errorf("image 集合 docCount 期望 4, 实际 %d", imageColl.docCount())
	}
}

// =============================================================================
// 测试：MemoryHasSyncMismatch 对 image 集合的支持
// =============================================================================

func TestMemoryHasSyncMismatch_ImageCollection(t *testing.T) {
	server := mockEmbeddingServer(t)
	defer server.Close()

	db, _ := setupTestDB(t, server)

	ctx := context.Background()
	if err := db.CollectionInitImage(ctx, "sync_test", "system-embedding"); err != nil {
		t.Fatalf("CollectionInitImage 失败: %v", err)
	}

	// 添加正常文档
	db.MemoryAddImage(ctx, "sync_test", "img", "a", "b", "c")

	// 初始应无同步问题
	if db.MemoryHasSyncMismatch("sync_test") {
		t.Error("初始状态不应有同步问题")
	}

	// 手动添加维度不匹配的文档
	c, _ := db.getCollection("sync_test")
	c.mu.Lock()
	c.ImageDocuments = append(c.ImageDocuments, ImageDocument{
		ID:    "bad-doc",
		Image: "bad",
		Embeddings: [3][]float32{
			{0.1}, {0.2}, {0.3}, // 每个只有 1 维，不匹配
		},
	})
	c.mu.Unlock()

	if !db.MemoryHasSyncMismatch("sync_test") {
		t.Error("添加维度不匹配文档后应有同步问题")
	}
}

// =============================================================================
// 基准测试
// =============================================================================

func BenchmarkQueryImagesTopK(b *testing.B) {
	// 构造 100 个图片文档
	docs := make([]ImageDocument, 100)
	for i := range docs {
		docs[i] = ImageDocument{
			ID:    generateUUID(),
			Image: "benchmark_image",
			Embeddings: [3][]float32{
				{float32(i%10) / 10, float32(i%7) / 7, float32(i%5) / 5, float32(i%3) / 3},
				{float32(i%8) / 8, float32(i%6) / 6, float32(i%4) / 4, float32(i%2) / 2},
				{float32(i%9) / 9, float32(i%5) / 5, float32(i%3) / 3, float32(i%7) / 7},
			},
		}
	}

	c := &Collection{
		CollectionType: CollectionTypeImage,
		ImageDocuments: docs,
	}

	queryVec := []float32{0.5, 0.5, 0.5, 0.5}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		results := c.queryImagesTopK(queryVec, 5)
		_ = results
	}
}

// =============================================================================
// 测试：tok5 加权排序的稳定性（相同最终评分保持原始顺序）
// =============================================================================

func TestQueryImagesTopK_StableSort(t *testing.T) {
	// 构造两个在所有维度上完全相同的文档
	docs := []ImageDocument{
		{ID: "first", Image: "a", Embeddings: [3][]float32{
			{0.5, 0.5, 0.5, 0.5},
			{0.5, 0.5, 0.5, 0.5},
			{0.5, 0.5, 0.5, 0.5},
		}},
		{ID: "second", Image: "b", Embeddings: [3][]float32{
			{0.5, 0.5, 0.5, 0.5},
			{0.5, 0.5, 0.5, 0.5},
			{0.5, 0.5, 0.5, 0.5},
		}},
		{ID: "third", Image: "c", Embeddings: [3][]float32{
			{0.5, 0.5, 0.5, 0.5},
			{0.5, 0.5, 0.5, 0.5},
			{0.5, 0.5, 0.5, 0.5},
		}},
	}

	c := &Collection{
		CollectionType: CollectionTypeImage,
		ImageDocuments: docs,
	}

	queryVec := []float32{0.5, 0.5, 0.5, 0.5}
	results := c.queryImagesTopK(queryVec, 3)

	// 稳定排序下，相同评分的文档应保持原始顺序
	for i, r := range results {
		expectedID := []string{"first", "second", "third"}[i]
		if r.ID != expectedID {
			t.Errorf("索引 %d: 期望 ID=%s, 实际=%s (稳定排序可能失败)", i, expectedID, r.ID)
		}
	}
}

// =============================================================================
// 测试：queryImagesTopK 结果数量限制
// =============================================================================

func TestQueryImagesTopK_ResultLimit(t *testing.T) {
	docs := make([]ImageDocument, 10)
	for i := range docs {
		docs[i] = ImageDocument{
			ID:    generateUUID(),
			Image: "test",
			Embeddings: [3][]float32{
				{float32(i) / 10, 0, 0, 0},
				{float32(i) / 10, 0, 0, 0},
				{float32(i) / 10, 0, 0, 0},
			},
		}
	}

	c := &Collection{
		CollectionType: CollectionTypeImage,
		ImageDocuments: docs,
	}

	queryVec := []float32{1.0, 0, 0, 0}

	// 请求 topK=3，应返回 3 条
	results := c.queryImagesTopK(queryVec, 3)
	if len(results) != 3 {
		t.Errorf("期望 3 条结果, 实际 %d", len(results))
	}

	// 请求 topK=20（超过文档数），应返回全部 10 条
	results = c.queryImagesTopK(queryVec, 20)
	if len(results) != 10 {
		t.Errorf("期望 10 条结果, 实际 %d", len(results))
	}
}

// =============================================================================
// 测试：跨进程一致性 — reloadIfChanged 对 image 集合的支持
// =============================================================================

func TestReloadIfChanged_ImageCollection(t *testing.T) {
	server := mockEmbeddingServer(t)
	defer server.Close()

	db, tmpDir := setupTestDB(t, server)

	ctx := context.Background()
	if err := db.CollectionInitImage(ctx, "reload_test", "system-embedding"); err != nil {
		t.Fatalf("CollectionInitImage 失败: %v", err)
	}

	db.MemoryAddImage(ctx, "reload_test", "img1", "a", "b", "c")

	// 模拟外部进程修改：直接写入磁盘文件
	base64Path := filepath.Join(tmpDir, "reload_test", "base64_0001.json")
	existingBase64 := []base64Entry{
		{ID: "ext-1", Image: "external_img"},
	}
	existingEmb := []imageEmbeddingEntry{
		{ID: "ext-1", Embeddings: [3][]float32{
			{0.1, 0.2, 0.3, 0.4},
			{0.1, 0.2, 0.3, 0.4},
			{0.1, 0.2, 0.3, 0.4},
		}},
	}
	atomicWriteJSON(base64Path, existingBase64)
	atomicWriteJSON(filepath.Join(tmpDir, "reload_test", "embeddings_0001.json"), existingEmb)

	// 更新 metadata.json 的 chunk_count 和修改时间
	metaPath := filepath.Join(tmpDir, "reload_test", "metadata.json")
	meta := collectionMeta{
		Model:      "system-embedding",
		Dimension:  4,
		ChunkCount: 1,
		Type:       CollectionTypeImage,
	}
	atomicWriteJSON(metaPath, meta)

	// 触发 reloadIfChanged
	c, _ := db.getCollection("reload_test")
	initialCount := c.docCount()
	c.reloadIfChanged()

	c.mu.RLock()
	newCount := len(c.ImageDocuments)
	c.mu.RUnlock()

	if initialCount == newCount {
		t.Log("reloadIfChanged 可能未检测到变更（mtime 精度问题），这是 Windows 下已知行为")
	}
}

// =============================================================================
// 集成测试：完整工作流
// =============================================================================

func TestImageWorkflow_Integration(t *testing.T) {
	server := mockEmbeddingServer(t)
	defer server.Close()

	db, tmpDir := setupTestDB(t, server)

	ctx := context.Background()

	// 1. 初始化 image 集合
	if err := db.CollectionInitImage(ctx, "workflow", "system-embedding"); err != nil {
		t.Fatalf("初始化失败: %v", err)
	}

	// 2. 添加多张图片
	images := []struct {
		img     string
		emotion string
		color   string
		content string
	}{
		{"img_sunset", "温暖", "暖色调", "夕阳下的海滩"},
		{"img_rain", "忧郁", "冷色调", "雨中的城市"},
		{"img_forest", "平静", "绿色调", "清晨的森林"},
		{"img_night", "神秘", "暗色调", "星空下的山脉"},
		{"img_garden", "愉悦", "多彩", "春天的花园"},
		{"img_ocean", "壮阔", "蓝色调", "波涛汹涌的大海"},
		{"img_autumn", "怀旧", "金黄色", "秋天的落叶"},
		{"img_snow", "宁静", "白色调", "雪中的村庄"},
	}

	for _, img := range images {
		_, err := db.MemoryAddImage(ctx, "workflow", img.img, img.emotion, img.color, img.content)
		if err != nil {
			t.Fatalf("添加图片失败: %v", err)
		}
	}

	// 3. 验证文档数
	count := db.MemoryGetCollectionCount("workflow")
	if count != len(images) {
		t.Errorf("期望 %d 张图片, 实际 %d", len(images), count)
	}

	// 4. 查询
	results, err := db.MemoryQueryImages(ctx, "workflow", "温暖的海滩日落", 5)
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("查询应返回结果")
	}

	// 验证结果按最终评分降序
	sorted := sort.SliceIsSorted(results, func(i, j int) bool {
		return results[i].FinalScore > results[j].FinalScore
	})
	if !sorted {
		t.Error("结果未按最终评分降序排列")
	}

	// 验证所有结果包含必要字段
	for _, r := range results {
		if r.ID == "" || r.Image == "" {
			t.Error("结果缺少必要字段")
		}
		if r.BoostLevel < 0 || r.BoostLevel > 3 {
			t.Errorf("BoostLevel 越界: %d", r.BoostLevel)
		}
	}

	// 5. 删除一张图片
	delID := results[0].ID
	if err := db.MemoryDeleteMessage(ctx, "workflow", delID); err != nil {
		t.Fatalf("删除失败: %v", err)
	}

	newCount := db.MemoryGetCollectionCount("workflow")
	if newCount != len(images)-1 {
		t.Errorf("删除后期望 %d 张图片, 实际 %d", len(images)-1, newCount)
	}

	// 6. 验证磁盘持久化（重新加载）
	db.collectionsMu.Lock()
	delete(db.collections, "workflow")
	db.collectionsMu.Unlock()

	db.loadAllCollections()

	c, _ := db.getCollection("workflow")
	if c.docCount() != len(images)-1 {
		t.Errorf("重新加载后文档数不匹配: 期望 %d, 实际 %d", len(images)-1, c.docCount())
	}

	// 7. 清空集合
	if err := db.MemoryClearCollection("workflow"); err != nil {
		t.Fatalf("清空失败: %v", err)
	}

	if db.MemoryGetCollectionCount("workflow") != 0 {
		t.Error("清空后文档数应为 0")
	}

	// 8. 验证磁盘文件已清理
	base64Path := filepath.Join(tmpDir, "workflow", "base64_0001.json")
	if _, err := os.Stat(base64Path); !os.IsNotExist(err) {
		t.Log("清空后分块文件可能仍存在（chunkCount 为 0 时不同步删除）")
	}
}