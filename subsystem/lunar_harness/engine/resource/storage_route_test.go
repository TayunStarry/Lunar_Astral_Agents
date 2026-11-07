package resource

// storage_route_test.go 存储根归位规则的回归测试：
// 目录由分类器给出的类型码决定——媒体进多媒体树、可解析文档留在 documents/、
// 不可读二进制进 unknown/；无扩展名媒体靠预读文件头识别，不再先落错目录。

import (
	"bytes"
	"io"
	"testing"
)

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
		// 媒体扩展名却落在文档 / 未知根下：按真实类型改投媒体树
		"documents/photo.png": "multimedia/photo.png",
		"unknown/voice.mp3":   "audios/voice.mp3",
	}
	for input, want := range cases {
		if got := RouteStorageRel(input, nil); got != want {
			t.Fatalf("%s 归位应为 %s，实得 %s", input, want, got)
		}
	}
}

// TestRouteStorageRelByMagic 无扩展名媒体按文件头识别后归入媒体树
func TestRouteStorageRelByMagic(t *testing.T) {
	mp4Head := []byte{0x00, 0x00, 0x00, 0x18, 'f', 't', 'y', 'p', 'm', 'p', '4', '2'}
	for _, input := range []string{"documents/clip", "unknown/clip"} {
		if got := RouteStorageRel(input, mp4Head); got != "multimedia/clip.mp4" {
			t.Fatalf("%s 应归入 multimedia/clip.mp4，实得 %s", input, got)
		}
	}
	wavHead := []byte("RIFF\x00\x00\x00\x00WAVE")
	if got := RouteStorageRel("documents/voice", wavHead); got != "audios/voice.wav" {
		t.Fatalf("无扩展名音频应归入 audios/voice.wav，实得 %s", got)
	}
	// 拿不到文件头时退回不可读二进制，与原行为一致
	if got := RouteStorageRel("documents/clip", nil); got != "unknown/clip" {
		t.Fatalf("无文件头应落入 unknown/clip，实得 %s", got)
	}
}

// TestRouteStorageReader 预读文件头路由后，字节必须原样交回下游
func TestRouteStorageReader(t *testing.T) {
	payload := append([]byte{0x00, 0x00, 0x00, 0x18, 'f', 't', 'y', 'p', 'm', 'p', '4', '2'}, []byte("tail")...)
	rel, body := RouteStorageReader("documents/clip", bytes.NewReader(payload))
	if rel != "multimedia/clip.mp4" {
		t.Fatalf("无扩展名视频应归入 multimedia/clip.mp4，实得 %s", rel)
	}
	got, err := io.ReadAll(body)
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("预读字节应原样交回，err=%v, len=%d", err, len(got))
	}

	// 扩展名已能确定目录：不预读，body 原样返回
	rel, body = RouteStorageReader("documents/pkg.zip", bytes.NewReader(payload))
	if rel != "unknown/pkg.zip" {
		t.Fatalf("压缩包应归入 unknown/pkg.zip，实得 %s", rel)
	}
	if got, _ := io.ReadAll(body); !bytes.Equal(got, payload) {
		t.Fatalf("未预读时字节应原样交回，len=%d", len(got))
	}
}

// TestSnapshotRelBinaryRoots 归档入库的落点同样遵守类型码规则
func TestSnapshotRelBinaryRoots(t *testing.T) {
	if got := snapshotRel("abc123", ".zip", "pkg.zip", KindDocument); got != "unknown/pkg.zip" {
		t.Fatalf("压缩包应落在 unknown/，实得 %s", got)
	}
	if got := snapshotRel("abc123", ".md", "note.md", KindDocument); got != "documents/note.md" {
		t.Fatalf("文本应落在 documents/，实得 %s", got)
	}
	if got := snapshotRel("abc123", ".pdf", "report.pdf", KindDocument); got != "documents/report.pdf" {
		t.Fatalf("可解析 PDF 应留在 documents/，实得 %s", got)
	}
	if got := snapshotRel("abc123", ".mp3", "voice.mp3", KindAudio); got != "audios/abc123.mp3" {
		t.Fatalf("音频应落在 audios/，实得 %s", got)
	}
	// 显式声明为视频但扩展名不常见：目录仍以类型码为准
	if got := snapshotRel("abc123", ".bin", "clip.bin", KindVideo); got != "multimedia/abc123.bin" {
		t.Fatalf("视频应落在 multimedia/，实得 %s", got)
	}
}
