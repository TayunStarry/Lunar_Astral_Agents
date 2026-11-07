package persistence

import (
	"encoding/binary"
	"testing"
)

// appendPNGChunk 只拼块头与占位数据：嗅探只看长度与类型，不校验 CRC
func appendPNGChunk(dst []byte, chunkType string, dataLen int) []byte {
	dst = append(dst, byte(dataLen>>24), byte(dataLen>>16), byte(dataLen>>8), byte(dataLen))
	dst = append(dst, chunkType...)
	return append(dst, make([]byte, dataLen+4)...)
}

// samplePNG 构造最小 PNG：签名 + IHDR + (acTL) + IDAT + IEND
func samplePNG(withACTL bool) []byte {
	raw := append([]byte{}, pngSignature...)
	raw = appendPNGChunk(raw, "IHDR", 13)
	if withACTL {
		raw = appendPNGChunk(raw, "acTL", 8)
	}
	raw = appendPNGChunk(raw, "IDAT", 8)
	return appendPNGChunk(raw, "IEND", 0)
}

func TestSniffImageFormatDistinguishesAPNG(t *testing.T) {
	apng := samplePNG(true)
	ext, mimeType := SniffImageFormat(apng)
	if ext != ".apng" || mimeType != "image/apng" {
		t.Fatalf("APNG 嗅探结果 = %q/%q，期望 .apng/image/apng", ext, mimeType)
	}
	if !IsAnimatedPNG(apng) {
		t.Fatal("带 acTL 的 PNG 应判为动图")
	}

	static := samplePNG(false)
	ext, mimeType = SniffImageFormat(static)
	if ext != ".png" || mimeType != "image/png" {
		t.Fatalf("普通 PNG 嗅探结果 = %q/%q，期望 .png/image/png", ext, mimeType)
	}
	if IsAnimatedPNG(static) {
		t.Fatal("不带 acTL 的 PNG 不应判为动图")
	}
}

func TestIsAnimatedPNGRejectsIncompleteChunks(t *testing.T) {
	// 签名正常但首个 chunk 声明长度越界：不能 panic，也不能误判为 APNG
	incomplete := append([]byte{}, pngSignature...)
	incomplete = append(incomplete, 0x00, 0x00, 0xFF, 0xFF, 'I', 'H', 'D', 'R')
	if IsAnimatedPNG(incomplete) {
		t.Fatal("数据不完整的 PNG 不应判为动图")
	}
	if IsAnimatedPNG([]byte{0x89, 'P', 'N', 'G'}) {
		t.Fatal("过短的 PNG 头不应判为动图")
	}
}

// riffChunk 构造 WebP 用的 RIFF chunk 声明
type riffChunk struct {
	Type string
	Size int
}

func appendLE32(dst []byte, value uint32) []byte {
	return append(dst, byte(value), byte(value>>8), byte(value>>16), byte(value>>24))
}

// sampleWebP 构造 RIFF/WEBP 容器：RIFF 长度按实际字节回填，奇数大小补 1 字节 padding
func sampleWebP(chunks ...riffChunk) []byte {
	body := []byte("WEBP")
	for _, chunk := range chunks {
		body = append(body, chunk.Type...)
		body = appendLE32(body, uint32(chunk.Size))
		body = append(body, make([]byte, chunk.Size)...)
		if chunk.Size%2 == 1 {
			body = append(body, 0)
		}
	}
	// RIFF 长度字段 = 文件总长 - 8，即 "WEBP" 及其后所有 chunk 的字节数
	raw := appendLE32([]byte("RIFF"), uint32(len(body)))
	return append(raw, body...)
}

// patchRIFFLength 按实际字节回填 RIFF 长度字段（= 总长 - 8）
func patchRIFFLength(raw []byte) []byte {
	if len(raw) >= 8 {
		binary.LittleEndian.PutUint32(raw[4:8], uint32(len(raw)-8))
	}
	return raw
}

func TestIsAnimatedWebP(t *testing.T) {
	animated := sampleWebP(riffChunk{"VP8X", 10}, riffChunk{"ANIM", 6}, riffChunk{"ANMF", 16})
	if !IsAnimatedWebP(animated) {
		t.Fatal("带 ANIM 块的 WebP 应判为动图")
	}

	static := sampleWebP(riffChunk{"VP8X", 10}, riffChunk{"VP8 ", 12})
	if IsAnimatedWebP(static) {
		t.Fatal("不带 ANIM 块的 WebP 不应判为动图")
	}

	// 奇数长度 chunk 后有 1 字节 padding，漏掉它就会错位到 ANIM 之前
	oddPadded := sampleWebP(riffChunk{"ICCP", 3}, riffChunk{"ANIM", 6})
	if !IsAnimatedWebP(oddPadded) {
		t.Fatal("奇数长度 chunk 的 padding 未被跳过")
	}
	if _, mimeType := SniffImageFormat(oddPadded); mimeType != "image/webp" {
		t.Fatalf("WebP 嗅探 MIME = %q，期望 image/webp", mimeType)
	}
}

func TestIsAnimatedWebPRejectsMalformedInput(t *testing.T) {
	// 非 RIFF/WEBP：即使 offset 12 处恰好是 ANIM 也不能判为动图
	notRiff := make([]byte, 20)
	copy(notRiff[12:], "ANIM")
	if IsAnimatedWebP(notRiff) {
		t.Fatal("非 RIFF/WEBP 数据不应判为动图")
	}
	if IsAnimatedWebP([]byte("RIFF")) {
		t.Fatal("过短数据不应判为动图")
	}

	// chunk 声明长度超出容器：后面的 ANIM 字节不能被当作有效块
	truncated := sampleWebP(riffChunk{"VP8X", 10})
	truncated = append(truncated, "ICCP"...)
	truncated = appendLE32(truncated, 1000)
	truncated = append(truncated, "ANIM"...)
	truncated = append(truncated, make([]byte, 6)...)
	if IsAnimatedWebP(patchRIFFLength(truncated)) {
		t.Fatal("chunk 声明长度越界的数据不应判为动图")
	}

	// chunk 大小最高位为 1：按无符号读取会直接越界停止，不能回退 offset 继续扫
	overflow := sampleWebP(riffChunk{"VP8X", 10})
	overflow = append(overflow, "VP8 "...)
	overflow = appendLE32(overflow, 0x80000000)
	if IsAnimatedWebP(patchRIFFLength(overflow)) {
		t.Fatal("chunk 长度溢出时不应判为动图")
	}

	// RIFF 声明长度小于实际：写在容器之外的 ANIM 不应被采信
	declaredShort := append([]byte{}, sampleWebP(riffChunk{"VP8X", 10})...)
	declaredShort = appendLE32(declaredShort, 6)
	declaredShort = append(declaredShort, "ANIM"...)
	declaredShort = append(declaredShort, make([]byte, 6)...)
	if IsAnimatedWebP(declaredShort) {
		t.Fatal("超出 RIFF 声明长度的 ANIM 不应被采信")
	}
}
