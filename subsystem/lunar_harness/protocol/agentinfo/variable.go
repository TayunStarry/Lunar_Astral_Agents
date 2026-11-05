package agentinfo

// variable.go 身份类型取值：智能体 / 链 / 执行体。
//
// 工具列表屏只把 KindAgent 计入「智能体数量」；链与执行体按各自类型标注，
// 避免把编排层或底层执行单元当成智能体统计。

const (
	// KindAgent 真实智能体（有独立系统提示词 / 上下文，可作为调用主体）
	KindAgent = "agent"
	// KindChain 多智能体编排链（本身不直接承接模型调用）
	KindChain = "chain"
	// KindExecutor 无模型调用的执行体（如归档解析）
	KindExecutor = "executor"
)
