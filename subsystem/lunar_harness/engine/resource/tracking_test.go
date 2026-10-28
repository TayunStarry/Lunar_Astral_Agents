package resource

// tracking_test.go 落盘追踪：persistence 的底层文件原语在内容仓库三棵树内落盘时，
// 句柄系统必须自动跟随（写 → 登记、移 → 改写、删 → 清理），派生缓存则不入索引。
//
// 这些断言直接调用 persistence 原语，覆盖的不只是 HTTP 上传路径。

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	persistence "LunarSubsystem/LunarHarness/engine/persistence"
)

// TestWriteInStorageTreesIsTracked documents / audios / multimedia 内写入自动获得句柄
func TestWriteInStorageTreesIsTracked(t *testing.T) {
	setupArchiveTest(t)

	doc, err := persistence.SaveFileEx("documents/tracked.md", persistence.SaveOptions{Overwrite: true}, strings.NewReader("# 标题\n正文\n"))
	if err != nil {
		t.Fatalf("写入 documents 失败: %v", err)
	}
	if doc.Handle == "" || doc.Handle[1] != 'D' {
		t.Fatalf("documents 内写入应自动获得文档句柄，实得 %q", doc.Handle)
	}
	if entry, ok := ForPath("documents/tracked.md"); !ok || entry.Handle != doc.Handle {
		t.Errorf("documents 内实体应按路径登记句柄: ok=%v entry=%#v", ok, entry)
	}

	audio, err := persistence.SaveFileEx("audios/tracked.wav", persistence.SaveOptions{Overwrite: true}, bytes.NewReader([]byte("RIFF....WAVE")))
	if err != nil {
		t.Fatalf("写入 audios 失败: %v", err)
	}
	if audio.Handle == "" || audio.Handle[1] != 'A' {
		t.Fatalf("audios 内写入应自动获得音频句柄，实得 %q", audio.Handle)
	}

	image, err := persistence.SaveFileEx("multimedia/tracked.png", persistence.SaveOptions{Overwrite: true}, bytes.NewReader([]byte("fake-png-bytes")))
	if err != nil {
		t.Fatalf("写入 multimedia 失败: %v", err)
	}
	if image.Handle == "" || image.Handle[1] != 'I' {
		t.Fatalf("multimedia 内写入应自动获得图片句柄，实得 %q", image.Handle)
	}
}

// TestDerivedCacheWriteNotTracked multimedia/vision 与 multimedia/footage 写入不登记句柄
func TestDerivedCacheWriteNotTracked(t *testing.T) {
	setupArchiveTest(t)

	for _, rel := range []string{"multimedia/vision/tracked_s1024.jpg", "multimedia/footage/tracked_seg0.mp4"} {
		result, err := persistence.SaveFileEx(rel, persistence.SaveOptions{Overwrite: true}, bytes.NewReader([]byte("derived-bytes")))
		if err != nil {
			t.Fatalf("写入派生缓存失败(%s): %v", rel, err)
		}
		if result.Handle != "" {
			t.Errorf("派生缓存不应获得句柄: %s -> %s", rel, result.Handle)
		}
		if _, ok := ForPath(rel); ok {
			t.Errorf("派生缓存不应出现在句柄索引: %s", rel)
		}
	}
}

// TestDeleteClearsHandles 删除实体后句柄随实体消失
func TestDeleteClearsHandles(t *testing.T) {
	setupArchiveTest(t)

	written, err := persistence.SaveFileEx("documents/to-delete.md", persistence.SaveOptions{Overwrite: true}, strings.NewReader("内容\n"))
	if err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	if written.Handle == "" {
		t.Fatalf("写入后应有句柄")
	}
	if _, err := persistence.DeleteFile("documents/to-delete.md"); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	if _, ok := ForPath("documents/to-delete.md"); ok {
		t.Errorf("实体删除后句柄应被清理")
	}
}

// TestMoveRewritesHandles 移动 / 改名后句柄改指新路径
func TestMoveRewritesHandles(t *testing.T) {
	tmp := setupArchiveTest(t)

	if _, err := persistence.SaveFileEx("documents/moving.md", persistence.SaveOptions{Overwrite: true}, strings.NewReader("内容\n")); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	before, ok := ForPath("documents/moving.md")
	if !ok {
		t.Fatalf("写入后应有句柄")
	}
	if err := os.MkdirAll(filepath.Join(tmp, "documents", "sub"), 0755); err != nil {
		t.Fatalf("准备目标目录失败: %v", err)
	}
	resp := persistence.ExecuteMove(persistence.MoveItemRequest{
		Sources:          []string{"documents/moving.md"},
		TargetDir:        "documents/sub",
		ConflictStrategy: "overwrite",
		CreateDirs:       true,
	})
	if !resp.Success {
		t.Fatalf("移动失败: %+v", resp)
	}
	after, ok := ForPath("documents/sub/moving.md")
	if !ok || after.Handle != before.Handle {
		t.Errorf("移动后句柄应改指新路径且编号不变: before=%s after=%#v", before.Handle, after)
	}
	if _, ok := ForPath("documents/moving.md"); ok {
		t.Errorf("旧路径不应残留句柄")
	}
}
