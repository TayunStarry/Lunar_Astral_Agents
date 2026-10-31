package DocumentReader

// variable.go 文献解析智能体的常量与识别规则（业务逻辑不得写在本文件）

import "regexp"

const (
	// roleLabel TUI 实况屏显示的角色标签
	roleLabel = "文献解析"
	// summaryStoreChars 写回资源目录的摘要最大字符数
	summaryStoreChars = 1000
	// maxToolRounds 文件探查的最大工具轮次（超长文档需要更多分片轮次）
	maxToolRounds = 8
	// inlineContentLimit 初始消息内联正文的最大字符数（超过即必须分批处理）
	inlineContentLimit = 10000
	// maxToolReadLines 单次分片读取的最大行数
	maxToolReadLines = 300
	// maxToolReadChars 单次分片读取的最大字符数（与 inlineContentLimit 对齐）
	maxToolReadChars = 10000
	// chunkRunes 分批处理的单片字符上限
	chunkRunes = 10000
	// maxDocRunes 单个文件载入内存的最大字符数（防御超大文件）
	maxDocRunes = 8000000
	// maxDocBytes 单个文件载入内存的最大字节数
	maxDocBytes = 64 << 20
	// maxOutlineItems 结构大纲最多返回项数
	maxOutlineItems = 200
	// maxOutlineScanLines 结构识别最多扫描的行数（超大型文件的保护上限）
	maxOutlineScanLines = 200000
	// maxReferenceItems 引用关系最多返回项数
	maxReferenceItems = 80
	// maxChunkPlanItems 分批计划最多返回的片数
	maxChunkPlanItems = 200
	// largeRunes 大型文档阈值
	largeRunes = 200000
	// hugeRunes 超大型文档阈值
	hugeRunes = 2000000
	// profileHeadBytes 内容结构识别采样的字节数
	profileHeadBytes = 4096

	// ==== 轻量扫描通道 ====
	// scanRoleLabel 轻量扫描阶段的实况角色标签
	scanRoleLabel = "文献解析·扫描"
	// scanMinChunks 少于该片数时不值得切换模型，直接走原直读流程
	scanMinChunks = 5
	// scanMergeRounds 主模型归并笔记时允许的工具轮次（用于回读原文核对）
	scanMergeRounds = 4
	// scanNoteMaxTokens 单片扫描的最大输出 token
	scanNoteMaxTokens = 1200
	// scanNoteMaxChars 单片笔记在归并时的最大字符数
	scanNoteMaxChars = 800
	// scanMergeNotesChars 归并时注入全部笔记的总字符上限
	scanMergeNotesChars = 20000
	// scanFallbackChars 归并失败时直接回吐笔记的总字符上限
	scanFallbackChars = 12000
	// scanPlanMaxChars 扫描计划文本的最大字符数
	scanPlanMaxChars = 800
	// scanFocusMaxItems 扫描计划中关注词的最大数量
	scanFocusMaxItems = 8
)

// 轻量扫描模式
const (
	// scanModeTargeted 按需求做定向检索与提取
	scanModeTargeted = "targeted"
	// scanModeGeneric 无明确目标时做通用摘要
	scanModeGeneric = "generic"
	// scanModeDirect 本次不值得走轻量扫描，回退原直读流程
	scanModeDirect = "direct"
)

// 文献解析工具名
const (
	ToolSummary    = "file_summary"
	ToolOutline    = "file_outline"
	ToolReadLines  = "read_lines"
	ToolReferences = "file_references"
	ToolProfile    = "file_profile"
	ToolChunkPlan  = "chunk_plan"
	ToolReadChunk  = "read_chunk"
)

// 引用关系识别规则
var (
	// reJSImport import/export ... from "x"
	reJSImport = regexp.MustCompile(`(?i)\b(?:import|export)\b[^'"\n]*\bfrom\s*['"]([^'"]+)['"]`)
	// reRequire require("x")
	reRequire = regexp.MustCompile(`(?i)\brequire\s*\(\s*['"]([^'"]+)['"]\s*\)`)
	// rePyFrom from x import ...
	rePyFrom = regexp.MustCompile(`^\s*from\s+([A-Za-z0-9_.]+)\s+import\b`)
	// rePyImport import x
	rePyImport = regexp.MustCompile(`^\s*import\s+([A-Za-z0-9_.]+)`)
	// reInclude #include "x"
	reInclude = regexp.MustCompile(`^\s*#\s*include\s+["<]([^">]+)[">]`)
	// reQuoted 提取引号内文本（Go import 块）
	reQuoted = regexp.MustCompile(`["']([^"']+)["']`)
	// reMDLink Markdown 链接与图片
	reMDLink = regexp.MustCompile(`!?\[[^\]]*\]\(\s*([^)\s]+)`)
	// reHTMLLink HTML src/href
	reHTMLLink = regexp.MustCompile(`(?i)\b(?:src|href)\s*=\s*["']([^"']+)["']`)
	// reCSSImport CSS @import
	reCSSImport = regexp.MustCompile(`(?i)@import\s+(?:url\()?\s*["']?([^"')\s]+)`)
)

// 结构大纲识别规则
var (
	// reMDHeading Markdown 标题
	reMDHeading = regexp.MustCompile(`^(#{1,6})\s+(.+?)\s*$`)
	// reNovelHeading 小说章节标题（第X章 / 第X回 / 卷X / Chapter N）
	reNovelHeading = regexp.MustCompile(`^(?:第[0-9一二三四五六七八九十百千]+[章节回卷篇]|卷[0-9一二三四五六七八九十百千]+|(?:Chapter|CHAPTER)\s+\d+)`)
	// reGoPackage Go package 声明
	reGoPackage = regexp.MustCompile(`^package\s+[A-Za-z_][A-Za-z0-9_]*`)
	// reGoFunc Go 函数/方法
	reGoFunc = regexp.MustCompile(`^func\s+(?:\([^)]*\)\s*)?([A-Za-z_][A-Za-z0-9_]*)`)
	// reGoDecl Go type/var/const
	reGoDecl = regexp.MustCompile(`^(type|var|const)\s+([A-Za-z_][A-Za-z0-9_]*)?`)
	// reTSDecl TypeScript/JavaScript 顶层声明
	reTSDecl = regexp.MustCompile(`^(?:export\s+)?(?:default\s+)?(?:async\s+)?(?:function|class|interface|type|enum|const|let|var)\s+([A-Za-z_$][A-Za-z0-9_$]*)`)
	// reTSInterface .d.ts 接口 / 类型声明
	reTSInterface = regexp.MustCompile(`^(?:export\s+)?(?:declare\s+)?(?:interface|type|namespace|const|function|class)\s+([A-Za-z_$][A-Za-z0-9_$]*)`)
	// rePyDecl Python 顶层定义
	rePyDecl = regexp.MustCompile(`^(?:async\s+)?(?:def|class)\s+([A-Za-z_][A-Za-z0-9_]*)`)
	// reRustDecl Rust 顶层定义
	reRustDecl = regexp.MustCompile(`^(?:pub\s+)?(?:fn|struct|enum|impl|mod|trait)\s+([A-Za-z_][A-Za-z0-9_]*)`)
	// reCDecl C/C++/Java/C# 顶层类型
	reCDecl = regexp.MustCompile(`^(?:class|struct|namespace)\s+([A-Za-z_][A-Za-z0-9_]*)`)
)
