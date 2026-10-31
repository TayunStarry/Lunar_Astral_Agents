package resource

// storage_route_test.go 存储根归位规则的回归测试：
// 不可读二进制进 unknown/，文本与可解析文档留在 documents/，媒体树不受影响。

import "testing"

func TestRouteStorageRel(t *testing.T) {
	cases := map[string]string{
		"documents/pkg.zip":    "unknown/pkg.zip",
		"documents/model.gguf": "unknown/model.gguf",
		"documents/tool.exe":   "unknown/tool.exe",
		"documents/note.md":    "documents/note.md",
		"documents/report.pdf": "documents/report.pdf",
		"documents/sheet.xlsx": "documents/sheet.xlsx",
		"multimedia/photo.png": "multimedia/photo.png",
		"audios/cache/a.wav":   "audios/cache/a.wav",
		"unknown/archive.7z":   "unknown/archive.7z",
		"pkg.zip":              "unknown/pkg.zip",
		"sub/tool.bin":         "unknown/sub/tool.bin",
		"config.json":          "config.json",
	}
	for input, want := range cases {
		if got := RouteStorageRel(input); got != want {
			t.Fatalf("%s 归位应为 %s，实得 %s", input, want, got)
		}
	}
}

// TestSnapshotRelBinaryRoots 归档入库的落点同样遵守 unknown/ 规则
func TestSnapshotRelBinaryRoots(t *testing.T) {
	if got := snapshotRel("abc123", ".zip", "pkg.zip"); got != "unknown/pkg.zip" {
		t.Fatalf("压缩包应落在 unknown/，实得 %s", got)
	}
	if got := snapshotRel("abc123", ".md", "note.md"); got != "documents/note.md" {
		t.Fatalf("文本应落在 documents/，实得 %s", got)
	}
	if got := snapshotRel("abc123", ".pdf", "report.pdf"); got != "documents/report.pdf" {
		t.Fatalf("可解析 PDF 应留在 documents/，实得 %s", got)
	}
}
