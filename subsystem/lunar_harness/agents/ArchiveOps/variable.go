package ArchiveOps

// variable.go 归档操作智能体的常量与操作名（业务逻辑不得写在本文件）

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

// ==== 自然语言调度 ====

const (
	// ScheduleOp 调度入口名（文件编辑 task_scheduling 回执使用）
	ScheduleOp = "archive_ops"
	// roleLabel TUI 实况屏显示的角色标签
	roleLabel = Module
	// maxScheduleRounds 单条调度指令的工具轮次上限
	maxScheduleRounds = 6
	// maxToolResultRunes 单条工具结果回填模型时的字符上限
	maxToolResultRunes = 6000
)
