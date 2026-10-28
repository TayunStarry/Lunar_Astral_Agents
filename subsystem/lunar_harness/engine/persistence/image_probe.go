package persistence

import (
	"encoding/base64"
	"fmt"
	"strings"
)

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
		comma := strings.Index(s, ",")
		if comma < 0 {
			return "", nil, fmt.Errorf("data URI 格式非法（缺少逗号分隔）")
		}
		header := s[:comma]
		mimeType = strings.TrimPrefix(strings.Split(header, ";")[0], "data:")
		raw, err = base64.StdEncoding.DecodeString(s[comma+1:])
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
