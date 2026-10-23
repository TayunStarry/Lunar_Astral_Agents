package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
)

func testMemoryDB(t *testing.T) *MemoryDB {
	t.Helper()
	base := t.TempDir()
	textID := "text-1"
	imageID := "image-1"
	audioID := "audio-1"
	legacyID := "legacy-image"
	collDir := filepath.Join(base, "日常闲聊")
	return &MemoryDB{
		baseDir: base,
		collections: map[string]*Collection{
			"日常闲聊": {
				Name:      "日常闲聊",
				Model:     "test-embedding",
				Dimension: 2,
				collDir:   collDir,
				metaPath:  filepath.Join(collDir, "metadata.json"),
				Documents: []Document{
					{ID: textID, Type: MemoryTypeText, Role: "user", Content: "你好，今天天气不错", Timestamp: 100},
					{ID: imageID, Type: MemoryTypeImage, URL: "/file/read/multimedia/sticker.png", Timestamp: 200},
					{ID: audioID, Type: MemoryTypeAudio, Content: "一段笑声", URL: "/file/read/audios/laugh.mp3", Timestamp: 300},
					// 旧数据没有 type 字段：按 URL 推断为 image
					{ID: legacyID, URL: "/file/read/multimedia/old.png", Timestamp: 50},
				},
				NameEmbedding:      []float32{1, 0},
				NameEmbeddingModel: "test-embedding",
			},
		},
	}
}

func TestNormalizeMemoryType(t *testing.T) {
	cases := map[string]string{
		"":      MemoryTypeText,
		"text":  MemoryTypeText,
		"IMAGE": MemoryTypeImage,
		"audio": MemoryTypeAudio,
	}
	for input, want := range cases {
		got, err := NormalizeMemoryType(input)
		if err != nil {
			t.Fatalf("NormalizeMemoryType(%q) 返回错误: %v", input, err)
		}
		if got != want {
			t.Fatalf("NormalizeMemoryType(%q) = %q, want %q", input, got, want)
		}
	}
	if _, err := NormalizeMemoryType("video"); err == nil {
		t.Fatal("非法类型应返回错误")
	}
}

func TestMemoryDocumentTypeInfersLegacy(t *testing.T) {
	if got := memoryDocumentType(Document{URL: "/file/read/multimedia/a.png"}); got != MemoryTypeImage {
		t.Fatalf("旧图片数据应推断为 image，实得 %q", got)
	}
	if got := memoryDocumentType(Document{URL: "/file/read/audios/a.mp3"}); got != MemoryTypeAudio {
		t.Fatalf("旧音频数据应推断为 audio，实得 %q", got)
	}
	if got := memoryDocumentType(Document{Content: "文本"}); got != MemoryTypeText {
		t.Fatalf("纯文本数据应推断为 text，实得 %q", got)
	}
	if got := memoryDocumentType(Document{Type: MemoryTypeAudio, URL: "/a.mp3"}); got != MemoryTypeAudio {
		t.Fatalf("显式 audio 应保持 audio，实得 %q", got)
	}
}

func TestMemoryListEntriesFiltersTypeAndCounts(t *testing.T) {
	d := testMemoryDB(t)

	all, total := d.MemoryListEntries("", 0, 0)
	if total != 4 || len(all) != 4 {
		t.Fatalf("全部条目应为 4，实得 total=%d len=%d", total, len(all))
	}
	// 按入库时间倒序
	if all[0].ID != "audio-1" || all[1].ID != "image-1" {
		t.Fatalf("条目应按时间倒序，实得 %q, %q", all[0].ID, all[1].ID)
	}
	if all[0].Collection != "日常闲聊" || all[0].Type != MemoryTypeAudio {
		t.Fatalf("条目缺少类型/集合信息: %+v", all[0])
	}

	images, imageTotal := d.MemoryListEntries(MemoryTypeImage, 0, 0)
	if imageTotal != 2 {
		t.Fatalf("图片条目应为 2（含旧数据推断），实得 %d", imageTotal)
	}
	for _, entry := range images {
		if entry.Type != MemoryTypeImage {
			t.Fatalf("图片过滤混入其它类型: %+v", entry)
		}
	}

	texts, textTotal := d.MemoryListEntries(MemoryTypeText, 0, 1)
	if textTotal != 1 || len(texts) != 1 || texts[0].Content == "" {
		t.Fatalf("文本条目应为 1 且带正文，实得 total=%d %+v", textTotal, texts)
	}

	if got := d.MemoryCountEntries(MemoryTypeAudio); got != 1 {
		t.Fatalf("音频条目数应为 1，实得 %d", got)
	}
}

func TestMemoryGetEntryAndDelete(t *testing.T) {
	d := testMemoryDB(t)
	entry, err := d.MemoryGetEntry("text-1")
	if err != nil {
		t.Fatalf("按 ID 读取失败: %v", err)
	}
	if entry.Content == "" || entry.Type != MemoryTypeText {
		t.Fatalf("读取结果不正确: %+v", entry)
	}

	if _, err := d.MemoryGetEntry("missing"); !errors.Is(err, ErrMemoryEntryNotFound) {
		t.Fatalf("缺失 ID 应返回 ErrMemoryEntryNotFound，实得 %v", err)
	}

	if err := d.MemoryDeleteMessage(context.Background(), "text-1"); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	if _, err := d.MemoryGetEntry("text-1"); !errors.Is(err, ErrMemoryEntryNotFound) {
		t.Fatalf("删除后仍能读到条目: %v", err)
	}
}

func TestMemoryKeywordSearchAcrossCollections(t *testing.T) {
	d := testMemoryDB(t)
	results, err := d.MemoryKeywordSearch(MemoryTypeText, []string{"天气"}, false, 5)
	if err != nil {
		t.Fatalf("关键词检索失败: %v", err)
	}
	if len(results) != 1 || results[0].Content != "你好，今天天气不错" {
		t.Fatalf("关键词检索结果不正确: %+v", results)
	}
	// 类型不匹配时不应返回
	empty, err := d.MemoryKeywordSearch(MemoryTypeAudio, []string{"天气"}, false, 5)
	if err != nil || len(empty) != 0 {
		t.Fatalf("按 audio 类型检索文本应返回空，实得 %+v, %v", empty, err)
	}
}

func TestDefaultCollectionsCoverage(t *testing.T) {
	if len(DefaultMemoryCollections) < 10 {
		t.Fatalf("默认分组过少: %d", len(DefaultMemoryCollections))
	}
	seen := map[string]bool{}
	for _, name := range DefaultMemoryCollections {
		if name == "" || seen[name] {
			t.Fatalf("默认分组名重复或为空: %q", name)
		}
		seen[name] = true
	}
}

// TestMigrateLegacyMemoryImages 旧版 multimedia/<分组>/ 图片应在加载时迁移到
// multimedia/memory/<分组>/，同时改写文档 URL。
func TestMigrateLegacyMemoryImages(t *testing.T) {
	isolateStorage(t)
	base := filepath.Clean(config.LocalDir)
	const collection = "测试分组"
	legacyRel := path.Join(StorageRootMultimedia, collection, "legacy.png")
	legacyAbs := filepath.Join(base, filepath.FromSlash(legacyRel))
	if err := os.MkdirAll(filepath.Dir(legacyAbs), 0o755); err != nil {
		t.Fatalf("准备旧目录失败: %v", err)
	}
	if err := os.WriteFile(legacyAbs, []byte("png-bytes"), 0o644); err != nil {
		t.Fatalf("准备旧图片失败: %v", err)
	}

	c := &Collection{
		Name: collection,
		Documents: []Document{{
			ID:   "legacy-image",
			Type: MemoryTypeImage,
			URL:  "/file/read/" + legacyRel,
		}},
	}
	if !c.migrateLegacyMemoryImages() {
		t.Fatal("旧版记忆图片应被迁移")
	}
	wantRel := path.Join(StorageRootMultimedia, MemoryMediaSubdir, collection, "legacy.png")
	if got := c.Documents[0].URL; got != "/file/read/"+wantRel {
		t.Fatalf("迁移后 URL 不正确: %s，期望 /file/read/%s", got, wantRel)
	}
	newAbs := filepath.Join(base, filepath.FromSlash(wantRel))
	if _, err := os.Stat(newAbs); err != nil {
		t.Fatalf("迁移后的图片不存在: %v", err)
	}
	if _, err := os.Stat(legacyAbs); !os.IsNotExist(err) {
		t.Fatalf("旧版图片应已移走: %v", err)
	}
}

// TestTagRequestDisablesThinking 标签生成必须显式关闭推理：
// Gemma-4 等模型会先输出很长的 reasoning_content，max_tokens 被耗尽后 content 为空，
// 表现为「LLM 返回空内容」并触发三次无意义重试（动态图标签尤其明显）。
func TestTagRequestDisablesThinking(t *testing.T) {
	var got chatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("解析标签请求失败: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"[\"猫\",\"表情\"]"}}]}`))
	}))
	defer server.Close()

	origURL := config.AgentMultimodalURL
	origModel := config.AgentMultimodalModel
	config.AgentMultimodalURL = server.URL
	config.AgentMultimodalModel = "test-multimodal"
	t.Cleanup(func() {
		config.AgentMultimodalURL = origURL
		config.AgentMultimodalModel = origModel
	})

	d := &MemoryDB{httpClient: server.Client()}
	tags, err := d.generateTags(context.Background(), "data:image/png;base64,AAAA", true, "auto", "")
	if err != nil {
		t.Fatalf("标签生成失败: %v", err)
	}
	if len(tags) != 2 || tags[0] != "猫" {
		t.Fatalf("标签结果不正确: %v", tags)
	}
	if enabled, ok := got.ChatTemplateKwargs["enable_thinking"]; !ok || enabled != false {
		t.Fatalf("标签请求未关闭思考: %+v", got.ChatTemplateKwargs)
	}
	if got.MaxTokens < 512 {
		t.Fatalf("标签请求 max_tokens 过小，推理可能耗尽输出: %d", got.MaxTokens)
	}
}
