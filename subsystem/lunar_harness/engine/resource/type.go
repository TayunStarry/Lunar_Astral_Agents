package resource

// type.go 资源句柄系统（资源目录 + 句柄文法 + 取回 / 入库 / 同步）的类型定义
//
// 设计依据：docs/adr/0001-统一资源寻址与三层存储.md
// 三层职责：
//   资源目录（store.go，SQLite archive.db）：句柄、元数据、段落结构
//   内容仓库（admit.go，文件系统 multimedia/ / audios/ / documents/）：资源原始字节
//   记忆库（engine/memory）：会话经历 + 每资源一条卡片向量
//
// 句柄文法（唯一实现，Go 侧为权威定义）：
//   resource-handle := "@" typecode "." seq3 [ anchor ]
//   typecode        := "D" | "I" | "V" | "A" | "T" | "U"
//   anchor          := "|§" section-ordinal | "|L" line-start "-" line-end

import "database/sql"

// Resource 资源目录中的一行（模型与前端可见的资源视图）
type Resource struct {
	ID           int64    `json:"id"`
	Kind         string   `json:"kind"`   // 类型码 D/I/V/A/T/U
	Seq          int64    `json:"seq"`    // 资源序号（句柄主体）
	Handle       string   `json:"handle"` // 规范句柄文本，形如 @D.003
	Name         string   `json:"name"`   // 原始文件名（可空）
	Ext          string   `json:"ext"`    // 小写扩展名（含点，可空）
	MIME         string   `json:"mime"`
	ContentHash  string   `json:"content_hash"` // sha16（SHA-256 前 16 位十六进制）
	BlobPath     string   `json:"blob_path"`    // 相对 LocalDir 的物理路径（内容仓库位置）
	URL          string   `json:"url"`          // 链接地址（仅 @U：网页 / 网络视频 / BV 号）
	SizeBytes    int64    `json:"size_bytes"`
	Width        int      `json:"width,omitempty"`
	Height       int      `json:"height,omitempty"`
	DurationMS   int64    `json:"duration_ms,omitempty"`
	Title        string   `json:"title"`   // 标题（文档取首个标题或文件名）
	Summary      string   `json:"summary"` // 摘要/描述（图片为视觉描述，文档可空）
	Tags         []string `json:"tags"`
	Origin       string   `json:"origin"`        // 来源
	Retain       string   `json:"retain"`        // 保留等级
	State        string   `json:"state"`         // blob 状态 active/lost
	SectionCount int      `json:"section_count"` // 段落数（文档）
	CreatedAt    int64    `json:"created_at"`
	UsedAt       int64    `json:"used_at"`
	// ParseText 该 URL 的唯一解析文本（use_resource 命中即直接返回）
	ParseText string `json:"-"`
	// ParseState 解析状态：none / ready / failed
	ParseState string `json:"-"`
	// BlobOwned 实体是否为「归档私有副本」：true = 归档自己写入、配额回收可清理；
	// false = 前端/引擎写入的实体，回收器不得删除（防止误删用户可见文件）
	BlobOwned bool `json:"blob_owned"`
}

// Section 段落结构表的一行（文档的大纲节点）
type Section struct {
	ID        int64  `json:"id"`
	Resource  int64  `json:"resource_id"`
	Ordinal   int    `json:"ordinal"`    // 章节序号（句柄 |§n 使用）
	Level     int    `json:"level"`      // 标题层级 1..6
	Title     string `json:"title"`      // 标题文本
	Path      string `json:"path"`       // 层级路径（如「一、功能 / 1.2 用法」）
	LineStart int    `json:"line_start"` // 正文起始行（1 基）
	LineEnd   int    `json:"line_end"`   // 正文结束行（含）
	Chars     int    `json:"chars"`      // 正文字数
}

// SectionDraft 写入段落表时的草稿（不含自增 ID 与资源 ID）
type SectionDraft struct {
	Ordinal   int
	Level     int
	Title     string
	Path      string
	LineStart int
	LineEnd   int
	Chars     int
}

// SearchHit 跨资源搜索命中
type SearchHit struct {
	Handle    string    `json:"handle"`
	Kind      string    `json:"kind"`
	Title     string    `json:"title"`
	Summary   string    `json:"summary"`
	Sections  []Section `json:"sections,omitempty"` // 命中章节（文档）
	MatchedIn string    `json:"matched_in"`         // title / summary / tag / section / name
}

// StatsReport 目录统计（GC 与运维视图）
type StatsReport struct {
	Total    int64            `json:"total"`
	ByKind   map[string]int64 `json:"by_kind"`
	ByState  map[string]int64 `json:"by_state"`
	ByRetain map[string]int64 `json:"by_retain"`
	Bytes    int64            `json:"bytes"`
}

// Handle 解析后的资源句柄
type Handle struct {
	// Kind 类型码（D/I/V/A/T/U）
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

// ReadResult 资源取回结果
type ReadResult struct {
	// Handle 规范句柄（含锚点，若请求带锚点）
	Handle string `json:"handle"`
	// Kind 类型码
	Kind string `json:"kind"`
	// Title 展示标题
	Title string `json:"title"`
	// Content 文本内容（文档 / 文本类资源；携带锚点时为其行区间）
	Content string `json:"content,omitempty"`
	// Base64 二进制内容（图片 / 视频 / 音频本体）
	//
	// 图片走思维链时**不是**原图：lunarloop 的资源取回适配器会把 KindImage 换成
	// 视觉读取副本（≤ vision_max_side）再填这里，原图字节不出 Go 侧（见 docs/adr/0008）
	Base64 string `json:"base64,omitempty"`
	// MIME 规范 MIME
	MIME string `json:"mime"`
	// Width / Height 图片副本的像素尺寸（仅图片取回时填写，供调用方取比例基准）
	Width  int `json:"width,omitempty"`
	Height int `json:"height,omitempty"`
	// BlobPath 内容仓库路径（相对 LocalDir，可交给图片处理管线取回）
	BlobPath string `json:"blob_path"`
	// Lost blob 已丢失：句柄有效但本体不可取，调用方应降级为描述
	Lost bool `json:"lost"`
	// Truncated 内容按上限被截断
	Truncated bool `json:"truncated"`
	// SectionTitle 命中的章节标题（携带锚点时）
	SectionTitle string `json:"section_title,omitempty"`
	// LineStart / LineEnd 实际返回的行区间（1 基，含端点）
	LineStart int `json:"line_start,omitempty"`
	LineEnd   int `json:"line_end,omitempty"`
	// TotalLines 资源总行数（文本文档）
	TotalLines int `json:"total_lines,omitempty"`
}

// AdmitInput 入库请求
type AdmitInput struct {
	// SourceKind 来源类型：path / url / data / bytes
	SourceKind string
	// Source 来源值（路径 / 地址 / data URI）
	Source string
	// Data 原始字节（SourceKind 为 bytes 时使用）
	Data []byte
	// Name 建议文件名（可空：path 来源自动取基名）
	Name string
	// KindHint 类型码建议值（可空：按扩展名与 MIME 推断）
	KindHint string
	// Origin 来源语义（client-upload / channel / tool-output / engine-temp）
	Origin string
	// Retain 保留等级建议值（可空：按 origin 推断）
	Retain string
	// MIME 建议 MIME（可空：按扩展名推断）
	MIME string
	// Dedup 按内容去重：为 true 时，若已存在相同类型码 + 相同内容哈希的资源，
	// 直接复用该资源句柄，**不再新增目录行**（blob 本就内容寻址，天然同一份）。
	//
	// 为什么需要它：本层默认「每次入库一行、序号永不复用」，同一张图被反复发送
	// 就会一路占出 @I.002 / @I.003 / @I.004…。图片没有文件名这类业务身份，
	// 内容哈希就是它唯一的身份，因此由调用方显式要求按内容去重。
	Dedup bool
	// MustStore 强制写入指定目录（仅对「新内容」生效）。
	// 内容哈希已存在时仍以唯一性为准：若目标路径优先级更高（生成目录 > 根目录），
	// 会把既有句柄迁移到目标路径并删除旧实体；否则直接复用既有路径。
	MustStore bool
}

// AdmitResult 入库结果
type AdmitResult struct {
	// Handle 规范句柄（形如 @I.007）
	Handle string `json:"handle"`
	// Kind 类型码
	Kind string `json:"kind"`
	// Seq 资源序号
	Seq int64 `json:"seq"`
	// ID 资源目录自增主键
	ID int64 `json:"id"`
	// ContentHash sha16
	ContentHash string `json:"content_hash"`
	// BlobPath 相对 LocalDir 的物理路径（内容仓库位置，斜杠分隔）
	BlobPath string `json:"blob_path"`
	// URL 该资源的目标 URL（本地资源为 /file/read/<rel>）
	URL string `json:"url"`
	// MIME 规范化 MIME
	MIME string `json:"mime"`
	// SizeBytes 字节数
	SizeBytes int64 `json:"size_bytes"`
	// Name 原始文件名
	Name string `json:"name"`
	// Deduped 是否复用了既有 blob（内容已存在）
	Deduped bool `json:"deduped"`
	// State blob 状态
	State string `json:"state"`
}

// URLAdmitInput 链接资源登记请求
type URLAdmitInput struct {
	// URL 规范链接（网页地址或 B 站视频地址）
	URL string
	// Title 标题（可空：优先缓存标题，其次 URL）
	Title string
	// Summary 摘要（可空：按 URL / BV 号查已缓存的观影感受或网页摘要）
	Summary string
	// Name 展示名（可空：取标题或 URL）
	Name string
	// Origin 来源语义（channel / tool-output / ...）
	Origin string
	// KindHint 类型码建议值（可空：按 URL 形态推断 V / G / U）
	KindHint string
}

// VisionCacheEntry 图片缩影缓存条目（统一缓存表中的 image-thumbnail 一类）
type VisionCacheEntry struct {
	// SourceURL 源图片对应的本地 URL（/file/read/<rel>）
	SourceURL string
	// Side 副本最大边长（像素）
	Side int
	// Tag 叠印的句柄文本（@I.003）
	Tag string
	// CachePath 副本相对 LocalDir 路径
	CachePath string
	// Bytes 副本字节数
	Bytes int64
}

// CacheEntry 统一缓存表的一行（视频切片 / 图片缩影）
type CacheEntry struct {
	Kind      string
	SourceURL string
	Variant   string
	Path      string
	Bytes     int64
}

// store 统一句柄库（SQLite handles.db）
// 职责：URL 主表（URL ⇄ 解析文本唯一绑定）、六类句柄映射、段落结构、统一缓存。
type store struct {
	db          *sql.DB // 句柄库 SQL 连接
	initialized bool    // 是否初始化完成
}
