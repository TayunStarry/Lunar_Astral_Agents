package memory

import (
	"path/filepath"
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
)

// TestInitAndEnsureReady 在临时目录初始化全局记忆库：默认分组应全部就绪并可列出。
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

	names := Collections()
	if len(names) < len(DefaultCollections) {
		t.Fatalf("默认分组未全部创建: %d < %d", len(names), len(DefaultCollections))
	}
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		seen[name] = true
	}
	for _, want := range DefaultCollections {
		if !seen[want] {
			t.Fatalf("缺少默认分组: %s", want)
		}
	}
}
