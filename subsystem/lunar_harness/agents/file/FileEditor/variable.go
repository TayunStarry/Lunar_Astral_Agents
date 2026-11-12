package FileEditor

// variable.go 文档库读写工具库的常量与操作名。

import (
	"time"

	"LunarSubsystem/LunarHarness/protocol/agentinfo"
)

const (
	// Module 日志模块名（日志分类归入「生活」）
	Module = agentinfo.NameFileEditor
	// documentsRootRel 文件操作根目录（相对 LocalDir）
	documentsRootRel = "documents"
	// maxFileReadBytes 单次文件读取上限
	maxFileReadBytes = 256 << 10
	// libraryHostWindowID 资源库窗口标识（浏览器端代码的主页面，已存在时重载）
	libraryHostWindowID = "library"
	// libraryHostWindowWidth 资源库窗口宽度
	libraryHostWindowWidth = 1180
	// libraryHostWindowHeight 资源库窗口高度
	libraryHostWindowHeight = 820
	// fileReadChunkRunes 单次 file_read 返回给模型的默认字符数
	fileReadChunkRunes = 4000
	// fileReadChunkLimit 单次 file_read 允许的最大字符数
	fileReadChunkLimit = 8000
	// searchHitLimit 内容检索命中上限
	searchHitLimit = 60
	// searchScanBytes 目录检索时单文件扫描上限
	searchScanBytes = 256 << 10
	// runTimeout 整轮项目运行的总超时
	runTimeout = 15 * time.Minute
)

// ==== 工具名 ====

const (
	ToolList    = "file_list"
	ToolRead    = "file_read"
	ToolWrite   = "file_write"
	ToolCreate  = "file_create"
	ToolDelete  = "file_delete"
	ToolCopy    = "file_copy"
	ToolMove    = "file_move"
	ToolRename  = "file_rename"
	ToolSearch  = "file_search"
	ToolReplace = "file_replace_at"
	ToolAppend  = "file_append"
	ToolInfo    = "file_info"
	ToolRunProj = "run_project"
)
