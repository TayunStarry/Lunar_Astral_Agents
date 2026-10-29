package Creator

// type.go 创作体系的类型定义。

import "time"

// ==== 模型调用类型 ====

// chatMessage OpenAI 兼容消息（支持 assistant.tool_calls 与 tool 回填）。
type chatMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content,omitempty"`
	ToolCalls  []toolCallItem `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

// toolCallItem 模型返回的工具调用。
type toolCallItem struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function toolCallFunction `json:"function"`
}

// toolCallFunction 工具名与 JSON 参数。
type toolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// toolDefinition 发给模型的函数工具定义。
type toolDefinition struct {
	Type     string       `json:"type"`
	Function toolFunction `json:"function"`
}

// toolFunction 函数描述与参数 Schema。
type toolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// modelResponse 一次模型调用的结果。
type modelResponse struct {
	Content   string
	Reasoning string
	ToolCalls []toolCallItem
}

// assistantWire 响应中的 assistant 消息。
type assistantWire struct {
	Role             string         `json:"role"`
	Content          string         `json:"content"`
	ReasoningContent string         `json:"reasoning_content"`
	ToolCalls        []toolCallItem `json:"tool_calls"`
}

// chatCompletion OpenAI 兼容响应（含代理包装前的原始结构）。
type chatCompletion struct {
	Choices []struct {
		Message assistantWire `json:"message"`
	} `json:"choices"`
	Usage *modelUsage `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// modelUsage token 用量。
type modelUsage struct {
	Prompt     int64 `json:"prompt_tokens"`
	Completion int64 `json:"completion_tokens"`
}

// ==== 工具执行类型 ====

// toolOutcome 工具执行结果：JSON 回填 + 给最终总结用的短句 + 可归档的参考资料。
type toolOutcome struct {
	JSON      string
	Summary   string
	Reference string
}

// toolHandlerFunc 工具分发；done 为 true 时结束当前推理循环。
type toolHandlerFunc func(name string, args map[string]any) (outcome toolOutcome, done bool, err error)

// ==== 规划与日志类型 ====

// planStep 一个按严格顺序执行的调度项。
type planStep struct {
	Agent string `json:"agent"` // writer / engineer / researcher / runner
	Task  string `json:"task"`  // 自然语言执行目标与操作要求
}

// planResult 调度者输出的本轮计划。
type planResult struct {
	Steps []planStep `json:"steps"`
	Memos []string   `json:"memos,omitempty"`
}

// journal 创作事件的规划日志：记录上一轮的全部规划、备忘、参考资料与错误。
type journal struct {
	Version     int        `json:"version"`
	LastPlans   []planStep `json:"last_plans"`
	Memos       []string   `json:"memos"`
	References  []string   `json:"references"`
	Errors      []string   `json:"errors"`
	LastSummary string     `json:"last_summary"`
	UpdatedAt   int64      `json:"updated_at"`
}

// stepResult 单个调度项的执行结果。
type stepResult struct {
	Agent   string `json:"agent"`
	Task    string `json:"task"`
	Result  string `json:"result,omitempty"`
	Skipped bool   `json:"skipped"`
	Error   string `json:"error,omitempty"`
}

// session 一次创作运行的临时状态：收集参考资料与错误，供日志归档。
type session struct {
	refs      []string
	errs      []string
	memos     []string
	plan      planResult
	planReady bool
}

// ==== 文件与代码索引类型 ====

// codeSymbol 代码索引条目。
type codeSymbol struct {
	Name      string `json:"name"`
	Signature string `json:"signature,omitempty"`
	Comment   string `json:"comment,omitempty"`
}

// codeIndex 一个代码文件的三类结构化索引。
type codeIndex struct {
	Variables []codeSymbol `json:"variables"`
	Functions []codeSymbol `json:"functions"`
	Classes   []codeSymbol `json:"classes"`
}

// fileEntry 目录列表条目。
type fileEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	Dir   bool   `json:"dir"`
	Size  int64  `json:"size"`
	MTime int64  `json:"mtime"`
}

// ==== 脚本运行类型 ====

// scriptRunResult 一次 LTP9 脚本运行的结果。
type scriptRunResult struct {
	Package    string   `json:"package"`
	Loaded     bool     `json:"loaded"`
	Running    bool     `json:"running"`
	Authorized []string `json:"authorized"`
	Exports    []string `json:"exports"`
	Result     any      `json:"result,omitempty"`
	Error      string   `json:"error,omitempty"`
}

// docInfo DTS 开发文档信息。
type docInfo struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

// nowUnix 当前时间。
func nowUnix() int64 { return time.Now().Unix() }
