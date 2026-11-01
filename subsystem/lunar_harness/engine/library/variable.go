package library

// variable.go 资源库的常量与全局状态。

import (
	"sync"

	"LunarSubsystem/LunarHarness/engine/resource"
)

const (
	// Module 日志模块名
	Module = "资源库"
	// maxEditHistory 每个文件保留的最近操作条数
	maxEditHistory = 10
	// maxSnippetRunes 单条新增 / 删除片段的最大字符数
	maxSnippetRunes = 6000
	// maxTreeEntries 单次递归清单的条目上限（防止超大库把响应撑爆）
	maxTreeEntries = 20000
)

// 文件视觉状态：前端据此着色。
const (
	// StatusNormal 无操作记录：系统默认配色
	StatusNormal = "normal"
	// StatusActive AI 最近查看 / 操作过：黄色
	StatusActive = "active"
	// StatusOK 运行完成无异常：绿色
	StatusOK = "ok"
	// StatusError 运行抛出异常：红色
	StatusError = "error"
)

// Roots 资源库根目录（句柄系统的 4 个存储根）。
var Roots = resource.StorageRoots

var (
	// stateMu 保护状态表与版本号
	stateMu sync.RWMutex
	// states 路径 → 文件状态
	states = map[string]*FileState{}
	// version 状态版本号（前端增量刷新依据）
	version uint64

	// broadcastMu 保护广播句柄
	broadcastMu sync.RWMutex
	broadcast   func(msgType string, data any)
)
