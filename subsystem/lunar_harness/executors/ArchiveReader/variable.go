package ArchiveReader

// variable.go 归档解析智能体的常量、扩展名表与工具名（业务逻辑不得写在本文件）

const (
	// maxEntryReadChars 单个压缩包条目读取的字符上限
	maxEntryReadChars = 10000
	// maxEntryReadBytes 单个压缩包条目读取的字节上限
	maxEntryReadBytes = 4 << 20
	// maxListEntries 压缩包结构查询最多返回的条目数
	maxListEntries = 500
	// maxPreviewEntries 摘要里最多展示的条目数
	maxPreviewEntries = 80
)

// 工具名
const (
	// ToolList 压缩包文件结构查询
	ToolList = "archive_list"
	// ToolRead 指定压缩包文件内容读取
	ToolRead = "archive_read"
	// ToolGGUF GGUF 模型元数据解析
	ToolGGUF = "gguf_metadata"
)

// archiveExts 压缩包扩展名（按 Lunar Harness 原生 zip 机制处理）
var archiveExts = map[string]bool{
	".zip": true, ".ltpx": true,
}
