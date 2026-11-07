package Dialogue

// variable.go 对话者的常量集中管理。

import "LunarSubsystem/LunarHarness/protocol/agentinfo"

const (
	// AgentID 故障归属 ID（与 agentrepair 注册的 ID 一致）
	AgentID = "dialogue"
	// Role 推理实况、工具目录与数据导出共用的显示名
	Role = agentinfo.NameDialogue
	// LoggerModule 日志分类（沿用思维链既有分类）
	LoggerModule = "Dialogue"
	// retryLoopAttempts 模型输出陷入重复时，单次调用的额外重试次数
	retryLoopAttempts = 1
)
