package napcat

// image_media_test.go QQ 出站大图压缩的回归测试
//
// 事故背景：6144x4096 / 38MB 的 4x 超分作品交给 NapCat 后，QQ sendMsg 返回
// 「rich media transfer failed」。此前的 404 是图片地址错误，这次是 QQ 富媒体
// 上传上限，需要在出站前压缩本机大图。

import (
	"LunarSubsystem/LunarHarness/engine/config"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupQQImageTest(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	orig := config.LocalDir
	config.LocalDir = tmp
	t.Cleanup(func() { config.LocalDir = orig })
	if err := os.MkdirAll(filepath.Join(tmp, "multimedia", "generated"), 0755); err != nil {
		t.Fatalf("准备图片目录失败: %v", err)
	}
	return tmp
}

func writeTestPNG(t *testing.T, path string, width, height int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{uint8(x % 256), uint8((x * y) % 256), uint8(y % 256), 255})
		}
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("创建测试图片失败: %v", err)
	}
	defer file.Close()
	if err := png.Encode(file, img); err != nil {
		t.Fatalf("编码测试图片失败: %v", err)
	}
}

func TestCompressImageForQQ(t *testing.T) {
	tmp := setupQQImageTest(t)
	rel := "multimedia/generated/big.png"
	writeTestPNG(t, filepath.Join(tmp, filepath.FromSlash(rel)), 4200, 100)

	url := "/file/read/" + rel
	compressed, ok := compressImageForQQ(url)
	if !ok {
		t.Fatal("长边 4200 的图片应触发 QQ 压缩")
	}
	t.Cleanup(func() { _ = os.Remove(compressed) })
	if !strings.HasSuffix(strings.ToLower(compressed), ".jpg") {
		t.Fatalf("压缩副本应为 JPEG，实得 %s", compressed)
	}
	file, err := os.Open(compressed)
	if err != nil {
		t.Fatalf("打开压缩副本失败: %v", err)
	}
	cfg, _, err := image.DecodeConfig(file)
	file.Close()
	if err != nil {
		t.Fatalf("解析压缩副本失败: %v", err)
	}
	if cfg.Width != qqImageTargetSide {
		t.Fatalf("压缩后长边应为 %d，实得 %dx%d", qqImageTargetSide, cfg.Width, cfg.Height)
	}

	// imageSegments 必须选用压缩副本，并在发送结束后清理临时文件
	segments, cleanup := imageSegments([]string{url})
	if len(segments) != 1 {
		t.Fatalf("消息段数量异常: %d", len(segments))
	}
	data := segments[0]["data"].(map[string]string)
	segmentFile := data["file"]
	if !strings.HasSuffix(strings.ToLower(segmentFile), ".jpg") {
		t.Fatalf("消息段应使用压缩后的 JPEG: %q", segmentFile)
	}
	if _, err := os.Stat(segmentFile); err != nil {
		t.Fatalf("消息段引用的压缩副本不存在: %v", err)
	}
	cleanup()
	if _, err := os.Stat(segmentFile); !os.IsNotExist(err) {
		t.Fatalf("清理后临时文件仍存在: %v", err)
	}
}

func TestCompressImageForQQSkipsNormalImage(t *testing.T) {
	tmp := setupQQImageTest(t)
	rel := "multimedia/generated/normal.png"
	writeTestPNG(t, filepath.Join(tmp, filepath.FromSlash(rel)), 800, 600)

	if compressed, ok := compressImageForQQ("/file/read/" + rel); ok {
		_ = os.Remove(compressed)
		t.Fatal("普通尺寸图片不应被压缩")
	}
}

func TestCompressImageForQQRejectsOutsideLocalDir(t *testing.T) {
	setupQQImageTest(t)
	for _, src := range []string{
		"/file/read/../outside.png",
		"https://example.com/a.png",
		"data:image/png;base64,AAAA",
	} {
		if _, ok := compressImageForQQ(src); ok {
			t.Fatalf("%q 不应参与本机图片压缩", src)
		}
	}
}
