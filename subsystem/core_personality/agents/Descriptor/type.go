package Descriptor

// type.go 描述者（句柄系统）的类型定义
//
// 描述者是句柄系统的一部分，不属于月华智能体体系：
//   - 影像描述：按视频/动态图句柄现场观看采样帧，产出可入库的内容描述；
//   - 文件探查：按文本句柄接收自然语言需求，用查询工具分片探查文件内容。
//
// 本文件只放类型定义，全局变量与常量见 variable.go。

import "CorePersonality/engine/persistence"

// chatMessage OpenAI v1 协议中的单条对话消息
type chatMessage struct {
	// Role 角色（system/user/assistant/tool）
	Role string `json:"role"`
	// Content 内容（文本或图文片段数组）
	Content any `json:"content"`
	// ToolCalls 模型请求的工具调用（assistant 消息）
	ToolCalls []toolCall `json:"tool_calls,omitempty"`
	// ToolCallID 工具结果对应的调用 ID（tool 消息）
	ToolCallID string `json:"tool_call_id,omitempty"`
}

// contentPart 图文混排内容片段
type contentPart struct {
	// Type 内容类型（text/image_url）
	Type string `json:"type"`
	// Text 文本内容（type=text）
	Text string `json:"text,omitempty"`
	// ImageURL 图片地址（type=image_url）
	ImageURL *imageURL `json:"image_url,omitempty"`
}

// imageURL 图片地址（data URI）
type imageURL struct {
	// URL 图片地址
	URL string `json:"url"`
}

// toolCall 模型返回的工具调用
type toolCall struct {
	// ID 调用 ID
	ID string `json:"id"`
	// Type 调用类型（function）
	Type string `json:"type"`
	// Function 函数名与 JSON 参数串
	Function toolFunction `json:"function"`
}

// toolFunction 工具调用的函数体
type toolFunction struct {
	// Name 函数名
	Name string `json:"name"`
	// Arguments JSON 参数串
	Arguments string `json:"arguments"`
}

// toolDef 暴露给模型的工具定义
type toolDef struct {
	// Type 工具类型（function）
	Type string `json:"type"`
	// Function 函数描述
	Function toolFunctionDef `json:"function"`
}

// toolFunctionDef 工具的函数描述体
type toolFunctionDef struct {
	// Name 函数名
	Name string `json:"name"`
	// Description 函数说明
	Description string `json:"description"`
	// Parameters JSON Schema 参数结构
	Parameters map[string]any `json:"parameters"`
}

// chatResponse 非流式响应体
type chatResponse struct {
	// Choices 选择项
	Choices []struct {
		// Message 助手消息
		Message chatMessage `json:"message"`
	} `json:"choices"`
	// Error 错误信息
	Error *chatError `json:"error"`
}

// chatError 模型错误信息
type chatError struct {
	// Message 错误文本
	Message string `json:"message"`
}

// streamChunk 流式响应分片
type streamChunk struct {
	// Choices 选择项
	Choices []struct {
		// Delta 增量
		Delta struct {
			// Content 正文增量
			Content string `json:"content"`
			// ReasoningContent 思考增量
			ReasoningContent string `json:"reasoning_content"`
			// ToolCalls 工具调用增量
			ToolCalls []struct {
				// Index 调用序号
				Index int `json:"index"`
				// ID 调用 ID（首个分片携带）
				ID string `json:"id"`
				// Type 调用类型
				Type string `json:"type"`
				// Function 函数增量
				Function struct {
					// Name 函数名
					Name string `json:"name"`
					// Arguments 参数串增量
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
	} `json:"choices"`
	// Error 错误信息
	Error *chatError `json:"error"`
}

// outlineItem 文件结构大纲的一项（行区间 + 内容标签）
type outlineItem struct {
	// Start 起始行（1 基）
	Start int
	// End 结束行（含）
	End int
	// Level 标题层级（0 表示无层级）
	Level int
	// Label 内容标签（标题 / 声明）
	Label string
}

// fileReference 文件引用关系的一项
type fileReference struct {
	// Line 引用所在行（1 基）
	Line int
	// Raw 引用原文
	Raw string
	// Target 引用目标原文
	Target string
	// Resolved 相对路径解析结果
	Resolved string
	// Exists 目标文件是否存在于本机
	Exists bool
}

// docContext 一次文件探查的上下文
type docContext struct {
	// Resource 资源目录条目
	Resource persistence.ArchiveResource
	// Handle 规范句柄
	Handle string
	// AbsPath 正文绝对路径
	AbsPath string
	// Text 正文文本
	Text string
	// Lines 按行切分的正文
	Lines []string
	// Truncated 正文是否因过大被截断
	Truncated bool
	// Outline 结构大纲
	Outline []outlineItem
	// References 引用关系
	References []fileReference
}
