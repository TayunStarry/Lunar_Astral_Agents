package resource

// classify_sniff.go 文件头魔数嗅探：只用于校验与存储层归位，不参与入站信封分类
//
// 图片格式复用 persistence.SniffImageFormat（与入库、记忆库同一套规则）；
// 视频 / 音频 / 常见二进制文档在这里补齐。

import (
	"bytes"
	"io"
	"os"

	"LunarSubsystem/LunarHarness/engine/persistence"
)

// readHead 读取本地文件头若干字节（大文件不整读）
func readHead(abs string) ([]byte, error) {
	f, err := os.Open(abs)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	head := make([]byte, classifyHeadBytes)
	n, err := io.ReadFull(f, head)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return nil, err
	}
	return head[:n], nil
}

// sniffMagic 按文件头魔数识别媒体 / 文档类型；无法识别返回 false
func sniffMagic(head []byte) (magicInfo, bool) {
	if len(head) < 4 {
		return magicInfo{}, false
	}
	if ext, mimeType := persistence.SniffImageFormat(head); mimeType != "" {
		return magicInfo{Kind: KindImage, Ext: ext, MIME: mimeType}, true
	}
	switch {
	case bytes.HasPrefix(head, []byte("%PDF-")):
		return magicInfo{Kind: KindDocument, Ext: ".pdf", MIME: "application/pdf"}, true
	case len(head) >= 12 && string(head[0:4]) == "RIFF" && string(head[8:12]) == "AVI ":
		return magicInfo{Kind: KindVideo, Ext: ".avi", MIME: "video/x-msvideo"}, true
	case len(head) >= 12 && string(head[0:4]) == "RIFF" && string(head[8:12]) == "WAVE":
		return magicInfo{Kind: KindAudio, Ext: ".wav", MIME: "audio/wav"}, true
	case string(head[0:4]) == "fLaC":
		return magicInfo{Kind: KindAudio, Ext: ".flac", MIME: "audio/flac"}, true
	case string(head[0:4]) == "OggS":
		return magicInfo{Kind: KindAudio, Ext: ".ogg", MIME: "audio/ogg"}, true
	case len(head) >= 3 && string(head[0:3]) == "ID3":
		return magicInfo{Kind: KindAudio, Ext: ".mp3", MIME: "audio/mpeg"}, true
	case len(head) >= 2 && head[0] == 0xFF && (head[1]&0xE0) == 0xE0:
		return magicInfo{Kind: KindAudio, Ext: ".mp3", MIME: "audio/mpeg"}, true
	case bytes.HasPrefix(head, []byte{0x1A, 0x45, 0xDF, 0xA3}):
		return magicInfo{Kind: KindVideo, Ext: ".webm", MIME: "video/webm"}, true
	case bytes.HasPrefix(head, []byte("#!AMR")):
		return magicInfo{Kind: KindAudio, Ext: ".amr", MIME: "audio/amr"}, true
	case bytes.HasPrefix(head, []byte{0x02, '#', '!', 'S', 'I', 'L', 'K'}):
		return magicInfo{Kind: KindAudio, Ext: ".silk", MIME: "audio/silk"}, true
	case len(head) >= 3 && string(head[0:3]) == "FLV":
		return magicInfo{Kind: KindVideo, Ext: ".flv", MIME: "video/x-flv"}, true
	case bytes.HasPrefix(head, []byte{0x00, 0x00, 0x01, 0xBA}):
		return magicInfo{Kind: KindVideo, Ext: ".mpg", MIME: "video/mpeg"}, true
	case len(head) >= 12 && string(head[4:8]) == "ftyp":
		return sniffISOBMFF(head), true
	case bytes.HasPrefix(head, []byte{0x00, 0x00, 0x01, 0x00}):
		return magicInfo{Kind: KindImage, Ext: ".ico", MIME: "image/x-icon"}, true
	case bytes.HasPrefix(head, []byte("II*\x00")) || bytes.HasPrefix(head, []byte("MM\x00*")):
		return magicInfo{Kind: KindImage, Ext: ".tiff", MIME: "image/tiff"}, true
	}
	return magicInfo{}, false
}

// sniffISOBMFF 按 ftyp brand 区分 MP4 / MOV / M4A / AVIF / HEIC
func sniffISOBMFF(head []byte) magicInfo {
	brand := ""
	if len(head) >= 12 {
		brand = string(head[8:12])
	}
	switch brand {
	case "avif", "avis":
		return magicInfo{Kind: KindImage, Ext: ".avif", MIME: "image/avif"}
	case "heic", "heix", "hevc", "hevx", "mif1", "msf1":
		return magicInfo{Kind: KindImage, Ext: ".heic", MIME: "image/heic"}
	case "M4A ", "M4B ", "mp4a":
		return magicInfo{Kind: KindAudio, Ext: ".m4a", MIME: "audio/mp4"}
	case "qt  ":
		return magicInfo{Kind: KindVideo, Ext: ".mov", MIME: "video/quicktime"}
	}
	return magicInfo{Kind: KindVideo, Ext: ".mp4", MIME: "video/mp4"}
}
