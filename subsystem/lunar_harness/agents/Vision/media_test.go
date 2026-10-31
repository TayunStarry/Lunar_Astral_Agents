package Vision

// media_test.go 视觉理解智能体的单元测试：本地视频副本复用与工具边界。

import (
	"os"
	"path/filepath"
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
)

func TestEnsureLocalVideoPathReusesLocalCopy(t *testing.T) {
	oldDir := config.LocalDir
	config.LocalDir = t.TempDir()
	t.Cleanup(func() { config.LocalDir = oldDir })

	name := "33bfb0e20c938527.mp4"
	abs := filepath.Join(config.LocalDir, "multimedia", name)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte("fake-video"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 远程 CDN 地址已失效时，应优先复用本地同名副本，而不是直接报 404。
	got, err := ensureLocalVideoPath("https://cdn.example.com/"+name, "")
	if err != nil {
		t.Fatal(err)
	}
	if got != abs {
		t.Fatalf("应复用本地副本 %s，实际 %s", abs, got)
	}

	// 句柄标题同样可作为回退依据。
	got, err = ensureLocalVideoPath("https://cdn.example.com/expired", name)
	if err != nil {
		t.Fatal(err)
	}
	if got != abs {
		t.Fatalf("标题回退失败，实际 %s", got)
	}

	// 本机 URL 指向不存在的规范化路径时，也要回退到同名副本。
	missing := "http://127.0.0.1:36789/file/read/multimedia/video_missing.mp4"
	got, err = ensureLocalVideoPath(missing, name)
	if err != nil {
		t.Fatal(err)
	}
	if got != abs {
		t.Fatalf("缺失本机路径应回退到同名副本，实际 %s", got)
	}
}

// TestToolCatalogVisionOnly 视觉理解只能暴露视觉链路工具，不能混入文件 / 归档能力。
func TestToolCatalogVisionOnly(t *testing.T) {
	agents := ToolCatalog()
	if len(agents) != 1 {
		t.Fatalf("应只有一个视觉理解智能体，实际 %d", len(agents))
	}
	agent := agents[0]
	if agent.ID != "vision" {
		t.Fatalf("智能体标识错误: %s", agent.ID)
	}
	names := map[string]bool{}
	for _, tool := range agent.Tools {
		names[tool.Name] = true
	}
	for _, want := range []string{ToolDescribe, ToolAsk, ToolWatch} {
		if !names[want] {
			t.Fatalf("缺少视觉工具 %s", want)
		}
	}
	for _, forbidden := range []string{"file_read", "archive_create", "gguf_metadata"} {
		if names[forbidden] {
			t.Fatalf("视觉理解不应持有非视觉工具 %s", forbidden)
		}
	}
}
