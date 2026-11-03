package Savior

// type.go 拯救者智能体的公共类型定义。

import (
	"time"

	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// Failure 一条待修复故障
type Failure struct {
	ID       int64
	Agent    string
	Error    string
	At       time.Time
	Attempts int
}

// toolDefinition 工具声明（统一复用基座类型）
type toolDefinition = agentbase.ToolDefinition

// toolOutcome 工具执行结果
type toolOutcome struct {
	JSON    string
	Summary string
	Done    bool
}
