package memory

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
)

// TestInitAndEnsureReady 在临时目录初始化全局记忆库：
// 空库加载 store-all（总库，必需）；写入后条目进总库，正式簇仅在迁移/重建时升级产生。
func TestInitAndEnsureReady(t *testing.T) {
	server := startFakeEmbeddingServer(t)
	defer server.Close()

	isolateStorage(t)
	origURL, origModel := config.AgentEmbeddingURL, config.AgentEmbeddingModel
	config.AgentEmbeddingURL = server.URL + "/v1"
	config.AgentEmbeddingModel = "test-embedding"
	t.Cleanup(func() {
		config.AgentEmbeddingURL, config.AgentEmbeddingModel = origURL, origModel
	})

	store := Init(filepath.Join(t.TempDir(), "memory"))
	if store == nil {
		t.Fatal("Init 应返回记忆库实例")
	}
	if err := EnsureReady(); err != nil {
		t.Fatalf("EnsureReady 初始化失败: %v", err)
	}

	// 空库加载 store-all（总库是必需的），正式簇需 ≥16 条才升级
	names := Collections()
	if len(names) != 1 || names[0] != StoreCollectionID {
		t.Fatalf("空库应只加载 store-all（总库），实得 %v", names)
	}

	id, err := store.AddText(context.Background(), "user", "今天天气不错")
	if err != nil {
		t.Fatalf("写入记忆失败: %v", err)
	}
	entry, err := store.Get(id)
	if err != nil {
		t.Fatalf("读取记忆失败: %v", err)
	}
	if entry.Collection != StoreCollectionID {
		t.Fatalf("条目应归入总库 store-all，实得 %q", entry.Collection)
	}
	if got := store.CollectionCount(); got != 1 {
		t.Fatalf("写入一条记忆后仍只有总库，实得 %d", got)
	}

	dir := filepath.Join(store.baseDir, clusterDirName(entry.Collection))
	if _, err := os.Stat(filepath.Join(dir, "documents_0001.json")); err != nil {
		t.Fatalf("总库目录分片未落盘: %v", err)
	}
}
