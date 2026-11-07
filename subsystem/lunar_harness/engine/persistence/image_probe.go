package persistence

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"strings"
)

// pngSignature PNG 文件头魔数
var pngSignature = []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}

// maxProbeChunks chunk 扫描上限，避免损坏数据把扫描拖成死循环
const maxProbeChunks = 100

// =============================================================================
// 图片字节探测 — data URI 解析、魔数嗅探、data URI 重建
//
// 这些是纯字节层面的通用能力：入库（archive）、记忆库（engine/memory）等
// 上层模块都按同一套「只信字节、不信扩展名/声明」的规则判断图片格式。
// =============================================================================

// StripDataURI 剥离图片/音频的 data URI 前缀，返回 mime 与原始字节。
// 兼容「data:image/...;base64,<payload>」与裸 base64（按魔数嗅探 mime）。
func StripDataURI(base64Image string) (mimeType string, raw []byte, err error) {
	s := strings.TrimSpace(base64Image)
	if s == "" {
		return "", nil, fmt.Errorf("图片数据为空")
	}
	if strings.HasPrefix(s, "data:") {
		before, after, ok := strings.Cut(s, ",")
		if !ok {
			return "", nil, fmt.Errorf("data URI 格式非法（缺少逗号分隔）")
		}
		header := before
		mimeType = strings.TrimPrefix(strings.Split(header, ";")[0], "data:")
		raw, err = base64.StdEncoding.DecodeString(after)
		if err != nil {
			return "", nil, fmt.Errorf("base64 解码失败: %v", err)
		}
		return mimeType, raw, nil
	}
	// 裸 base64：解码后按魔数嗅探
	raw, err = base64.StdEncoding.DecodeString(s)
	if err != nil {
		return "", nil, fmt.Errorf("base64 解码失败: %v", err)
	}
	return SniffImageMIME(raw), raw, nil
}

// SniffImageFormat 按文件头魔数识别图片真实格式，返回规范扩展名与 MIME。
// 无法识别时返回空串，由调用方决定兜底策略。
//
// 为什么必须嗅探内容：QQ 等渠道经常把 GIF / 动态 WebP 命名为 .png、.jpg，
// data URI 里的 MIME 声明也随之失真。扩展名与声明都不可信，只有字节可信。
func SniffImageFormat(raw []byte) (ext string, mimeType string) {
	switch {
	case len(raw) >= 3 && raw[0] == 0xFF && raw[1] == 0xD8 && raw[2] == 0xFF:
		return ".jpg", "image/jpeg"
	case len(raw) >= 8 && raw[0] == 0x89 && raw[1] == 0x50 && raw[2] == 0x4E && raw[3] == 0x47:
		if IsAnimatedPNG(raw) {
			return ".apng", "image/apng"
		}
		return ".png", "image/png"
	case len(raw) >= 4 && raw[0] == 0x47 && raw[1] == 0x49 && raw[2] == 0x46 && raw[3] == 0x38:
		return ".gif", "image/gif"
	case len(raw) >= 12 && string(raw[0:4]) == "RIFF" && string(raw[8:12]) == "WEBP":
		return ".webp", "image/webp"
	case len(raw) >= 2 && raw[0] == 0x42 && raw[1] == 0x4D:
		return ".bmp", "image/bmp"
	default:
		return "", ""
	}
}

// IsAnimatedPNG 判断 PNG 字节是否携带 acTL（Animation Control）块，即 APNG。
//
// 按 APNG 规范 acTL 必须出现在 IDAT 之前，因此扫到 IDAT/IEND 即可停止；
// 任一 chunk 的声明长度越界视为数据不完整，同样判为非 APNG。
func IsAnimatedPNG(raw []byte) bool {
	if len(raw) < len(pngSignature) || !bytes.HasPrefix(raw, pngSignature) {
		return false
	}
	offset := len(pngSignature)
	for i := 0; i < maxProbeChunks && offset+8 <= len(raw); i++ {
		length := int(binary.BigEndian.Uint32(raw[offset : offset+4]))
		switch string(raw[offset+4 : offset+8]) {
		case "acTL":
			return true
		case "IDAT", "IEND":
			return false
		}
		if offset+12+length > len(raw) {
			return false
		}
		offset += 12 + length
	}
	return false
}

// IsAnimatedWebP 判断 WebP 字节是否携带 ANIM 块（动态 WebP）。
//
// RIFF 的长度字段与 chunk 大小都是小端无符号数：先按 RIFF 声明的容器长度裁出
// 可扫描区间（超过实际字节数则按实际长度），再逐 chunk 前进；chunk 大小为奇数时
// 后随 1 字节 padding。chunk 声明长度超出容器即视为数据不完整，停止扫描。
// 长度一律按 uint64 参与运算，避免 32 位平台上最高位置 1 时溢出成负数。
func IsAnimatedWebP(raw []byte) bool {
	if len(raw) < 12 || string(raw[0:4]) != "RIFF" || string(raw[8:12]) != "WEBP" {
		return false
	}
	fileEnd := 8 + uint64(binary.LittleEndian.Uint32(raw[4:8]))
	if fileEnd > uint64(len(raw)) {
		fileEnd = uint64(len(raw))
	}
	offset := uint64(12)
	for i := 0; i < maxProbeChunks && offset+8 <= fileEnd; i++ {
		chunkType := string(raw[offset : offset+4])
		chunkSize := uint64(binary.LittleEndian.Uint32(raw[offset+4 : offset+8]))
		if chunkType == "ANIM" {
			return true
		}
		if offset+8+chunkSize > fileEnd {
			return false
		}
		offset += 8 + chunkSize
		if chunkSize%2 == 1 {
			offset++
		}
	}
	return false
}

// SniffImageMIME 按文件头魔数嗅探图片 mime（未知格式回退 image/png）。
func SniffImageMIME(raw []byte) string {
	if _, mimeType := SniffImageFormat(raw); mimeType != "" {
		return mimeType
	}
	return "image/png"
}

// RebuildDataURI 重组为完整 data URI；原始串已带前缀且 mime 与内容一致时直接复用，避免无谓的重编码。
func RebuildDataURI(mimeType string, raw []byte, original string) string {
	trimmed := strings.TrimSpace(original)
	if mimeType == "" {
		mimeType = SniffImageMIME(raw)
	}
	if strings.HasPrefix(trimmed, "data:") {
		// 声明 MIME 可能与内容不符（QQ 常把 GIF 动图标成 image/png）：
		// 只有声明与嗅探结果一致时才原样复用，否则按真实格式重建。
		if comma := strings.Index(trimmed, ","); comma > 0 {
			declared := strings.TrimPrefix(strings.Split(trimmed[:comma], ";")[0], "data:")
			if strings.EqualFold(declared, mimeType) {
				return trimmed
			}
		}
	}
	return "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(raw)
}
