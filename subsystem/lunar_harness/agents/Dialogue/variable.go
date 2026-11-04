package Dialogue

// variable.go 对话者的常量集中管理。

const (
	// AgentID 故障归属 ID（与 agentrepair 注册的 ID 一致）
	AgentID = "dialogue"
	// LoggerModule 日志分类（沿用思维链既有分类）
	LoggerModule = "Dialogue"
	// retryLoopAttempts 模型输出陷入重复时，单次调用的额外重试次数
	retryLoopAttempts = 1
)
