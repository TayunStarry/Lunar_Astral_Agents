package CreatorPlanner

// type.go 创作规划者的对外类型：工具结果、工具处理函数与一次推理循环的请求。

import (
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// Outcome 工具执行结果：JSON 回填 + 给最终总结用的短句 + 可归档的参考资料。
type Outcome struct {
	JSON      string
	Summary   string
	Reference string
}

// Handler 工具分发；done 为 true 时结束当前推理循环。
type Handler func(name string, args map[string]any) (outcome Outcome, done bool, err error)

// Request 一次创作推理循环的输入。
type Request struct {
	// Role 推理实况中显示的角色名。
	Role string
	// SystemPrompt 本轮系统提示。
	SystemPrompt string
	// Instruction 用户指令或任务描述。
	Instruction string
	// Tools 本轮可用工具。
	Tools []agentbase.ToolDefinition
	// Handler 工具执行器，由装配层注入。
	Handler Handler
	// OnReference 把工具产生的参考资料写回链侧记录。
	OnReference func(reference string)
}

// toolCallRecord 一次工具调用的有界记录。
type toolCallRecord struct {
	Name string
	Args string
	Note string
}
