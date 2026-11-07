package Savior

// type.go 拯救者智能体的公共类型定义。

import (
	"time"

	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// Failure 一条待诊断故障（按「智能体 + 故障类型」去重）
type Failure struct {
	ID int64
	// Key 去重键（agent|kind），队列与历史共用
	Key   string
	Agent string
	Kind  string
	Error string
	// Criterion 触发判据原文
	Criterion string
	// Entry 触发入口（步骤名 / 角色名）
	Entry string
	// Tool 失败或可疑的工具名
	Tool string
	// ToolArgs 该次工具调用的参数
	ToolArgs string
	// ToolResult 该次工具调用的返回
	ToolResult string
	// Retries 上报方已自行重试的次数
	Retries int
	// ContextMessages 出错时的上下文条数
	ContextMessages int
	// ContextRunes 出错时的上下文正文长度
	ContextRunes int
	// Metrics 其它数值水位
	Metrics map[string]int64

	At      time.Time
	FirstAt time.Time
	// Occurrences 同一 agent+kind 连续出现的次数
	Occurrences int
	// Attempts 本次故障已执行的诊断尝试次数
	Attempts int
	// Escalated 是否已判定为需人工跟进
	Escalated bool
	// Conclusions 历次诊断尝试的结论（有界）
	Conclusions []string
}

// Advice 一条维修建议：只有判断与步骤，不包含任何已执行动作。
type Advice struct {
	FailureID int64
	Agent     string
	Kind      string
	Error     string
	// Diagnosis 故障判断
	Diagnosis string
	// Causes 可能原因
	Causes string
	// Steps 建议的修复步骤（可执行，但不代执行）
	Steps []string
	// Risks 需要人工确认的风险点
	Risks string
	// Confidence 置信度（high / medium / low）
	Confidence string
	// Basis 判断依据（引用了哪些简报/状态字段）
	Basis string
	At    time.Time
}

// faultHistory 跨诊断周期的故障计数，用于识别同类故障反复出现。
type faultHistory struct {
	Count  int
	LastAt time.Time
}

// escalation 一条已标记为需人工跟进的故障记录。
type escalation struct {
	At         time.Time
	Failure    *Failure
	Conclusion string
}

// toolDefinition 工具声明（统一复用基座类型）
type toolDefinition = agentbase.ToolDefinition

// toolOutcome 工具执行结果
type toolOutcome struct {
	JSON    string
	Summary string
	Done    bool
}

// diagnoseSession 单次诊断会话的有界状态。
type diagnoseSession struct {
	// actions 已执行的只读动作数
	actions int
	// advice 已提交的维修建议
	advice *Advice
}
