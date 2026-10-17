package module

// archive_type.go 统一资源归档层的类型定义
//
// 设计依据：docs/adr/0001-统一资源寻址与三层存储.md
// 三层职责：
//   资源目录（本文件 + archive_store.go，SQLite archive.db）：句柄、元数据、段落结构
//   内容仓库（archive_admit.go，文件系统 multimedia/）：资源原始字节
//   记忆库（memory.go，向量库）：会话经历 + 每资源一条卡片向量
//
// 句柄文法（唯一实现，Go 侧为权威定义）：
//   resource-handle := "@" typecode "." seq3 [ anchor ]
//   typecode        := "D" | "I" | "V" | "A" | "T"
//   anchor          := "|§" section-ordinal | "|L" line-start "-" line-end

import (
	"regexp"
	"strconv"
	"strings"
)

// 资源类型码
const (
	KindDocument = "D" // 文档（文本类文件：源码 / Markdown / 纯文本 / 配置）
	KindImage    = "I" // 图片
	KindVideo    = "V" // 视频
	KindAudio    = "A" // 音频
	KindTable    = "T" // 表格（预留）
)

// 资源生命周期状态（blob 存在性）
const (
	// StateActive blob 在位，可直接取回本体
	StateActive = "active"
	// StateLost blob 已被回收或缺失，句柄仍可解析，取用时降级为描述/大纲
	StateLost = "lost"
)

// 保留等级（决定 GC 顺序，等级相同再按最久未用）
const (
	// RetainNormal A 级：用户主动提供，永久保留
	RetainNormal = "normal"
	// RetainSoft B 级：引擎产出，可被回收（回收后降级为描述）
	RetainSoft = "soft"
	// RetainEphemeral C 级：临时参考物（视频抽帧、GIF 中间帧），最优先回收
	RetainEphemeral = "ephemeral"
)

// 资源来源（决定默认保留等级与入库行为）
const (
	OriginClientUpload = "client-upload" // 前端上传（已落盘，只登记不复制）
	OriginChannel      = "channel"       // QQ 等外部渠道（仅 URL，需下载落盘）
	OriginToolOutput   = "tool-output"   // 工具产出（截图 / 绘图 / 超分）
	OriginEngineTemp   = "engine-temp"   // 引擎临时媒体（视频分段 / 抽帧）
)

// 句柄文法正则：唯一权威定义，TypeScript 与前端通过 resourceGrammar 取用，禁止各处自写
const (
	// HandlePattern 完整句柄（含可选锚点）
	HandlePattern = `@([DIVAT])\.(\d{3,})(?:\|(?:§(\d+)|L(\d+)-(\d+)))?`
	// SeqPadWidth 句柄序号最小宽度（不足补零）
	SeqPadWidth = 3
	// seqOverflowWidth 序号超出三位后的宽度（999 之后自然变为四位）
	seqOverflowWidth = 4
)

var handleRegexp = regexp.MustCompile("^" + HandlePattern + "$")

// Handle 解析后的资源句柄
type Handle struct {
	// Kind 类型码（D/I/V/A/T）
	Kind string
	// Seq 资源序号（持久化主键，永不复用）
	Seq int64
	// SectionOrdinal 章节序号（|§n 形态，0 表示未指定）
	SectionOrdinal int
	// LineStart / LineEnd 行区间（|L120-168 形态，0 表示未指定）
	LineStart int
	LineEnd   int
	// HasSection 是否携带 § 锚点
	HasSection bool
	// HasLine 是否携带 L 锚点
	HasLine bool
}

// Text 输出规范句柄文本（不含锚点）
func (h Handle) Text() string {
	return FormatHandle(h.Kind, h.Seq)
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
func ParseHandle(raw string) (Handle, bool) {
	m := handleRegexp.FindStringSubmatch(strings.TrimSpace(raw))
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
