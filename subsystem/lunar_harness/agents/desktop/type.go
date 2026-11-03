package desktop

import (
	"time"
)

// RunState 桌面操纵当前/最近一次运行状态（供控制台「运行状态」屏展示）。
type RunState struct {
	Running     bool      // 是否正在运行
	Stopping    bool      // 是否已请求中断、正在等待任务退出
	Instruction string    // 本轮指令
	StartedAt   time.Time // 启动时间
	FinishedAt  time.Time // 结束时间（未结束时为零值）
	Result      string    // 成功时的结果文本
	Error       string    // 失败/中断原因（空串表示无）
}

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

// ltpToolResult 工具执行结果（成功标记 + 文本/错误）。
type ltpToolResult struct {
	Success bool   `json:"success"`         // 是否成功
	Text    string `json:"text,omitempty"`  // 文本结果（可选）
	Error   string `json:"error,omitempty"` // 错误信息（可选）
}

// HandoffRecord 角色间跨轮交接记录，是桌面操纵唯一跨轮信息介质。
type HandoffRecord struct {
	Round         int    `json:"round"`          // 轮次
	Foreground    string `json:"foreground"`     // 前景窗口标题
	VisualSummary string `json:"visual_summary"` // 可视摘要
	InputBoxText  string `json:"input_box_text"` // 输入框文本
	CursorPos     string `json:"cursor_pos"`     // 光标位置
	LastAction    string `json:"last_action"`    // 最后操作
	Progress      string `json:"progress"`       // 进度
	Evidence      string `json:"evidence"`       // 证据

	TargetApp         string          `json:"target_app,omitempty"`         // 本轮启动的目标应用关键字
	ForegroundProcess string          `json:"foreground_process,omitempty"` // 前台窗口进程名
	Attempts          []ActionAttempt `json:"attempts,omitempty"`           // 最近几轮的尝试及结果（供规划者避免重复）
}

// ActionAttempt 一次执行尝试的结构化记录。
type ActionAttempt struct {
	Round     int    `json:"round"`      // 轮次
	Decision  string `json:"decision"`   // keyboard / uia / mouse / complete
	Tool      string `json:"tool"`       // 实际调用的工具（空表示未执行任何工具）
	InputText string `json:"input_text"` // 文本输入类工具写入的内容（用于防止重复输入）
	Duplicate bool   `json:"duplicate"`  // 是否因窗口已包含相同文本而跳过重复输入
	Result    string `json:"result"`     // 工具返回摘要
	Success   bool   `json:"success"`    // 工具自身是否成功
	Changed   bool   `json:"changed"`    // 是否产生可见变化（画面或窗口状态）
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
	Title      string `json:"title"` // 窗口标题
	lowerTitle string // 小写标题缓存，供排序比较使用
	Class      string `json:"class"`   // 窗口类名
	PID        uint32 `json:"pid"`     // 进程ID
	Process    string `json:"process"` // 进程名
}
