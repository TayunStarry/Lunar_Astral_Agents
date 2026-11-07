package resource

import (
	"os"
	"path/filepath"
	"testing"
)

// appendPNGChunk 只拼块头与占位数据：校验只看长度、类型与魔数，不校验 CRC
func appendPNGChunk(dst []byte, chunkType string, dataLen int) []byte {
	dst = append(dst, byte(dataLen>>24), byte(dataLen>>16), byte(dataLen>>8), byte(dataLen))
	dst = append(dst, chunkType...)
	return append(dst, make([]byte, dataLen+4)...)
}

// samplePNG 构造最小 PNG：签名 + IHDR + (acTL) + IDAT + IEND
func samplePNG(withACTL bool) []byte {
	raw := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
	raw = appendPNGChunk(raw, "IHDR", 13)
	if withACTL {
		raw = appendPNGChunk(raw, "acTL", 8)
	}
	raw = appendPNGChunk(raw, "IDAT", 8)
	return appendPNGChunk(raw, "IEND", 0)
}

func writeVerifyFixture(t *testing.T, name string, raw []byte) string {
	t.Helper()
	abs := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(abs, raw, 0o600); err != nil {
		t.Fatalf("写入测试文件失败: %v", err)
	}
	return abs
}

func TestVerifyStoredAcceptsAPNGAsAnimated(t *testing.T) {
	abs := writeVerifyFixture(t, "apng.png", samplePNG(true))
	if err := VerifyStored(KindAnimated, abs, "image/apng"); err != nil {
		t.Fatalf("APNG 按动图声明应通过: %v", err)
	}
	if err := VerifyStored(KindImage, abs, "image/png"); err == nil {
		t.Fatal("APNG 声明为静态图应被拒绝")
	}
}

func TestVerifyStoredRejectsStaticPNGAsAnimated(t *testing.T) {
	abs := writeVerifyFixture(t, "static.png", samplePNG(false))
	if err := VerifyStored(KindImage, abs, "image/png"); err != nil {
		t.Fatalf("普通 PNG 按静态图声明应通过: %v", err)
	}
	if err := VerifyStored(KindAnimated, abs, "image/apng"); err == nil {
		t.Fatal("普通 PNG 声明为动图应被拒绝")
	}
}

// appendRIFFChunk 拼 WebP 块：类型 + 小端长度 + 占位数据，奇数长度补 1 字节 padding
func appendRIFFChunk(dst []byte, chunkType string, dataLen int) []byte {
	dst = append(dst, chunkType...)
	dst = append(dst, byte(dataLen), byte(dataLen>>8), byte(dataLen>>16), byte(dataLen>>24))
	dst = append(dst, make([]byte, dataLen)...)
	if dataLen%2 == 1 {
		dst = append(dst, 0)
	}
	return dst
}

// sampleWebP 构造最小 RIFF/WEBP：VP8X + (ANIM) + VP8
func sampleWebP(withANIM bool) []byte {
	body := []byte("WEBP")
	body = appendRIFFChunk(body, "VP8X", 10)
	if withANIM {
		body = appendRIFFChunk(body, "ANIM", 6)
	}
	body = appendRIFFChunk(body, "VP8 ", 12)
	raw := []byte("RIFF")
	raw = append(raw, byte(len(body)), byte(len(body)>>8), byte(len(body)>>16), byte(len(body)>>24))
	return append(raw, body...)
}

func TestVerifyStoredAcceptsAnimatedWebPAsAnimated(t *testing.T) {
	abs := writeVerifyFixture(t, "animated.webp", sampleWebP(true))
	if err := VerifyStored(KindAnimated, abs, "image/webp"); err != nil {
		t.Fatalf("动态 WebP 按动图声明应通过: %v", err)
	}
	if err := VerifyStored(KindImage, abs, "image/webp"); err == nil {
		t.Fatal("动态 WebP 声明为静态图应被拒绝")
	}
}

func TestVerifyStoredRejectsStaticWebPAsAnimated(t *testing.T) {
	abs := writeVerifyFixture(t, "static.webp", sampleWebP(false))
	if err := VerifyStored(KindImage, abs, "image/webp"); err != nil {
		t.Fatalf("静态 WebP 按静态图声明应通过: %v", err)
	}
	if err := VerifyStored(KindAnimated, abs, "image/webp"); err == nil {
		t.Fatal("静态 WebP 声明为动图应被拒绝")
	}
}
