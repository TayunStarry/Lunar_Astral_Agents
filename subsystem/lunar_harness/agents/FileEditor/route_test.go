package FileEditor

// route_test.go 文件编辑「压缩包任务路由分发」的单元测试。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/agents/ArchiveOps"
	"LunarSubsystem/LunarHarness/agents/ArchiveReader"
	"LunarSubsystem/LunarHarness/engine/config"
)

func prepareRouteDocuments(t *testing.T) {
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
}

func TestRouteArchiveLifecycle(t *testing.T) {
	prepareRouteDocuments(t)

	created := ArchiveOps.Create("/document/pkg.zip", []string{"/document/a.txt"})
	if !created.OK {
		t.Fatalf("准备压缩包失败: %s", created.Summary)
	}

	// 1. 整包结构查询：自动改道到归档解析
	outcome, routed := routeArchive(ToolList, map[string]any{"path": "unknown/pkg.zip"})
	if !routed || !outcome.OK {
		t.Fatalf("压缩包查询未路由成功: routed=%v summary=%s", routed, outcome.Summary)
	}
	if !strings.Contains(outcome.Summary, "a.txt") {
		t.Fatalf("结构摘要缺少条目: %s", outcome.Summary)
	}

	// 2. 条目读取：pkg.zip/a.txt 语法应读取条目内容
	outcome, routed = routeArchive(ToolRead, map[string]any{"path": "unknown/pkg.zip/a.txt"})
	if !routed || !outcome.OK {
		t.Fatalf("条目读取未路由成功: routed=%v summary=%s", routed, outcome.Summary)
	}
	if !strings.Contains(outcome.Summary, "alpha") {
		t.Fatalf("条目内容错误: %s", outcome.Summary)
	}

	// 3. 用普通写入工具指向压缩包：必须被改道并拒绝
	outcome, routed = routeArchive(ToolWrite, map[string]any{"path": "unknown/pkg.zip", "content": "x"})
	if !routed {
		t.Fatal("压缩包写入应被路由")
	}
	if outcome.OK {
		t.Fatalf("压缩包不应允许 file_write 直写: %s", outcome.Summary)
	}
	if !strings.Contains(outcome.Summary, ToolArchiveAdd) {
		t.Fatalf("失败提示应引导 archive_add: %s", outcome.Summary)
	}

	// 4. 显式归档工具：创建新压缩包
	outcome, routed = routeArchive(ToolArchiveCreate, map[string]any{
		"path": "new.zip", "paths": []any{"/document/a.txt"},
	})
	if !routed || !outcome.OK {
		t.Fatalf("归档创建未成功: routed=%v summary=%s", routed, outcome.Summary)
	}

	// 5. 普通删除工具指向压缩包：自动改道到归档操作
	outcome, routed = routeArchive(ToolDelete, map[string]any{"path": "unknown/pkg.zip"})
	if !routed || !outcome.OK {
		t.Fatalf("压缩包删除未路由成功: routed=%v summary=%s", routed, outcome.Summary)
	}
	if _, _, err := ArchiveReader.ResolveArchivePath(created.Path); err == nil {
		t.Fatal("路由删除后压缩包仍存在")
	}

	// 6. 普通文件不应被误路由
	if _, routed := routeArchive(ToolList, map[string]any{"path": "a.txt"}); routed {
		t.Fatal("普通文本文件不应走归档路由")
	}
}

// TestRouteArchiveAddRemove 显式归档工具应完整转发新增 / 移除。
func TestRouteArchiveAddRemove(t *testing.T) {
	prepareRouteDocuments(t)
	created := ArchiveOps.Create("/document/pkg.zip", []string{"/document/a.txt"})
	if !created.OK {
		t.Fatalf("准备压缩包失败: %s", created.Summary)
	}
	outcome, routed := routeArchive(ToolArchiveAdd, map[string]any{
		"path": "unknown/pkg.zip", "entry": "c.txt", "content": "gamma",
	})
	if !routed || !outcome.OK {
		t.Fatalf("归档新增未成功: routed=%v summary=%s", routed, outcome.Summary)
	}
	outcome, routed = routeArchive(ToolArchiveRemove, map[string]any{
		"path": "unknown/pkg.zip", "entry": "c.txt",
	})
	if !routed || !outcome.OK {
		t.Fatalf("归档移除未成功: routed=%v summary=%s", routed, outcome.Summary)
	}
}
