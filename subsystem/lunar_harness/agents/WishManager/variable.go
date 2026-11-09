package WishManager

// variable.go 愿望管理智能体的常量与工具名（业务逻辑不得写在本文件）。

import (
	"time"

	"LunarSubsystem/LunarHarness/protocol/agentinfo"
)

const (
	// AgentID 智能体稳定标识
	AgentID = "wish_manager"
	// Module 日志模块名
	Module = agentinfo.NameWishManager
	// roleLabel TUI 实况屏显示的角色标签
	roleLabel = agentinfo.NameWishManager
	// maxRounds 单条指令的工具循环上限
	maxRounds = 6
	// maxToolResultRunes 单条工具结果回填上限
	maxToolResultRunes = 3000
	// modelTimeout 单次模型调用超时
	modelTimeout = 90 * time.Second
)

// 工具名
const (
	// ToolQuery 查询愿望列表
	ToolQuery = "wish_query"
	// ToolAdd 新增愿望
	ToolAdd = "wish_add"
	// ToolUpdate 改写愿望
	ToolUpdate = "wish_update"
	// ToolComplete 标记愿望完成
	ToolComplete = "wish_complete"
	// ToolDelete 删除愿望
	ToolDelete = "wish_delete"
	// ToolPriority 调整愿望优先级
	ToolPriority = "wish_priority"
)
