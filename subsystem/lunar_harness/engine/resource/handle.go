package resource

// handle.go 句柄文法的权威实现（解析 / 格式化 / 类型判定 / 全文提取）
//
// 任何「这串文本是不是句柄」「类型码是什么意思」「该按文本还是二进制取回」
// 的判定都必须走这里，禁止在调用方另写一份正则或标签表。

import (
	"strconv"
	"strings"
)

// Text 输出规范句柄文本（不含锚点）
func (h Handle) Text() string {
	return FormatHandle(h.Kind, h.Seq)
}

// FullText 输出含锚点的完整句柄文本（用于回显与日志）
func (h Handle) FullText() string {
	text := h.Text()
	switch {
	case h.HasSection && h.SectionOrdinal > 0:
		return text + "|§" + strconv.Itoa(h.SectionOrdinal)
	case h.HasLine && h.LineStart > 0:
		end := h.LineEnd
		if end < h.LineStart {
			end = h.LineStart
		}
		return text + "|L" + strconv.Itoa(h.LineStart) + "-" + strconv.Itoa(end)
	}
	return text
}

// FormatHandle 按句柄文法格式化资源句柄
func FormatHandle(kind string, seq int64) string {
	return "@" + kind + "." + padSeq(seq)
}

// padSeq 序号补零：三位起，超出三位按实际宽度输出（999 之后自然四位）
func padSeq(seq int64) string {
	width := SeqPadWidth
	if seq >= 1000 {
		width = seqOverflowWidth
	}
	s := strconv.FormatInt(seq, 10)
	for len(s) < width {
		s = "0" + s
	}
	return s
}

// ParseHandle 解析句柄文本，失败返回 false
//
// 容忍首尾空白与方括号 / 中文书名号包裹（模型经常把句柄包起来引用）。
func ParseHandle(raw string) (Handle, bool) {
	text := strings.TrimSpace(raw)
	text = strings.TrimPrefix(text, "[")
	text = strings.TrimPrefix(text, "【")
	text = strings.TrimSuffix(text, "]")
	text = strings.TrimSuffix(text, "】")
	m := handleExactRegexp.FindStringSubmatch(text)
	if m == nil {
		return Handle{}, false
	}
	h := Handle{Kind: m[1]}
	h.Seq = parseInt64(m[2])
	if m[3] != "" {
		h.HasSection = true
		h.SectionOrdinal = int(parseInt64(m[3]))
	} else if m[4] != "" {
		h.HasLine = true
		h.LineStart = int(parseInt64(m[4]))
		h.LineEnd = int(parseInt64(m[5]))
	}
	return h, true
}

// ExtractHandles 从文本中提取全部规范句柄（按出现顺序，已去空白与包裹符）
func ExtractHandles(text string) []string {
	if text == "" {
		return nil
	}
	found := make([]string, 0)
	for _, match := range handleGlobalRegexp.FindAllString(text, -1) {
		if handle, ok := ParseHandle(match); ok && handle.Seq > 0 {
			found = append(found, handle.FullText())
		}
	}
	return found
}

// parseInt64 解析十进制整数（非法输入返回 0）
func parseInt64(s string) int64 {
	var v int64
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return v
		}
		v = v*10 + int64(c-'0')
	}
	return v
}

// KindOfExt 按扩展名推断资源类型码，未知返回空字符串
func KindOfExt(ext string) string {
	switch strings.ToLower(strings.TrimPrefix(ext, ".")) {
	case "png", "jpg", "jpeg", "webp", "gif", "bmp", "tiff", "tif", "svg", "ico", "apng":
		return KindImage
	case "mp4", "webm", "avi", "mov", "mkv", "wmv", "flv", "m4v", "mpg", "mpeg":
		return KindVideo
	case "wav", "mp3", "flac", "ogg", "m4a", "aac", "opus", "wma":
		return KindAudio
	case "csv", "tsv", "xlsx", "xls":
		return KindTable
	case "txt", "md", "markdown", "json", "xml", "yaml", "yml", "toml", "ini", "cfg",
		"log", "go", "ts", "tsx", "js", "jsx", "py", "rs", "java", "c", "h", "cpp", "cxx",
		"hpp", "cs", "rb", "sh", "ps1", "bat", "sql", "html", "htm", "css", "vue", "kt",
		"swift", "php", "lua", "r", "m", "mm", "scala", "pl", "dart", "proto", "gradle":
		return KindDocument
	}
	return ""
}

// KindLabel 类型码的中文名（未知类型原样返回）
func KindLabel(kind string) string {
	if label, ok := kindLabels[kind]; ok {
		return label
	}
	return kind
}

// IsTextKind 判断资源是否按文本取回（文档 / 表格 / 链接）
//
// 链接只含地址与摘要，同样按文本直接取回。
func IsTextKind(kind string) bool {
	return kind == KindDocument || kind == KindTable || kind == KindURL
}

// DefaultRetain 按来源推断默认保留等级
func DefaultRetain(origin string) string {
	switch origin {
	case OriginClientUpload:
		return RetainNormal
	case OriginToolOutput:
		return RetainSoft
	case OriginEngineTemp:
		return RetainEphemeral
	default:
		return RetainSoft
	}
}

// IsMediaExtension 判断扩展名是否属媒体类（图片 / 视频 / 音频）
func IsMediaExtension(ext string) bool {
	return mediaExtensions[strings.ToLower(ext)]
}

// IsStoragePath 判断相对路径是否位于三棵内容仓库树内
func IsStoragePath(rel string) bool {
	rel = normalizeRel(rel)
	for _, root := range StorageRoots {
		if rel == root || strings.HasPrefix(rel, root+"/") {
			return true
		}
	}
	return false
}

// IsDerivedStoragePath 判断相对路径是否位于派生缓存子树（不参与句柄索引）
func IsDerivedStoragePath(rel string) bool {
	rel = normalizeRel(rel)
	for _, root := range DerivedStorageRoots {
		if rel == root || strings.HasPrefix(rel, root+"/") {
			return true
		}
	}
	return false
}
