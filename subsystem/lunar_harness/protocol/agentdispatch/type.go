package agentdispatch

// type.go 智能体聚合工具的统一输入 / 输出原语。
//
// 所有「智能体调度智能体」的工具都只使用这四个输入字段与三个输出字段：
// 分支差异全部由 type 选择，目标差异全部由 handle 指定，
// 不允许再夹带分支专属参数，也不允许输出规范以外的额外字段。

// Call 统一输入原语。
type Call struct {
	// Type 聚合工具的分支类型，必须与工具注册的类型标识完全匹配
	Type string
	// Instruction 自然语言描述的任务要求与完整上下文
	Instruction string
	// Handle 本次操作涉及的目标句柄列表；无目标时为空数组
	Handle []string
	// Details 为 true 时只返回职能范围描述，不执行任何实际操作
	Details bool
}

// Result 统一输出原语。
type Result struct {
	// Result 自然语言描述的最终结果，需覆盖任务要求的输出维度
	Result string `json:"result"`
	// Handle 本次操作实际涉及的句柄列表；无涉及句柄时为空数组
	Handle []string `json:"handle"`
	// Error 执行过程中捕获的错误信息；无错误时为空字符串
	Error string `json:"error"`
}
