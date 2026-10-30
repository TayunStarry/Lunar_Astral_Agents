package FileEditor

// variable.go 文件编辑智能体的常量、工具名与工具分组。

import (
	"net/http"
	"sync"
	"time"
)

const (
	// Module 日志模块名（日志分类归入「生活」）
	Module = "文件编辑"
	// documentsRootRel 文件操作根目录（相对 LocalDir）
	documentsRootRel = "documents"
	// maxFileReadBytes 单次文件读取上限
	maxFileReadBytes = 256 << 10
	// fileReadChunkRunes 单次 file_read 返回给模型的默认字符数
	fileReadChunkRunes = 4000
	// fileReadChunkLimit 单次 file_read 允许的最大字符数
	fileReadChunkLimit = 8000
	// searchHitLimit 内容检索命中上限
	searchHitLimit = 60
	// searchScanBytes 目录检索时单文件扫描上限
	searchScanBytes = 256 << 10
	// toolMessageMaxRunes 单条工具结果回填模型的最大字符数
	toolMessageMaxRunes = 6000
	// maxRounds 单条指令的工具循环上限
	maxRounds = 10
	// maxRepeats 同一工具调用重复达到该次数即判定循环输出
	maxRepeats = 3
	// modelTimeout 单次模型调用超时
	modelTimeout = 90 * time.Second
	// runTimeout 整轮文件编辑的总超时
	runTimeout = 15 * time.Minute
	// defaultRunLimit 项目脚本的默认运行时长上限。
	// 私有 js 运行时可能因事件订阅长期驻留，超过该时长后自动卸载实例。
	defaultRunLimit = 120 * time.Second
	// minRunLimit 允许的最短运行时长（防止 AI 设成无法完成加载的值）
	minRunLimit = 5 * time.Second
	// maxRunLimit 允许的最长运行时长
	maxRunLimit = 3600 * time.Second
)

// ==== 工具名 ====

const (
	ToolList     = "file_list"
	ToolRead     = "file_read"
	ToolWrite    = "file_write"
	ToolCreate   = "file_create"
	ToolDelete   = "file_delete"
	ToolCopy     = "file_copy"
	ToolMove     = "file_move"
	ToolRename   = "file_rename"
	ToolSearch   = "file_search"
	ToolReplace  = "file_replace_at"
	ToolInfo     = "file_info"
	ToolWeb      = "web_search"
	ToolMemory   = "memory_search"
	ToolHandleSr = "handle_search"
	ToolHandleRd = "handle_read"
	ToolRunProj  = "run_project"
)

// allTools 全部工具（供自然语言模式与工具列表屏展示）。
var allTools = []string{
	ToolList, ToolRead, ToolWrite, ToolCreate, ToolDelete, ToolCopy, ToolMove,
	ToolRename, ToolSearch, ToolReplace, ToolInfo, ToolWeb, ToolMemory,
	ToolHandleSr, ToolHandleRd, ToolRunProj,
}

var (
	// runMu 串行化自然语言文件编辑，避免同一文档库并发写入
	runMu sync.Mutex
	// httpClient 模型调用客户端
	httpClient = &http.Client{Timeout: modelTimeout}
)
