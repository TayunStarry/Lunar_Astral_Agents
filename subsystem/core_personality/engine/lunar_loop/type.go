package lunarloop

import (
	"strings"

	"github.com/dop251/goja"
)

// IPInfo 存储IP地址信息（对应 ip-api.com 响应结构）
type IPInfo struct {
	Status     string `json:"status"`
	Message    string `json:"message"`
	RegionName string `json:"regionName"`
	City       string `json:"city"`
}

// Runtime 存储JavaScript运行时实例，用于调用适配器函数
type Runtime struct {
	runtime *goja.Runtime
}

// TextContent 文本内容
type TextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ImageContent 图片内容
type ImageContent struct {
	Type     string `json:"type"`
	ImageURL struct {
		URL string `json:"url"`
	} `json:"image_url"`
}

// PostMessageRole 消息角色类型
type PostMessageRole string

const (
	RoleUser      PostMessageRole = "user"
	RoleAssistant PostMessageRole = "assistant"
	RoleSystem    PostMessageRole = "system"
	RoleTool      PostMessageRole = "tool"
)

// PostMessage 消息结构体
type PostMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`          // 可以是string、[]MessageContent或MediaUrlContent
	Source  string `json:"source,omitempty"` // 消息来源：qq=QQ适配器；空=前端/宿主
}

// MediaUrlPayload 媒体URL载荷（与 TS 侧多模态内容项的 URL 包装结构对齐）
type MediaUrlPayload struct {
	URL string `json:"url"`
}

// MediaUrlContent 统一消息队列中的媒体URL内容项
// 视频/音频URL不再走独立队列，而是包装为本结构按到达时序与文本消息混排入 UnreadContext，
// 由智能体理解后原位置换为文本（理解失败时置换为兜底提示文本）
type MediaUrlContent struct {
	Type     string           `json:"type"`                // 内容类型："video_url"（视频）或 "audio_url"（音频）
	VideoUrl *MediaUrlPayload `json:"video_url,omitempty"` // 视频URL载荷（Type 为 video_url 时存在）
	AudioUrl *MediaUrlPayload `json:"audio_url,omitempty"` // 音频URL载荷（Type 为 audio_url 时存在）
}

// LTPXPackageInfo LTPX 工具包配置结构
type LTPXPackageInfo struct {
	ID          string           `json:"id"`
	Title       string           `json:"title"`
	Description string           `json:"description"`
	Tags        []string         `json:"tags"`
	URL         string           `json:"url"`
	Tools       []map[string]any `json:"tools"`
}

// LTPXRemoteToolDef 琉璃工具链中的单个工具定义
// 琉璃对外统一暴露「智能体式」工具：接受字符串指令，返回文本结果（含操作结果与推荐后续操作）
type LTPXRemoteToolDef struct {
	Name        string `json:"name"`        // 工具名
	Description string `json:"description"` // 工具能力描述（供 LLM 决策）
	Parameters  any    `json:"parameters"`  // JSON Schema 参数定义（聚合工具的 {details, tool, instruction}）
}

// LTPXRemoteToolRef 聚合工具可调用的单个应用（名称 + 能力描述）。
// 月华把 Name 写成 use_the_program 的 tool 参数枚举，并按名查询 Description。
type LTPXRemoteToolRef struct {
	Name        string `json:"name"`        // 应用名（tool 参数枚举值）
	Description string `json:"description"` // 应用能力描述（details 查询结果）
}

// LTPXRemoteStatusResult 月华同步 LTPX 工具链的返回结果。
// 合并为单进程后不含在线状态：宿主未就绪时 Tool 为 nil（JS 侧保留上次定义）。
type LTPXRemoteStatusResult struct {
	Tool  *LTPXRemoteToolDef  `json:"tool,omitempty"`  // 聚合工具（v5 起恒为单一 use_the_program；未就绪时为 nil）
	Tools []LTPXRemoteToolRef `json:"tools,omitempty"` // 可调用应用清单（tool 枚举 + details 数据源）
}

// LTPXRemoteEventResult 月华事件推送结果（同步返回给 JS 端）
type LTPXRemoteEventResult struct {
	Modified bool `json:"modified"` // 是否有插件通过 modifiedData 改写了负载
	Data     any  `json:"data"`     // 插件处理后的负载；未改写时为 null（调用方用原始数据）
	Returned bool `json:"returned"` // 是否有插件通过 return 回传了本次事件的业务结果
	Return   any  `json:"return"`   // 插件经 return 回传的业务结果；无回传时为 null（调用方走本地默认处理）
}

// PushContextData 推送上下文数据
type PushContextData struct {
	Type    string `json:"type"`
	Content string `json:"content"`
	Audio   string `json:"audio,omitempty"`
	// Auto 标记这条消息是「高耗时任务的快速应答」：QQ 桥接会转发但不占用应答份额
	Auto bool `json:"auto,omitempty"`
}

// PushImageData 推送图片数据
type PushImageData struct {
	Type    string   `json:"type"`              // 图片类型
	Sticker bool     `json:"sticker,omitempty"` // 是否作为表情包发送
	Images  []string `json:"images"`            // 图片URL列表
}

// AgentPositionData 智能体3D位置数据
type AgentPositionData struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

// 引擎命令消息格式（与 engine.js 的 handleChannelMessage 对齐）
type EngineCommand struct {
	Type      string `json:"type"`
	Source    string `json:"source"`
	Payload   any    `json:"payload"`
	Timestamp int64  `json:"timestamp"`
}

// ==== syncFetch 流式桥协议类型（OpenAI 兼容流式协议） ====

// sseToolCall 流式 tool_calls 单个分片（按 index 聚合还原完整调用）
type sseToolCall struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// sseChunk SSE data 帧结构（仅取流式桥所需字段；reasoning/reasoning_content 兼容不同后端）
type sseChunk struct {
	Choices []struct {
		Delta struct {
			Role             string        `json:"role"`
			Content          string        `json:"content"`
			ReasoningContent string        `json:"reasoning_content"`
			Reasoning        string        `json:"reasoning"`
			ToolCalls        []sseToolCall `json:"tool_calls"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *tokenUsage `json:"usage"`
}

// tokenUsage 模型调用的 token 用量（流式 usage 帧原文；缺失时由流式桥按文本长度估算）
type tokenUsage struct {
	Prompt     int64 `json:"prompt_tokens"`
	Completion int64 `json:"completion_tokens"`
}

// toolAccum 单个流式工具调用的聚合缓冲
type toolAccum struct {
	id   string
	name string
	args strings.Builder
}

// toolCallAccumulator 按 index 聚合流式 tool_calls 分片（保持到达顺序）
type toolCallAccumulator struct {
	order []int
	items map[int]*toolAccum
}
