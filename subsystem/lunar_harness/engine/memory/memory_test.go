package memory

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	base := t.TempDir()
	textID := "text-1"
	imageID := "image-1"
	audioID := "audio-1"
	legacyID := "legacy-image"
	collDir := filepath.Join(base, "日常闲聊")
	return &Store{
		baseDir: base,
		collections: map[string]*Collection{
			"日常闲聊": {
				Name:      "日常闲聊",
				Dimension: 2,
				collDir:   collDir,
				metaPath:  filepath.Join(collDir, "metadata.json"),
				Documents: []Document{
					{ID: textID, Type: TypeText, Role: "user", Content: "你好，今天天气不错", Timestamp: 100},
					{ID: imageID, Type: TypeImage, URL: "/file/read/multimedia/sticker.png", Timestamp: 200},
					{ID: audioID, Type: TypeAudio, Content: "一段笑声", URL: "/file/read/audios/laugh.mp3", Timestamp: 300},
					// 旧数据没有 type 字段：按 URL 推断为 image
					{ID: legacyID, URL: "/file/read/multimedia/old.png", Timestamp: 50},
				},
				NameEmbedding:      []float32{1, 0},
				NameEmbeddingModel: "test-embedding",
			},
		},
	}
}

// TestUpdateTextEntry 原地更新：ID 不变、正文与嵌入向量一起更新。
func TestUpdateTextEntry(t *testing.T) {
	server := scopeEmbeddingServer(t)
	defer server.Close()
	origURL, origModel := config.AgentEmbeddingURL, config.AgentEmbeddingModel
	config.AgentEmbeddingURL = server.URL + "/v1"
	config.AgentEmbeddingModel = "test-embedding"
	t.Cleanup(func() {
		config.AgentEmbeddingURL, config.AgentEmbeddingModel = origURL, origModel
	})

	d := testStore(t)
	d.initialized = true
	d.httpClient = server.Client()

	if err := d.Update(context.Background(), "text-1", "更新后的正文"); err != nil {
		t.Fatalf("更新失败: %v", err)
	}
	entry, err := d.Get("text-1")
	if err != nil || entry == nil || entry.Content != "更新后的正文" {
		t.Fatalf("更新后应读到新正文，实得 %+v err=%v", entry, err)
	}
	if err := d.Update(context.Background(), "missing", "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("不存在的条目应返回 ErrNotFound，实得 %v", err)
	}
}

// TestListBySource 来源过滤只取回同一写入来源的条目。
func TestListBySource(t *testing.T) {
	d := testStore(t)
	coll := d.collections["日常闲聊"]
	coll.Documents[0].Source = "bilibili_viewer"
	coll.Documents = append(coll.Documents, Document{
		ID: "text-2", Type: TypeText, Role: "assistant", Content: "看了《A》。",
		Source: "bilibili_viewer", Timestamp: 400,
	})

	entries := d.ListBySource(TypeText, "bilibili_viewer", 1)
	if len(entries) != 1 || entries[0].ID != "text-2" {
		t.Fatalf("应按来源过滤并按时间倒序，实得 %+v", entries)
	}
	if got := d.ListBySource(TypeText, "", 10); len(got) != 0 {
		t.Fatalf("空来源应返回空，实得 %+v", got)
	}
}

func TestNormalizeType(t *testing.T) {
	cases := map[string]string{
		"":      TypeText,
		"text":  TypeText,
		"IMAGE": TypeImage,
		"audio": TypeAudio,
	}
	for input, want := range cases {
		got, err := NormalizeType(input)
		if err != nil {
			t.Fatalf("NormalizeType(%q) 返回错误: %v", input, err)
		}
		if got != want {
			t.Fatalf("NormalizeType(%q) = %q, want %q", input, got, want)
		}
	}
	if _, err := NormalizeType("video"); err == nil {
		t.Fatal("非法类型应返回错误")
	}
}

func TestDocumentTypeInfersLegacy(t *testing.T) {
	if got := documentType(Document{URL: "/file/read/multimedia/a.png"}); got != TypeImage {
		t.Fatalf("旧图片数据应推断为 image，实得 %q", got)
	}
	if got := documentType(Document{URL: "/file/read/audios/a.mp3"}); got != TypeAudio {
		t.Fatalf("旧音频数据应推断为 audio，实得 %q", got)
	}
	if got := documentType(Document{Content: "文本"}); got != TypeText {
		t.Fatalf("纯文本数据应推断为 text，实得 %q", got)
	}
	if got := documentType(Document{Type: TypeAudio, URL: "/a.mp3"}); got != TypeAudio {
		t.Fatalf("显式 audio 应保持 audio，实得 %q", got)
	}
}

func TestListFiltersTypeAndCounts(t *testing.T) {
	d := testStore(t)

	all, total := d.List("", 0, 0)
	if total != 4 || len(all) != 4 {
		t.Fatalf("全部条目应为 4，实得 total=%d len=%d", total, len(all))
	}
	// 按入库时间倒序
	if all[0].ID != "audio-1" || all[1].ID != "image-1" {
		t.Fatalf("条目应按时间倒序，实得 %q, %q", all[0].ID, all[1].ID)
	}
	if all[0].Collection != "日常闲聊" || all[0].Type != TypeAudio {
		t.Fatalf("条目缺少类型/集合信息: %+v", all[0])
	}

	images, imageTotal := d.List(TypeImage, 0, 0)
	if imageTotal != 2 {
		t.Fatalf("图片条目应为 2（含旧数据推断），实得 %d", imageTotal)
	}
	for _, entry := range images {
		if entry.Type != TypeImage {
			t.Fatalf("图片过滤混入其它类型: %+v", entry)
		}
	}

	texts, textTotal := d.List(TypeText, 0, 1)
	if textTotal != 1 || len(texts) != 1 || texts[0].Content == "" {
		t.Fatalf("文本条目应为 1 且带正文，实得 total=%d %+v", textTotal, texts)
	}

	if got := d.Count(TypeAudio); got != 1 {
		t.Fatalf("音频条目数应为 1，实得 %d", got)
	}
}

func TestGetAndDelete(t *testing.T) {
	d := testStore(t)
	entry, err := d.Get("text-1")
	if err != nil {
		t.Fatalf("按 ID 读取失败: %v", err)
	}
	if entry.Content == "" || entry.Type != TypeText {
		t.Fatalf("读取结果不正确: %+v", entry)
	}

	if _, err := d.Get("missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("缺失 ID 应返回 ErrNotFound，实得 %v", err)
	}

	if err := d.Delete(context.Background(), "text-1"); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	if _, err := d.Get("text-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("删除后仍能读到条目: %v", err)
	}
}

func TestKeywordSearchAcrossCollections(t *testing.T) {
	d := testStore(t)
	results, err := d.KeywordSearch(TypeText, []string{"天气"}, false, 5)
	if err != nil {
		t.Fatalf("关键词检索失败: %v", err)
	}
	if len(results) != 1 || results[0].Content != "你好，今天天气不错" {
		t.Fatalf("关键词检索结果不正确: %+v", results)
	}
	// 类型不匹配时不应返回
	empty, err := d.KeywordSearch(TypeAudio, []string{"天气"}, false, 5)
	if err != nil || len(empty) != 0 {
		t.Fatalf("按 audio 类型检索文本应返回空，实得 %+v, %v", empty, err)
	}
}

// TestListWithErrorDistinguishesUnavailableFromEmpty 记忆库不可用时必须报错，
// 已初始化且确实没有条目时才返回空列表 —— 迁移不能把「读不到」当成「没有」。
func TestListWithErrorDistinguishesUnavailableFromEmpty(t *testing.T) {
	// 1. 未初始化：返回错误而不是空列表。
	unready := &Store{baseDir: t.TempDir(), collections: map[string]*Collection{}}
	if _, _, err := unready.ListWithError("text", 0, 10); err == nil {
		t.Fatal("未初始化的记忆库应返回错误")
	}

	// 2. 启动加载失败：错误必须能传出，而不是静默返回空列表。
	failed := &Store{baseDir: t.TempDir(), initialized: true, collections: map[string]*Collection{}}
	storeDir := filepath.Join(failed.baseDir, StoreCollectionID)
	if err := os.MkdirAll(storeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storeDir, "metadata.json"), []byte("{ 坏掉的 JSON"), 0o644); err != nil {
		t.Fatal(err)
	}
	failed.loadSingleCollection(StoreCollectionID, "store")
	_, _, err := failed.ListWithError("text", 0, 10)
	if err == nil || !strings.Contains(err.Error(), "加载失败") {
		t.Fatalf("加载失败应返回可识别错误，实际 %v", err)
	}

	// 3. 已初始化且确实没有条目：空列表 + 无错误。
	empty := &Store{initialized: true, collections: map[string]*Collection{}}
	entries, total, err := empty.ListWithError("text", 0, 10)
	if err != nil || len(entries) != 0 || total != 0 {
		t.Fatalf("空记忆库应返回空列表且无错误，实际 entries=%v total=%d err=%v", entries, total, err)
	}
}
