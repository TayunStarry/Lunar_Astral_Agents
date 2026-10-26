package LunarMatrix

// type.go Lunar Matrix 智能体的全部类型定义。

// chatMessage OpenAI 兼容消息（支持 assistant.tool_calls 与 tool 回填）
type chatMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content,omitempty"`
	ToolCalls  []toolCallItem `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

// toolCallItem 模型返回的工具调用
type toolCallItem struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function toolCallFunction `json:"function"`
}

// toolCallFunction 工具名与 JSON 参数
type toolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// toolDefinition 发给模型的函数工具定义
type toolDefinition struct {
	Type     string       `json:"type"`
	Function toolFunction `json:"function"`
}

// toolFunction 函数描述与参数 Schema
type toolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// modelResponse 一次模型调用的结果
type modelResponse struct {
	Content   string
	Reasoning string
	ToolCalls []toolCallItem
}

// assistantWire 响应中的 assistant 消息
type assistantWire struct {
	Role             string         `json:"role"`
	Content          string         `json:"content"`
	ReasoningContent string         `json:"reasoning_content"`
	ToolCalls        []toolCallItem `json:"tool_calls"`
}

// chatCompletion OpenAI 兼容响应（含代理包装前的原始结构）
type chatCompletion struct {
	Choices []struct {
		Message assistantWire `json:"message"`
	} `json:"choices"`
	Usage *modelUsage `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// modelUsage token 用量
type modelUsage struct {
	Prompt              int64               `json:"prompt_tokens"`
	Completion          int64               `json:"completion_tokens"`
	PromptTokensDetails *promptTokenDetails `json:"prompt_tokens_details,omitempty"`
}

// promptTokenDetails 输入 token 的细分（提示缓存命中数）
type promptTokenDetails struct {
	CachedTokens int64 `json:"cached_tokens"`
}

// toolOutcome 工具执行结果：JSON 回填 + 给最终总结用的短句
type toolOutcome struct {
	JSON    string
	Summary string
}
