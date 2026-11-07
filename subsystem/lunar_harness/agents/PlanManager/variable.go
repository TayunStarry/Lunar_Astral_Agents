package PlanManager

// variable.go 计划管理智能体的常量与工具名（业务逻辑不得写在本文件）。

import (
	"time"

	"LunarSubsystem/LunarHarness/protocol/agentinfo"
)

const (
	// Module 日志模块名
	Module = agentinfo.NamePlanManager
	// roleLabel TUI 实况屏显示的角色标签
	roleLabel = agentinfo.NamePlanManager
	// maxRounds 单条指令的工具循环上限
	maxRounds = 5
	// maxToolResultRunes 单条工具结果回填上限
	maxToolResultRunes = 3000
	// modelTimeout 单次模型调用超时
	modelTimeout = 90 * time.Second
)

// 工具名
const (
	// ToolQuery 查询计划表
	ToolQuery = "plan_query"
	// ToolCreate 新增计划项
	ToolCreate = "plan_create"
	// ToolUpdate 改写计划项
	ToolUpdate = "plan_update"
	// ToolDelete 删除计划项
	ToolDelete = "plan_delete"
)
