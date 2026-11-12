package viewer

// ==== 族共享类型集中定义 ====
// 「观影」族的五个成员（阅片 / 审查 / 评论 / 观感 / 分享）与观影链共用这里的
// 调用载体。成员各自的专属类型放在各自包的 type.go，不回流到这里。

import "LunarSubsystem/LunarHarness/protocol/agentbase"

// ChatRequest 一次观影者模型调用。
type ChatRequest struct {
	// Agent 故障归属 ID（智能修复按此去重）
	Agent string
	// Role 推理实况角色标签
	Role string
	// Model 模型名（空 = 主多模态模型）
	Model string
	// System 系统提示词
	System string
	// User 用户输入
	User string
	// Images 图文混排时附带的图片（data URL 或 http 地址）
	Images []string
	// Attempts 总尝试次数（含首次；<1 视为 1）
	Attempts int
}

// SubmitRequest 一次「必填工具提交」调用。
// 结果只从 Tool 的工具调用参数读取，正文一律不解析。
type SubmitRequest struct {
	// Agent 故障归属 ID（智能修复按此去重）
	Agent string
	// Role 推理实况角色标签
	Role string
	// Model 模型名（空 = 主多模态模型）
	Model string
	// System 系统提示词
	System string
	// User 用户输入
	User string
	// Images 图文混排时附带的图片（data URL 或 http 地址）
	Images []string
	// Tool 唯一的提交工具定义（链侧只解析它的参数）
	Tool agentbase.ToolDefinition
	// Attempts 总尝试次数（含首次；<1 视为 1）
	Attempts int
}

// chatMessage OpenAI 兼容消息。
type chatMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

// imageURL 图片地址。
type imageURL struct {
	URL string `json:"url"`
}

// contentPart 图文混排片段。
type contentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *imageURL `json:"image_url,omitempty"`
}
