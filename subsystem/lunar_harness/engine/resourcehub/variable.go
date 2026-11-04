package resourcehub

// variable.go 统一资源基座的常量与全局状态（业务逻辑不得写在本文件）。

import (
	"sync"
	"time"
)

const (
	// TypeMemory 检索长期记忆
	TypeMemory = "memory"
	// TypeHandle 按句柄读取内容（非句柄输入会自动解析为句柄）
	TypeHandle = "handle"
	// TypePath 按文件名或路径读取内容（解析并登记为句柄后读取）
	TypePath = "path"
	// TypeList 列出符合标准的句柄数组
	TypeList = "list"

	// defaultLimit 未指定 limit 时的默认返回数量
	defaultLimit = 5
	// memoryMaxResults 记忆检索的返回上限
	memoryMaxResults = 30
	// listDefaultLimit 列表模式的默认返回数量
	listDefaultLimit = 50
	// listMaxLimit 列表模式的返回上限
	listMaxLimit = 200

	// indexTTL 目录索引的最长复用时间（外部写入可能绕过显式失效）
	indexTTL = 3 * time.Second
	// maxCandidateDisplay 歧义提示里最多列出的候选条数
	maxCandidateDisplay = 12

	// lostFileNotice 源文件丢失时的固定提示
	lostFileNotice = "文件损毁或丢失,请更换句柄"
	// handleInvalidNotice 句柄不存在 / 无法解析时的固定提示
	handleInvalidNotice = "句柄无效,文件不存在"
	// documentReadBytes 文档整篇取回的字节上限
	documentReadBytes = 12000
	// previewRunesLimit 结果预览的最大字符数
	previewRunesLimit = 512
	// listSummaryRunes 列表摘要的最大字符数
	listSummaryRunes = 512
)

var (
	// indexMu 保护索引缓存
	indexMu sync.Mutex
	// indexCache 搜索根绝对路径 → 目录索引
	indexCache = map[string]*dirIndex{}

	// hookMu 保护注入的能力钩子
	hookMu sync.RWMutex
	// hookSet 当前注入的能力钩子
	hookSet Hooks

	// memoryMu 保护记忆库初始化状态
	memoryMu sync.Mutex
	// memoryReady 记忆库是否已初始化
	memoryReady bool
)
