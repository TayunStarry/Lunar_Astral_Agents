package napcat

import (
	"path/filepath"
	"testing"

	file "CorePersonality/engine/persistence"
	"LunarSubsystem/GeneralConfig"
)

// TestDataURIExt 合并转发内的图片常常只有 file 引用，get_image 下载后是 data URI。
// 归档层按扩展名推断类型，data URI 必须补上正确扩展名，否则会被当成文档。
func TestDataURIExt(t *testing.T) {
	cases := map[string]string{
		"data:image/png;base64,AAAA":  ".png",
		"data:image/jpeg;base64,AAAA": ".jpg",
		"data:image/jpg;base64,AAAA":  ".jpg",
		"data:image/gif;base64,AAAA":  ".gif",
		"data:image/webp;base64,AAA":  ".webp",
		"data:image/bmp;base64,AAAA":  ".bmp",
		"data:image/avif;base64,AAA":  ".png", // 未知格式回退 png
		"data:,":                      ".png",
	}
	for uri, want := range cases {
		if got := dataURIExt(uri); got != want {
			t.Fatalf("dataURIExt(%q) = %q, want %q", uri, got, want)
		}
	}
}

// TestForwardImageName QQ CDN 直链常常没有扩展名；没有图片扩展名时归档层会按文档处理。
func TestForwardImageName(t *testing.T) {
	cases := map[string]string{
		"https://multimedia.nt.qq.com.cn/download?fileid=abc": "forward.jpg",
		"https://example.com/a.png":                           "forward.png",
		"https://example.com/a.JPEG?x=1":                      "forward.jpeg",
		"data:image/gif;base64,AAA":                           "forward.gif",
	}
	for source, want := range cases {
		if got := forwardImageName(source); got != want {
			t.Fatalf("forwardImageName(%q) = %q, want %q", source, got, want)
		}
	}
}

// TestAdmitForwardImageDataURI 只有 file 引用、下载后为 data URI 的转发图片
// 必须能收进资源归档并返回句柄，而不是退化成 [图片]。
func TestAdmitForwardImageDataURI(t *testing.T) {
	origLocal := *GeneralConfig.LocalDir
	origArchive := *GeneralConfig.ArchiveDBPath
	tmp := t.TempDir()
	*GeneralConfig.LocalDir = tmp
	*GeneralConfig.ArchiveDBPath = filepath.Join(tmp, "database", "archive.db")
	t.Cleanup(func() {
		_ = file.CloseArchiveDB()
		*GeneralConfig.LocalDir = origLocal
		*GeneralConfig.ArchiveDBPath = origArchive
	})

	const onePixelPNG = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="
	handle, ok := admitForwardImage(ImageData{URL: onePixelPNG})
	if !ok || handle == "" {
		t.Fatal("data URI 转发图片入库失败，会退化成 [图片]")
	}
	if handle[0] != '@' || handle[1] != 'I' {
		t.Fatalf("转发图片应返回图片句柄，实得 %q", handle)
	}

	if _, ok := admitForwardImage(ImageData{}); ok {
		t.Fatal("空图片数据不应返回句柄")
	}
}
