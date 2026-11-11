// Package file 文件族：文件读写相关的五个智能体、三个工具库与共用工具推理循环。
//
// 族根只承载成员共用的数据载体（Request / Outcome）与族级运行参数，
// 不 import 任何成员包；成员包 import 族根，依赖方向单向。
//
// 五个成员（文件操作 / 文件操纵 / 归档操纵 / 代码执行 / 内容回滚）的调用形态相同：
// 每个成员只声明自己的 AgentID、角色、提示词与工具，再由族根的 Run 完成模型调用、
// 工具回填、轮次上限、失败上报与实况登记。这里不承载任何文件业务判断。
package file

import (
	"time"

	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// Outcome 工具执行结果：JSON 回填 + 给最终总结用的短句 + 本次涉及的资源句柄 + 审核证据。
type Outcome struct {
	JSON      string
	Summary   string
	Reference string
	Handles   []string
	// Evidence 供审核智能体核对的有界变更证据（不包含整份文件）
	Evidence string
	// Mutated 本次工具调用是否成功写入
	Mutated bool
}

// Handler 工具分发；done 为 true 时结束当前推理循环。
type Handler func(name string, args map[string]any) (Outcome, bool, error)

// Request 一次子链路智能体推理循环的输入。
type Request struct {
	// AgentID 推理实况、数据导出与失败上报使用的智能体标识。
	AgentID string
	// Role 推理实况中显示的角色名。
	Role string
	// SystemPrompt 本轮系统提示。
	SystemPrompt string
	// Instruction 用户指令或任务描述。
	Instruction string
	// Tools 本轮可用工具。
	Tools []agentbase.ToolDefinition
	// Handler 工具执行器，由所属链路注入。
	Handler Handler
	// MaxRounds 工具循环轮次上限；<=0 时使用默认值。
	MaxRounds int
	// Timeout 单次模型调用超时；<=0 时使用默认值。
	Timeout time.Duration
	// MaxTokens 单次模型调用的输出上限；<=0 时使用默认值。
	// 用于阻止模型在任务完成后继续长篇生成，避免长时间卡住。
	MaxTokens int
	// SummaryMaxTokens 已发生修改后的下一轮输出上限；<=0 时使用默认值。
	// 修改完成后模型只需一句话总结，不应再复述或续写正文。
	SummaryMaxTokens int
}
