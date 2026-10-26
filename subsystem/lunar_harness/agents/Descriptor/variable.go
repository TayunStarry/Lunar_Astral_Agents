package Descriptor

// variable.go 描述者的全局变量与常量（业务逻辑不得写在本文件）

import (
	"net/http"
	"regexp"
	"sync"
	"time"
)

const (
	// roleLabel TUI 实况屏显示的角色标签
	roleLabel = "描述者"
	// maxSegments 单次观看最多评估的片段/分组数（超出部分等距抽样）
	maxSegments = 8
	// summaryThreshold 拼接理解文本超过该字符数时触发客观摘要
	summaryThreshold = 4096
	// summaryStoreChars 写回资源目录的摘要最大字符数
	summaryStoreChars = 1000
	// maxToolRounds 文件探查的最大工具轮次
	maxToolRounds = 6
	// inlineContentLimit 初始消息内联正文的最大字符数（超出则让模型用工具分片读取）
	inlineContentLimit = 6000
	// maxToolReadLines 单次 read_lines 返回的最大行数
	maxToolReadLines = 300
	// maxToolReadChars 单次 read_lines 返回的最大字符数
	maxToolReadChars = 12000
	// maxDocRunes 单个文件载入内存的最大字符数（防御超大文件）
	maxDocRunes = 1000000
	// maxOutlineItems 结构大纲最多返回项数
	maxOutlineItems = 120
	// maxReferenceItems 引用关系最多返回项数
	maxReferenceItems = 80
	// mediaRetryWait 影像理解失败后的重试等待
	mediaRetryWait = 10 * time.Second
)

var (
	// modelOnce 模型配置只解析一次
	modelOnce sync.Once
	// modelName 多模态模型名
	modelName string
	// modelURL 模型服务地址（以 /v1 结尾）
	modelURL string
	// modelKey 模型服务密钥
	modelKey string
	// httpClient 模型请求客户端（影像理解可能持续较久）
	httpClient = &http.Client{Timeout: 10 * time.Minute}

	// observerMu 保护实况观察者
	observerMu sync.Mutex
	// observerBegin 实况开始回调
	observerBegin func(role string)
	// observerDelta 实况增量回调（kind: think/content）
	observerDelta func(role, kind, text string)
	// observerEnd 实况结束回调
	observerEnd func(role string)
	// observerMeter 模型调用计量回调（运行状态屏的模型计量）
	observerMeter func(prompt, completion, cached int64, generation time.Duration, estimated bool)

	// promptMu 保护提示词缓存
	promptMu sync.Mutex
	// promptCache 提示词缓存（文件名 → 文本）
	promptCache = map[string]string{}

	// runMu 串行化描述者运行（共享模型通道与实况流）
	runMu sync.Mutex
	// toolCallSeq 为缺失 ID 的工具调用分配唯一序号
	toolCallSeq int64
	// placeholderPattern 提示词占位符形态（{key}）
	placeholderPattern = regexp.MustCompile(`\{[A-Za-z0-9_\-]+\}`)
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
	// reGoPackage Go package 声明
	reGoPackage = regexp.MustCompile(`^package\s+[A-Za-z_][A-Za-z0-9_]*`)
	// reGoFunc Go 函数/方法
	reGoFunc = regexp.MustCompile(`^func\s+(?:\([^)]*\)\s*)?([A-Za-z_][A-Za-z0-9_]*)`)
	// reGoDecl Go type/var/const
	reGoDecl = regexp.MustCompile(`^(type|var|const)\s+([A-Za-z_][A-Za-z0-9_]*)?`)
	// reTSDecl TypeScript/JavaScript 顶层声明
	reTSDecl = regexp.MustCompile(`^(?:export\s+)?(?:default\s+)?(?:async\s+)?(?:function|class|interface|type|enum|const|let|var)\s+([A-Za-z_$][A-Za-z0-9_$]*)`)
	// rePyDecl Python 顶层定义
	rePyDecl = regexp.MustCompile(`^(?:async\s+)?(?:def|class)\s+([A-Za-z_][A-Za-z0-9_]*)`)
	// reRustDecl Rust 顶层定义
	reRustDecl = regexp.MustCompile(`^(?:pub\s+)?(?:fn|struct|enum|impl|mod|trait)\s+([A-Za-z_][A-Za-z0-9_]*)`)
	// reCDecl C/C++/Java/C# 顶层类型
	reCDecl = regexp.MustCompile(`^(?:class|struct|namespace)\s+([A-Za-z_][A-Za-z0-9_]*)`)
)
