package resource

// handles_test.go 新句柄库的核心约束：URL 唯一绑定、本地路由、动图独立分类

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"os"
	"path/filepath"
	"testing"
)

// TestLocalResourceBindsFileReadURL 本地资源句柄绑定 /file/read/<rel>，且同一个 URL 只对应一个句柄
func TestLocalResourceBindsFileReadURL(t *testing.T) {
	setupArchiveTest(t)

	first, err := Admit(AdmitInput{SourceKind: SourceKindBytes, Data: []byte("同一份内容"), Name: "same.txt"})
	if err != nil {
		t.Fatalf("首次入库失败: %v", err)
	}
	if first.URL != FileReadURL(first.BlobPath) {
		t.Fatalf("本地资源 URL 应为服务相对路由，实际 %q（blob=%q）", first.URL, first.BlobPath)
	}
	second, err := Admit(AdmitInput{SourceKind: SourceKindBytes, Data: []byte("同一份内容"), Name: "same.txt"})
	if err != nil {
		t.Fatalf("二次入库失败: %v", err)
	}
	if second.Handle != first.Handle {
		t.Errorf("同一 URL 必须复用同一句柄: %s / %s", first.Handle, second.Handle)
	}
	// 一条 URL 在句柄表与 URL 主表各自只有一行
	var handleRows, urlRows int
	_ = archiveDB.db.QueryRow(`SELECT COUNT(*) FROM handle_file WHERE url = ?`, NormalizeURL(first.URL)).Scan(&handleRows)
	_ = archiveDB.db.QueryRow(`SELECT COUNT(*) FROM url_master WHERE url = ?`, NormalizeURL(first.URL)).Scan(&urlRows)
	if handleRows != 1 || urlRows != 1 {
		t.Errorf("URL 应唯一绑定: handle 行=%d url 行=%d", handleRows, urlRows)
	}
}

// TestAnimatedImageGetsOwnKind 多帧 GIF 归 @G，静图归 @I
func TestAnimatedImageGetsOwnKind(t *testing.T) {
	setupArchiveTest(t)

	frame := image.NewPaletted(image.Rect(0, 0, 8, 8), color.Palette{color.Black})
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, &gif.GIF{
		Image: []*image.Paletted{frame, frame, frame},
		Delay: []int{10, 10, 10},
	}); err != nil {
		t.Fatalf("编码测试动图失败: %v", err)
	}
	animated, err := Admit(AdmitInput{SourceKind: SourceKindBytes, Data: buf.Bytes(), Name: "anim.gif", Origin: OriginChannel})
	if err != nil {
		t.Fatalf("动图入库失败: %v", err)
	}
	if animated.Kind != KindAnimated || animated.Handle == "" || animated.Handle[1] != 'G' {
		t.Errorf("动图应为 @G 句柄，实际 kind=%s handle=%s", animated.Kind, animated.Handle)
	}
	if animated.URL != FileReadURL(animated.BlobPath) {
		t.Errorf("动图 URL 应为本地路由，实际 %q", animated.URL)
	}

	png := []byte{
		0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 'I', 'H', 'D', 'R',
		0x00, 0x00, 0x00, 0x04, 0x00, 0x00, 0x00, 0x04,
		0x08, 0x02, 0x00, 0x00, 0x00,
	}
	still, err := Admit(AdmitInput{SourceKind: SourceKindBytes, Data: png, Name: "still.png"})
	if err != nil {
		t.Fatalf("静图入库失败: %v", err)
	}
	if still.Kind != KindImage {
		t.Errorf("静图应为 @I，实际 %s", still.Kind)
	}
}

// TestRemoteBilibiliBindsFullPageURL B 站链接建 @V 句柄并指向完整页面 URL
func TestRemoteBilibiliBindsFullPageURL(t *testing.T) {
	setupArchiveTest(t)

	res, err := AdmitURL(URLAdmitInput{
		URL:    "https://www.bilibili.com/video/BV1o4xg6RE9m?spm_id_from=333.1007.0.0",
		Origin: OriginChannel,
	})
	if err != nil {
		t.Fatalf("B 站链接入仓失败: %v", err)
	}
	if res.Kind != KindVideo {
		t.Errorf("B 站链接应为 @V，实际 %s", res.Kind)
	}
	if res.URL != "https://www.bilibili.com/video/BV1o4xg6RE9m" {
		t.Errorf("应存规范化完整页面 URL，实际 %q", res.URL)
	}
	entry, ok := ByHash(KindVideo, "")
	if ok {
		t.Errorf("空哈希不应命中: %+v", entry)
	}
	row, found := archiveDB.FindResourceByURL(res.URL)
	if !found || row.Handle != res.Handle {
		t.Errorf("URL 主表应绑定同一句柄: %+v", row)
	}
}

// TestScanRegistersWorkspaceTrees 三棵内容仓库树启动扫描仍自动纳入句柄体系
func TestScanRegistersWorkspaceTrees(t *testing.T) {
	tmp := setupArchiveTest(t)
	for _, rel := range []string{"documents/readme.md", "audios/voice.mp3"} {
		abs := filepath.Join(tmp, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatalf("创建目录失败: %v", err)
		}
		if err := os.WriteFile(abs, []byte("content-"+rel), 0o644); err != nil {
			t.Fatalf("写入测试文件失败: %v", err)
		}
	}
	if err := SyncStorageHandles(); err != nil {
		t.Fatalf("扫描同步失败: %v", err)
	}
	for rel, kind := range map[string]string{"documents/readme.md": KindDocument, "audios/voice.mp3": KindAudio} {
		res, ok := ForPath(rel)
		if !ok || res.Kind != kind || res.URL != FileReadURL(rel) {
			t.Errorf("%s 应登记为 %s 且绑定本地路由，实际 ok=%v res=%+v", rel, kind, ok, res)
		}
	}
}
