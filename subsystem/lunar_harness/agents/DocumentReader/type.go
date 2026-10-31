package DocumentReader

// type.go 文献解析智能体的类型定义。
//
// 能力边界：只处理文本类资源（文档 / 表格 / 源码 / Markdown / 日志 / 配置等）。
// 压缩包与 GGUF 等二进制容器交给归档解析智能体，二进制文档不做文本扫描。

import "LunarSubsystem/LunarHarness/engine/resource"

// outlineItem 文件结构大纲的一项（行区间 + 内容标签）
type outlineItem struct {
	// Start 起始行（1 基）
	Start int
	// End 结束行（含）
	End int
	// Level 标题层级（0 表示无层级）
	Level int
	// Label 内容标签（标题 / 声明）
	Label string
}

// fileReference 文件引用关系的一项
type fileReference struct {
	// Line 引用所在行（1 基）
	Line int
	// Raw 引用原文
	Raw string
	// Target 引用目标原文
	Target string
	// Resolved 相对路径解析结果
	Resolved string
	// Exists 目标文件是否存在于本机
	Exists bool
}

// formatProfile 自动识别出的内容格式画像
type formatProfile struct {
	// Ext 小写扩展名（含点，可空）
	Ext string
	// Kind 格式类别（prose / markdown / source / dts / table / config / log / document / archive / model / text）
	Kind string
	// Label 中文格式名
	Label string
	// Encoding 文本编码（utf-8 / binary）
	Encoding string
	// SizeLabel 规模档位（小型 / 中型 / 大型 / 超大型）
	SizeLabel string
	// Strategy 处理策略（内联 / 分批 …）
	Strategy string
	// SizeBytes 文件字节数
	SizeBytes int64
	// Runes 字符数
	Runes int
	// Lines 行数
	Lines int
	// Binary 是否为不可按文本处理的二进制内容
	Binary bool
	// Large 是否超过单次内联上限（必须分批）
	Large bool
}

// chunkSpan 分批处理的一片的行区间
type chunkSpan struct {
	// Index 片序号（0 基）
	Index int
	// StartLine 起始行（1 基，含）
	StartLine int
	// EndLine 结束行（1 基，含）
	EndLine int
	// Chars 该片字符数（估算值）
	Chars int
}

// scanPlan 主模型给出的轻量扫描计划
type scanPlan struct {
	// Mode 扫描模式：targeted 定向检索 / generic 通用摘要 / direct 直接走原流程
	Mode string
	// Goal 交给轻量扫描模型的检索目标
	Goal string
	// Focus 希望重点关注的词或线索
	Focus []string
}

// scanNote 单个分片的轻量扫描笔记
type scanNote struct {
	// Span 该笔记对应的行区间
	Span chunkSpan
	// Text 扫描模型输出的结构化笔记
	Text string
}

// spanScore 选片时的分片相关度评分
type spanScore struct {
	// Span 分片
	Span chunkSpan
	// Score 相关度（词频 + 大纲命中）
	Score int
}

// docContext 一次文件探查的上下文
type docContext struct {
	// Resource 资源目录条目
	Resource resource.Resource
	// Handle 规范句柄
	Handle string
	// AbsPath 正文绝对路径
	AbsPath string
	// Text 正文文本（二进制内容为空）
	Text string
	// Lines 按行切分的正文
	Lines []string
	// Truncated 正文是否因过大被截断
	Truncated bool
	// Outline 结构大纲
	Outline []outlineItem
	// References 引用关系
	References []fileReference
	// Profile 自动识别的内容格式
	Profile formatProfile
	// Chunks 分批处理计划
	Chunks []chunkSpan
}
