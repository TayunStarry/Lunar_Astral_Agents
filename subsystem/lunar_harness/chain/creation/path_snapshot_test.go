package creation

// path_snapshot_test.go 调度者路径树快照的验收：只列两个根、体积受限、缓存可失效。

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/internal/testenv"
)

// writeSnapshotFile 在隔离的本地目录下写入测试文件（不触碰真实 local_data）。
func writeSnapshotFile(t *testing.T, rel, content string) {
	t.Helper()
	full := filepath.Join(config.LocalDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPathSnapshotListsOnlyDocumentsAndUnknown(t *testing.T) {
	testenv.Isolate(t)
	writeSnapshotFile(t, "documents/a.md", "正文")
	writeSnapshotFile(t, "documents/sub/b.md", "正文")
	writeSnapshotFile(t, "unknown/out.txt", "正文")
	writeSnapshotFile(t, "audios/ignored.mp3", "正文")
	writeSnapshotFile(t, "multimedia/ignored.png", "正文")
	invalidatePathSnapshot()

	text := pathSnapshotText()
	for _, want := range []string{
		"/document：2 个文件 / 1 个目录",
		"unknown：1 个文件 / 0 个目录",
		"/document/",
		"/document/a.md",
		"/document/sub/",
		"/document/sub/b.md",
		"unknown/out.txt",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("快照缺少 %q：\n%s", want, text)
		}
	}
	for _, unwanted := range []string{"audios", "multimedia", "ignored"} {
		if strings.Contains(text, unwanted) {
			t.Fatalf("快照不应包含 %q：\n%s", unwanted, text)
		}
	}
	for _, unwanted := range []string{"size", "mtime", "handle"} {
		if strings.Contains(text, unwanted) {
			t.Fatalf("快照不应包含元数据字段 %q：\n%s", unwanted, text)
		}
	}
}

func TestPathSnapshotTruncatesWithinBudget(t *testing.T) {
	testenv.Isolate(t)
	for index := 0; index < 300; index++ {
		name := fmt.Sprintf("documents/%03d-%s.md", index, strings.Repeat("长", 20))
		writeSnapshotFile(t, name, "正文")
	}
	invalidatePathSnapshot()

	text := pathSnapshotText()
	if !strings.Contains(text, "已截断，其余需用工具查询") {
		t.Fatalf("超出上限时必须显式标注截断：\n%s", text)
	}
	if runes := utf8.RuneCountInString(text); runes > pathSnapshotMaxRunes {
		t.Fatalf("快照字符数 %d 超过上限 %d", runes, pathSnapshotMaxRunes)
	}
	for _, root := range pathSnapshotRoots {
		if !strings.Contains(text, displayRoot(root)+"：") {
			t.Fatalf("截断后仍须保留根目录统计 %q", root)
		}
	}
}

func TestPathSnapshotCacheInvalidates(t *testing.T) {
	testenv.Isolate(t)
	writeSnapshotFile(t, "documents/first.md", "正文")
	invalidatePathSnapshot()
	if text := pathSnapshotText(); !strings.Contains(text, "first.md") {
		t.Fatalf("首次快照缺少 first.md：\n%s", text)
	}

	writeSnapshotFile(t, "documents/second.md", "正文")
	if text := pathSnapshotText(); strings.Contains(text, "second.md") {
		t.Fatalf("TTL 内应复用缓存，不应看到 second.md")
	}

	invalidatePathSnapshot()
	if text := pathSnapshotText(); !strings.Contains(text, "second.md") {
		t.Fatalf("显式失效后应看到 second.md：\n%s", text)
	}

	writeSnapshotFile(t, "documents/third.md", "正文")
	pathSnapshotMu.Lock()
	pathSnapshotMemo.BuiltAt = time.Now().Add(-pathSnapshotTTL - time.Second)
	pathSnapshotMu.Unlock()
	if text := pathSnapshotText(); !strings.Contains(text, "third.md") {
		t.Fatalf("TTL 到期后应重新扫描并看到 third.md：\n%s", text)
	}
}
