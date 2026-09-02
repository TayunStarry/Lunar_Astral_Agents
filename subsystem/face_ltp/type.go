package FaceLTP

// ==== OpenAI v1 协议类型（与琉璃 /v1 代理兼容） ====

// chatMessage 对话消息：role + content（string 或 []contentPart 多模态）+ 可选工具调用/回填字段
type chatMessage struct {
	Role       string     `json:"role"`
	Content    any        `json:"content"`
	ToolCalls  []toolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

// contentPart 多模态消息的单个组成部分（文本或图片）
type contentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *imageURL `json:"image_url,omitempty"`
}

// imageURL 图片内容的 data URL（base64）
type imageURL struct {
	URL string `json:"url"`
}

// toolCall 模型返回的工具调用
type toolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function toolCallFunc `json:"function"`
}

// toolCallFunc 工具调用的函数名与参数（arguments 为 JSON 字符串）
type toolCallFunc struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// toolDef 工具定义（OpenAI function calling schema）
type toolDef struct {
	Type     string   `json:"type"`
	Function function `json:"function"`
}

// function 工具的函数签名（name + description + JSON Schema 参数）
type function struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// chatRequest 聊天补全请求体
type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Tools    []toolDef     `json:"tools,omitempty"`
	Stream   bool          `json:"stream"`
}

// chatResponse 聊天补全响应体
type chatResponse struct {
	Choices []struct {
		Message struct {
			Role      string     `json:"role"`
			Content   any        `json:"content"`
			ToolCalls []toolCall `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// ==== 进程内调用结果类型 ====

// Result Face-LTP 执行结果（与琉璃 LTPXRemoteCallResponse 对齐）
type Result struct {
	Success bool   `json:"success"`
	Text    string `json:"text"`
	Error   string `json:"error,omitempty"`
}

// ==== 智能体工具执行结果 ====

// toolResult 单个工具执行的结构化结果（回填给模型）
type toolResult struct {
	Success bool   `json:"success"`
	Text    string `json:"text,omitempty"`
	Error   string `json:"error,omitempty"`
}

// ==== 窗口信息 ====

// windowInfo 顶层窗口描述（供 list_windows 返回给模型）
type windowInfo struct {
	Title   string `json:"title"`   // 窗口标题
	Class   string `json:"class"`   // 窗口类名
	PID     uint32 `json:"pid"`     // 所属进程 ID
	Process string `json:"process"` // 所属进程可执行文件名（如 QQ.exe）
}

// agentRound 已完成的对话轮（1 轮 = 月华指令文本 + 智能体答复文本）
type agentRound struct {
	User      string `json:"user"`
	Assistant string `json:"assistant"`
}

// agentPlanItem 执行计划中的单个计划项（对应桌面上的一个具体操作）。
// 由 set_plan 建立，经 confirm_step 标记为完成/重试；运行中状态由程序维护。
type agentPlanItem struct {
	No     int    `json:"no"`     // 步骤序号（从 1 起）
	Action string `json:"action"` // 该步骤操作描述（简短）
	Detail string `json:"detail"` // 具体落点/定位说明
	Status string `json:"status"` // pending / running / done / retry
	Tries  int    `json:"tries"`  // 重试次数
}
