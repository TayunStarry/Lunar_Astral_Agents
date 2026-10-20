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
	"image/png"
	"os"
	"path/filepath"
	"testing"

	mmmodule "CorePersonality/engine/multimodal"
	persist "CorePersonality/engine/persistence"
	"LunarSubsystem/GeneralConfig"

	"github.com/dop251/goja"
)

// TestBase64DecodePreservesBinaryBytes 截图/图片落盘必须走字节解码：
// atob 返回 Go 字符串，goja 按 UTF-8 转换会把 PNG 头 0x89 等非法字节替换成
// U+FFFD，再按 charCodeAt 还原就变成 0xFD，最终写出损坏的 PNG。
func TestBase64DecodePreservesBinaryBytes(t *testing.T) {
	vm := goja.New()
	rt := &Runtime{runtime: vm}
	vm.Set("decode", rt.base64Decode)
	// 1x1 PNG，首四字节应为 89 50 4E 47
	const png = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="
	v, err := vm.RunString(`(() => {
		const [buffer, decodeErr] = decode("` + png + `");
		if (decodeErr) return false;
		const bytes = new Uint8Array(buffer);
		return bytes.length > 8
			&& bytes[0] === 0x89 && bytes[1] === 0x50 && bytes[2] === 0x4E && bytes[3] === 0x47;
	})()`)
	if err != nil {
		t.Fatalf("执行失败: %v", err)
	}
	if !v.ToBoolean() {
		t.Fatalf("base64Decode 未保留 PNG 原始字节: %v", v.Export())
	}
}

// TestSaveFileWithBase64DecodeRoundTrip 复现截图落盘链路：
// base64Decode → Uint8Array → saveFile，落盘后必须仍是可解码的 PNG。
func TestSaveFileWithBase64DecodeRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	origLocal := *GeneralConfig.LocalDir
	origArchive := *GeneralConfig.ArchiveDBPath
	*GeneralConfig.LocalDir = tmp
	*GeneralConfig.ArchiveDBPath = filepath.Join(tmp, "database", "archive.db")
	t.Cleanup(func() {
		_ = persist.CloseArchiveDB()
		*GeneralConfig.LocalDir = origLocal
		*GeneralConfig.ArchiveDBPath = origArchive
	})

	vm := goja.New()
	rt := &Runtime{runtime: vm}
	vm.Set("base64Decode", rt.base64Decode)
	vm.Set("saveFile", rt.saveFile)

	const pngBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAIAAAACCAIAAAD91JpzAAAAEElEQVR4nGP8zwACTGCSAQANHQEDgslx/wAAAABJRU5ErkJggg=="
	v, err := vm.RunString(`(() => {
		const [buffer, decodeErr] = base64Decode("` + pngBase64 + `");
		if (decodeErr) return decodeErr.message;
		const bytes = new Uint8Array(buffer);
		const [, path, saveErr] = saveFile("multimedia/screenshot_regression.png", true, bytes);
		return saveErr ? saveErr.message : path;
	})()`)
	if err != nil {
		t.Fatalf("执行落盘链路失败: %v", err)
	}
	path := v.String()
	if path == "" {
		t.Fatal("saveFile 未返回路径")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取落盘文件失败: %v", err)
	}
	if len(data) < 8 || string(data[:8]) != "\x89PNG\r\n\x1a\n" {
		t.Fatalf("落盘文件不是合法 PNG 头: %x", data[:min(8, len(data))])
	}
	if _, err := png.Decode(bytes.NewReader(data)); err != nil {
		t.Fatalf("落盘 PNG 无法解码: %v", err)
	}
}

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
