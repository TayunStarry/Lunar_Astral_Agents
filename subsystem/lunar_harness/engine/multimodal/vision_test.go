package multimodal

// vision_test.go 视觉读取变换管线的回归测试
//
// 覆盖四条核心保证：
//   1. 静态图副本受边长约束（只缩不放）
//   2. 句柄标签叠印在副本右上角（原图不改动）
//   3. 副本按缓存路径复用（第二次调用直接读回，不再变换）
//   4. 动图原样直通（字节不变，不缩放不叠印不缓存）

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// TestVisionTransformScalesAndOverlays 静态图：缩放受边长约束 + 右上角叠印 + 缓存命中
func TestVisionTransformScalesAndOverlays(t *testing.T) {
	dir := t.TempDir()
	// 源图：2000x1000 纯白 PNG（长边超限，必触发缩放）
	src := image.NewRGBA(image.Rect(0, 0, 2000, 1000))
	for y := 0; y < 1000; y++ {
		for x := 0; x < 2000; x++ {
			src.SetRGBA(x, y, color.RGBA{R: 255, G: 255, B: 255, A: 255})
		}
	}
	inPath := filepath.Join(dir, "src.png")
	var raw bytes.Buffer
	if err := png.Encode(&raw, src); err != nil {
		t.Fatalf("编码测试源图失败: %v", err)
	}
	if err := os.WriteFile(inPath, raw.Bytes(), 0o644); err != nil {
		t.Fatalf("写入测试源图失败: %v", err)
	}
	cachePath := filepath.Join(dir, "vision", "hash_s1080.jpg")

	data, mime, cached, err := VisionTransform(inPath, cachePath, 1080, "@I.003")
	if err != nil {
		t.Fatalf("变换失败: %v", err)
	}
	if mime != "image/jpeg" {
		t.Errorf("静态图副本 contentType 应为 image/jpeg，实际 %s", mime)
	}
	if cached {
		t.Errorf("首次变换不应命中缓存")
	}
	decoded, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("副本无法解码: %v", err)
	}
	bounds := decoded.Bounds()
	if bounds.Dx() > 1080 || bounds.Dy() > 1080 {
		t.Errorf("副本边长超出约束: %dx%d", bounds.Dx(), bounds.Dy())
	}
	if bounds.Dx() != 1080 || bounds.Dy() != 540 {
		t.Errorf("副本应按比例缩放为 1080x540，实际 %dx%d", bounds.Dx(), bounds.Dy())
	}
	// 叠印验证：右上角药丸区域应为深色（黑底 alpha 165 压平后明显暗于纯白原图）。
	// 1080x540 副本的药丸几何：短边 540 → 2× 缩放，药丸约 96x34、右上角外边距 8px
	pillX := bounds.Dx() - 40
	pillY := 20
	r, g, b, _ := decoded.At(pillX, pillY).RGBA()
	if r > 32768 || g > 32768 || b > 32768 {
		t.Errorf("右上角(%d,%d)应为叠印药丸深色区域，实际 RGB=(%d,%d,%d)", pillX, pillY, r>>8, g>>8, b>>8)
	}
	// 药丸占比验证：高度应达到短边的 4% 上下（模型可靠 OCR 的下限）
	if pillHeightRatio := 42.0 / float64(bounds.Dy()); pillHeightRatio < 0.03 {
		t.Errorf("药丸高度占画面比例过低（%.1f%%），模型可能读不出句柄", pillHeightRatio*100)
	}
	// 原图不变：源文件仍为 2000x1000
	srcAgain, err := os.ReadFile(inPath)
	if err != nil || !bytes.Equal(srcAgain, raw.Bytes()) {
		t.Fatalf("原图被修改（必须只读）")
	}
	// 缓存命中：同样入参第二次调用直接读回
	data2, _, cached2, err := VisionTransform(inPath, cachePath, 1080, "@I.003")
	if err != nil {
		t.Fatalf("二次变换失败: %v", err)
	}
	if !cached2 {
		t.Errorf("第二次调用应命中缓存")
	}
	if !bytes.Equal(data, data2) {
		t.Errorf("缓存副本与首次产物不一致")
	}
	// 缓存文件真实落盘
	if _, err := os.Stat(cachePath); err != nil {
		t.Errorf("缓存文件未落盘: %v", err)
	}
}

// TestVisionTransformAnimatedPassthrough 动图：原样直通（字节不变、不缓存）
func TestVisionTransformAnimatedPassthrough(t *testing.T) {
	dir := t.TempDir()
	// 源图：3 帧 GIF（超过 >2 帧阈值，判定为动图）
	frame := image.NewPaletted(image.Rect(0, 0, 64, 64), color.Palette{color.RGBA{R: 255, G: 0, B: 0, A: 255}})
	var raw bytes.Buffer
	if err := gif.EncodeAll(&raw, &gif.GIF{Image: []*image.Paletted{frame, frame, frame}, Delay: []int{10, 10, 10}}); err != nil {
		t.Fatalf("编码测试动图失败: %v", err)
	}
	if !IsAnimatedImage(raw.Bytes()) {
		t.Fatalf("测试前提不成立：3 帧 GIF 应被判定为动图")
	}
	inPath := filepath.Join(dir, "anim.gif")
	if err := os.WriteFile(inPath, raw.Bytes(), 0o644); err != nil {
		t.Fatalf("写入测试动图失败: %v", err)
	}
	cachePath := filepath.Join(dir, "vision", "hash_s1080.jpg")

	data, mime, cached, err := VisionTransform(inPath, cachePath, 1080, "@I.003")
	if err != nil {
		t.Fatalf("变换失败: %v", err)
	}
	if !bytes.Equal(data, raw.Bytes()) {
		t.Errorf("动图必须原样直通，实际字节被改动")
	}
	if mime != "image/gif" {
		t.Errorf("动图直通应保留原 MIME image/gif，实际 %s", mime)
	}
	if cached {
		t.Errorf("动图直通不应写入缓存")
	}
	if _, err := os.Stat(cachePath); err == nil {
		t.Errorf("动图直通不应产生缓存文件")
	}
}
