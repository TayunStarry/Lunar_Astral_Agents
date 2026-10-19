package lunarloop

// multimodal_test.go 视觉读取 URL 解析与动态图判定的回归测试
//
// 背景：视觉读取 URL（/file/miniature/...?tag=@I.003）带查询参数，
// 直接按整串当文件路径会查找失败，动态图判定随之报错并退化为静态图；
// 同时 QQ 等渠道常把 GIF 动图命名为 .png，扩展名不能作为判定依据。

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"os"
	"path/filepath"
	"testing"

	mmmodule "CorePersonality/engine/multimodal"
	"LunarSubsystem/GeneralConfig"

	"github.com/dop251/goja"
)

// TestResolveImageBytesStripsServiceURLQueryAndDetectsAnimated
// 断言「.png 后缀的 GIF + 带 tag 查询串的视觉 URL」仍能被识别为动态图。
func TestResolveImageBytesStripsServiceURLQueryAndDetectsAnimated(t *testing.T) {
	tmp := t.TempDir()
	origLocal := *GeneralConfig.LocalDir
	*GeneralConfig.LocalDir = tmp
	t.Cleanup(func() { *GeneralConfig.LocalDir = origLocal })

	frame := image.NewPaletted(image.Rect(0, 0, 8, 8), color.Palette{color.RGBA{R: 255, A: 255}})
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, &gif.GIF{
		Image: []*image.Paletted{frame, frame, frame},
		Delay: []int{10, 10, 10},
	}); err != nil {
		t.Fatalf("编码测试动图失败: %v", err)
	}
	rel := "multimedia/anim.png" // 故意用静态扩展名：内容才是真实格式
	abs := filepath.Join(tmp, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("准备目录失败: %v", err)
	}
	if err := os.WriteFile(abs, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("写入测试动图失败: %v", err)
	}

	imageURL := goja.New().ToValue("/file/miniature/" + rel + "?tag=%40I.001")
	data, err := resolveImageBytes(imageURL)
	if err != nil {
		t.Fatalf("视觉 URL 解析失败: %v", err)
	}
	if !mmmodule.IsAnimatedImage(data) {
		t.Errorf(".png 后缀的 GIF 应被识别为动态图")
	}
}
