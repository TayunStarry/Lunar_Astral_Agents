package ArchiveOps

// variable.go 归档写操作工具库的常量与操作名（业务逻辑不得写在本文件）

import "LunarSubsystem/LunarHarness/protocol/agentinfo"

const (
	// Module 日志模块名
	Module = agentinfo.NameArchiveOps
	// maxArchiveEntries 单个压缩包允许处理的最大条目数
	maxArchiveEntries = 20000
	// maxArchiveBytes 单个压缩包重建时的解压总量上限
	maxArchiveBytes = 512 << 20
)

// 操作名（也是对外工具名）
const (
	// OpCreate 创建压缩包
	OpCreate = "archive_create"
	// OpDelete 删除压缩包
	OpDelete = "archive_delete"
	// OpAdd 压缩包内新增 / 覆盖文件
	OpAdd = "archive_add"
	// OpRemove 压缩包内移除文件
	OpRemove = "archive_remove"
)
