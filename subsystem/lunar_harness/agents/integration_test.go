package agents

// integration_test.go 跨智能体集成测试：归档生命周期 + 文献格式识别 + 能力边界。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/agents/ArchiveOps"
	"LunarSubsystem/LunarHarness/agents/ArchiveReader"
	"LunarSubsystem/LunarHarness/agents/DocumentReader"
	"LunarSubsystem/LunarHarness/agents/Vision"
	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/resource"
	"LunarSubsystem/LunarHarness/protocol/agentinfo"
)

// prepareIntegrationDir 建立临时 LocalDir 与 documents 内容。
func prepareIntegrationDir(t *testing.T) {
	t.Helper()
	oldDir := config.LocalDir
	config.LocalDir = t.TempDir()
	t.Cleanup(func() { config.LocalDir = oldDir })
	dir := filepath.Join(config.LocalDir, "documents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("alpha"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "big.txt"), []byte(strings.Repeat("文献内容。", 5000)), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestCrossAgentArchiveFlow 归档操作 → 归档解析 → 句柄取用 的完整链路。
func TestCrossAgentArchiveFlow(t *testing.T) {
	prepareIntegrationDir(t)

	created := ArchiveOps.Create("/document/pkg.zip", []string{"/document/a.txt"})
	if !created.OK {
		t.Fatalf("创建压缩包失败: %s", created.Summary)
	}
	entries, err := ArchiveReader.ListEntries(created.Path)
	if err != nil || len(entries) != 1 {
		t.Fatalf("结构查询失败: %v / %d 条", err, len(entries))
	}
	text, err := ArchiveReader.ReadEntry(created.Path, "a.txt", 0)
	if err != nil || !strings.Contains(text, "alpha") {
		t.Fatalf("条目读取失败: %v / %q", err, text)
	}
	inspected, err := ArchiveReader.Inspect(created.Path, "")
	if err != nil || !strings.Contains(inspected, "a.txt") {
		t.Fatalf("确定性摘要失败: %v / %s", err, inspected)
	}
	// 归档操作复用归档解析的读取管线
	read, err := ArchiveOps.Read(created.Path, "a.txt", 0)
	if err != nil || !strings.Contains(read, "alpha") {
		t.Fatalf("归档操作读取管线失败: %v / %q", err, read)
	}
}

// TestCrossAgentBoundaries 各智能体只处理自己声明的能力边界。
func TestCrossAgentBoundaries(t *testing.T) {
	prepareIntegrationDir(t)

	if _, _, err := ArchiveReader.ResolveArchivePath("/document/a.txt"); err == nil {
		t.Fatal("归档解析不应接受普通文本")
	}
	if result := ArchiveOps.Create("/document/a.txt", []string{"/document/big.txt"}); result.OK {
		t.Fatal("归档操作不应把普通文件当压缩包")
	}
	if _, err := DocumentReader.ProfileOf("@D.999"); err == nil {
		t.Fatal("文献解析不应解析不存在的句柄")
	}
}

// TestDocumentReaderProfileIntegration 文献解析对真实句柄自动识别格式与分批策略。
func TestDocumentReaderProfileIntegration(t *testing.T) {
	prepareIntegrationDir(t)

	_, handle, err := resource.RegisterPath("documents/big.txt")
	if err != nil || handle == "" {
		t.Fatalf("登记句柄失败: %v / %q", err, handle)
	}
	profile, err := DocumentReader.ProfileOf(handle)
	if err != nil {
		t.Fatalf("格式识别失败: %v", err)
	}
	if !strings.Contains(profile, "分批") || !strings.Contains(profile, "25000 字") {
		t.Fatalf("大文件应识别为分批处理并给出字数: %s", profile)
	}
}

// TestAgentCatalogsAreDisjoint 工具目录必须体现互不重叠的能力边界。
func TestAgentCatalogsAreDisjoint(t *testing.T) {
	catalogs := map[string][]string{
		"vision":          toolsOf(Vision.ToolCatalog()),
		"document_reader": toolsOf(DocumentReader.ToolCatalog()),
		"archive_reader":  toolsOf(ArchiveReader.ToolCatalog()),
		"archive_ops":     toolsOf(ArchiveOps.ToolCatalog()),
	}
	for agent, tools := range catalogs {
		if len(tools) == 0 {
			t.Fatalf("智能体 %s 没有工具", agent)
		}
	}
	seen := map[string]string{}
	// 归档操作按需求 #3 明确可复用归档解析的读取管线，这两个工具名允许重叠。
	shared := map[string]bool{"archive_list": true, "archive_read": true}
	for agent, tools := range catalogs {
		for _, tool := range tools {
			if owner, ok := seen[tool]; ok {
				if !shared[tool] {
					t.Fatalf("工具 %s 同时属于 %s 与 %s", tool, owner, agent)
				}
				pair := owner + "+" + agent
				if pair != "archive_reader+archive_ops" && pair != "archive_ops+archive_reader" {
					t.Fatalf("工具 %s 的共享方非法: %s", tool, pair)
				}
				continue
			}
			seen[tool] = agent
		}
	}
	// 视觉理解不得持有文件 / 归档工具
	for _, tool := range catalogs["vision"] {
		if strings.Contains(tool, "file_") || strings.Contains(tool, "archive_") || tool == "gguf_metadata" {
			t.Fatalf("视觉理解越界持有 %s", tool)
		}
	}
}

// toolsOf 展开工具目录中的工具名。
func toolsOf(catalog []agentinfo.Agent) []string {
	names := make([]string, 0, len(catalog))
	for _, agent := range catalog {
		for _, tool := range agent.Tools {
			names = append(names, tool.Name)
		}
	}
	return names
}
