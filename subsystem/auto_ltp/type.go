package AutoLTP

// chatMessage OpenAI v1 协议中的单条对话消息。
type chatMessage struct {
	Role       string        `json:"role"`                   // 角色（user/assistant/system）
	Content    any           `json:"content"`                // 内容（文本或图片）
	ToolCalls  []ltpToolCall `json:"tool_calls,omitempty"`   // 工具调用请求（可选）
	ToolCallID string        `json:"tool_call_id,omitempty"` // 工具调用 ID（可选）
	Name       string        `json:"name,omitempty"`         // 名称（可选）
}

// contentPart 多媒体内容片段（文本或图片），用于携带文本与内嵌截图。
type contentPart struct {
	Type     string    `json:"type"`                // 内容类型（text/image）
	Text     string    `json:"text,omitempty"`      // 文本内容（可选）
	ImageURL *imageURL `json:"image_url,omitempty"` // 图片地址（data URI）（可选）
}

// imageURL 内嵌图片的地址（data URI）。
type imageURL struct {
	URL string `json:"url"` // 图片地址（data URI）
}

// ltpToolCall 模型返回的工具调用请求。
type ltpToolCall struct {
	ID       string      `json:"id"`       // 工具调用 ID
	Type     string      `json:"type"`     // 工具调用类型（function）
	Function ltpToolFunc `json:"function"` // 函数调用（包含函数名与参数）
}

// ltpToolFunc 工具调用的函数名与 JSON 参数串。
type ltpToolFunc struct {
	Name      string `json:"name"`      // 函数名
	Arguments string `json:"arguments"` // JSON 参数串
}

// ltpToolDef 对外暴露的工具定义（function calling schema）。
type ltpToolDef struct {
	Type     string     `json:"type"`     // 工具类型（function）
	Function ltpFuncDef `json:"function"` // 函数描述体（名称/描述/参数结构）
}

// ltpFuncDef 工具的 function 描述体（名称/描述/参数结构）。
type ltpFuncDef struct {
	Name        string         `json:"name"`        // 函数名
	Description string         `json:"description"` // 函数描述
	Parameters  map[string]any `json:"parameters"`  // 参数结构（键值对）
}

// ltpChatRequest 发往模型 /chat/completions 的请求体。
type ltpChatRequest struct {
	Model    string        `json:"model"`           // 模型名称
	Messages []chatMessage `json:"messages"`        // 对话消息序列
	Tools    []ltpToolDef  `json:"tools,omitempty"` // 工具定义序列（可选）
	Stream   bool          `json:"stream"`          // 是否流式输出
}

// ltpChatResponse 模型 /chat/completions 的响应体（含选择结果或错误）。
type ltpChatResponse struct {
	Choices []ltpChatRespChoice `json:"choices"` // 选择项序列
	Error   *ltpChatRespError   `json:"error"`   // 错误信息（可选）
}

// ltpChatRespChoice 响应体中的单个选择项。
type ltpChatRespChoice struct {
	Message ltpChatRespMessage `json:"message"` // 消息（包含角色/内容/工具调用）
}

// ltpChatRespMessage 响应体中选择项携带的消息。
type ltpChatRespMessage struct {
	Role      string        `json:"role"`       // 角色（user/assistant/system）
	Content   any           `json:"content"`    // 内容（文本或图片）
	ToolCalls []ltpToolCall `json:"tool_calls"` // 工具调用请求（可选）
}

// ltpChatRespError 响应体中的错误信息。
type ltpChatRespError struct {
	Message string `json:"message"` // 错误信息
}

// ltpToolResult 工具执行结果（成功标记 + 文本/错误）。
type ltpToolResult struct {
	Success bool   `json:"success"`         // 是否成功
	Text    string `json:"text,omitempty"`  // 文本结果（可选）
	Error   string `json:"error,omitempty"` // 错误信息（可选）
}

// HandoffRecord 角色间跨轮交接记录，是 Auto-LTP 唯一跨轮信息介质。
type HandoffRecord struct {
	Round         int    `json:"round"`          // 轮次
	Foreground    string `json:"foreground"`     // 前景窗口标题
	VisualSummary string `json:"visual_summary"` // 可视摘要
	InputBoxText  string `json:"input_box_text"` // 输入框文本
	CursorPos     string `json:"cursor_pos"`     // 光标位置
	LastAction    string `json:"last_action"`    // 最后操作
	Progress      string `json:"progress"`       // 进度
	Evidence      string `json:"evidence"`       // 证据
}

// editorTask 编纂者输出。
type editorTask struct {
	Task          string   `json:"task"`           // 任务描述
	Goal          string   `json:"goal"`           // 任务目标
	Acceptance    []string `json:"acceptance"`     // 接受标准
	StepsExpected []string `json:"steps_expected"` // 预期步骤
	Assumptions   []string `json:"assumptions"`    // 假设条件
}

// planDecision 规划者决策输出。
type planDecision struct {
	Decision string `json:"decision"` // 决策（执行/跳过）
	Action   string `json:"action"`   // 执行操作（可选）
	Via      string `json:"via"`      // 执行方式（可选）
}

// dtWindow 顶层窗口的概要信息。
type dtWindow struct {
	Title      string `json:"title"`    // 窗口标题
	lowerTitle string                // 小写标题缓存，供排序比较使用
	Class   string `json:"class"`   // 窗口类名
	PID     uint32 `json:"pid"`     // 进程ID
	Process string `json:"process"` // 进程名
}
