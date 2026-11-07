package thinking

import (
	"encoding/json"
	"time"

	"LunarSubsystem/LunarHarness/agents/Dialogue"
	"LunarSubsystem/LunarHarness/chain/thinking/tickloop"
	"LunarSubsystem/LunarHarness/engine/mediaobject"
	"LunarSubsystem/LunarHarness/engine/resource"
)

// ==== 消息与内容 ====

// 消息内容类型码（与 OpenAI 兼容协议的内容项 type 一一对应）
const (
	ContentText       = mediaobject.EnvelopeText
	ContentImageURL   = mediaobject.EnvelopeImageURL
	ContentInputAudio = mediaobject.EnvelopeInputAudio
	ContentVideoURL   = mediaobject.EnvelopeVideoURL
	ContentAudioURL   = mediaobject.EnvelopeAudioURL
	// 入站多模态信封（发送侧声明类型，后端只校验）
	ContentImageStatic   = mediaobject.EnvelopeImageStatic
	ContentImageAnimated = mediaobject.EnvelopeImageAnimated
	ContentVideo         = mediaobject.EnvelopeVideo
	ContentAudio         = mediaobject.EnvelopeAudio
	ContentFile          = mediaobject.EnvelopeFile
	ContentWebpage       = mediaobject.EnvelopeWebpage
	// ContentAudioHandle 音频句柄内容项（接入阶段完成 ASR 后由服务端生成）
	ContentAudioHandle = "audio_handle"
	// ContentQuote 引用消息内容项（单条或多条消息引用）
	ContentQuote = "quote"
)

// 消息角色
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleSystem    = "system"
	RoleTool      = "tool"
)

// URLPayload 媒体载荷（图片 / 视频 / 音频共用），与写入侧共用同一份结构定义
type URLPayload = mediaobject.Object

// AudioPayload 内联音频载荷，与写入侧共用同一份结构定义
type AudioPayload = mediaobject.Object

// AudioHandlePayload 音频转写悬空句柄载荷
type AudioHandlePayload struct {
	// Handle 规范资源句柄（@A.nnn）
	Handle string `json:"handle"`
	// Text 转写文本（描述信息）
	Text string `json:"text,omitempty"`
	// Name 展示文件名
	Name string `json:"name,omitempty"`
}

// QuotedMessage 被引用消息的快照
type QuotedMessage struct {
	// ID 被引用消息的本地 ID（前端用于跳转定位）
	ID string `json:"id,omitempty"`
	// Role 被引用消息角色（user / assistant）
	Role string `json:"role,omitempty"`
	// Sender 发送者展示名
	Sender string `json:"sender,omitempty"`
	// Content 被引用消息正文
	Content string `json:"content,omitempty"`
	// Categories 被引用消息分类（用于语音/图片预览）
	Categories []string `json:"categories,omitempty"`
	// AudioHandle 若被引用消息包含音频，此处携带音频句柄
	AudioHandle string `json:"audio_handle,omitempty"`
	// Text 音频转写文本（句柄不可用时的兜底描述）
	Text string `json:"text,omitempty"`
}

// QuotePayload 引用消息载荷
type QuotePayload struct {
	// Messages 被引用的消息快照（支持多条同时引用）
	Messages []QuotedMessage `json:"messages"`
}

// ContentItem 多模态内容项
type ContentItem struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	// ResourceHandle 清洗阶段为图片记下的资源句柄（不上线，仅用于远期记忆追溯；
	// json:"-" 同时保证发送侧无法伪造，是「服务端清洗产物」的唯一可信标记）
	ResourceHandle string              `json:"-"`
	ImageURL       *URLPayload         `json:"image_url,omitempty"`
	AudioHandle    *AudioHandlePayload `json:"audio_handle,omitempty"`
	Quote          *QuotePayload       `json:"quote,omitempty"`

	// 入站信封专用载荷：每类信封只允许出现对应的一项
	ImageStatic   *mediaobject.ImageStaticPayload   `json:"image_static,omitempty"`
	ImageAnimated *mediaobject.ImageAnimatedPayload `json:"image_animated,omitempty"`
	Video         *mediaobject.VideoPayload         `json:"video,omitempty"`
	Audio         *mediaobject.AudioPayload         `json:"audio,omitempty"`
	File          *mediaobject.FilePayload          `json:"file,omitempty"`
	Webpage       *mediaobject.WebpagePayload       `json:"webpage,omitempty"`
}

// MarshalJSON 序列化内容项：文本项必须始终带 text 键。
// 严格解析的 OpenAI 兼容服务端（如 llama.cpp）会对缺失的 text 键直接报错，
// 空文本项若因 omitempty 省略该键，整次请求都会 500。
func (c ContentItem) MarshalJSON() ([]byte, error) {
	type contentItemJSON struct {
		Type          string                            `json:"type"`
		Text          *string                           `json:"text,omitempty"`
		ImageURL      *URLPayload                       `json:"image_url,omitempty"`
		AudioHandle   *AudioHandlePayload               `json:"audio_handle,omitempty"`
		Quote         *QuotePayload                     `json:"quote,omitempty"`
		ImageStatic   *mediaobject.ImageStaticPayload   `json:"image_static,omitempty"`
		ImageAnimated *mediaobject.ImageAnimatedPayload `json:"image_animated,omitempty"`
		Video         *mediaobject.VideoPayload         `json:"video,omitempty"`
		Audio         *mediaobject.AudioPayload         `json:"audio,omitempty"`
		File          *mediaobject.FilePayload          `json:"file,omitempty"`
		Webpage       *mediaobject.WebpagePayload       `json:"webpage,omitempty"`
	}
	out := contentItemJSON{
		Type:          c.Type,
		ImageURL:      c.ImageURL,
		AudioHandle:   c.AudioHandle,
		Quote:         c.Quote,
		ImageStatic:   c.ImageStatic,
		ImageAnimated: c.ImageAnimated,
		Video:         c.Video,
		Audio:         c.Audio,
		File:          c.File,
		Webpage:       c.Webpage,
	}
	if c.Type == ContentText {
		text := c.Text
		out.Text = &text
	}
	return json.Marshal(out)
}

// PostMessage 模型消息（content 为 string 或 []ContentItem）
type PostMessage struct {
	Role       string         `json:"role"`
	Content    any            `json:"content"`
	Source     string         `json:"source,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCallItem `json:"tool_calls,omitempty"`
}

// ==== 工具调用 ====

// ToolCall 发给模型的函数工具定义
type ToolCall struct {
	Type     string         `json:"type"`
	Function ToolDefinition `json:"function"`
}

// ToolDefinition 函数定义
type ToolDefinition struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters,omitempty"`
	Strict      bool   `json:"strict,omitempty"`
}

// ToolCallFunction 模型返回的调用函数
type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolCallItem 模型返回的工具调用项
type ToolCallItem struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function ToolCallFunction `json:"function"`
}

// ToolResult 工具执行结果：文本 + 可选图片 data URI（定义归智能体侧，链内沿用同一类型）
type ToolResult = Dialogue.ToolResult

// ToolHandler 工具处理函数
type ToolHandler func(args map[string]any) ToolResult

// ==== 模型响应 ====

// AssistantMessage 助手回复消息
type AssistantMessage struct {
	Role             string         `json:"role"`
	Content          string         `json:"content"`
	ReasoningContent string         `json:"reasoning_content,omitempty"`
	ToolCalls        []ToolCallItem `json:"tool_calls,omitempty"`
}

// ModelChoice 单条完成选项
type ModelChoice struct {
	Index        int              `json:"index"`
	Message      AssistantMessage `json:"message"`
	FinishReason string           `json:"finish_reason"`
}

// ModelResponseBody 模型响应主体
type ModelResponseBody struct {
	Choices []ModelChoice   `json:"choices"`
	Error   json.RawMessage `json:"error,omitempty"`
}

// ModelResponse 模型调用结果
type ModelResponse struct {
	Body ModelResponseBody
}

// ChatCache 单次应答周期的工具调用聚合缓存
type ChatCache struct {
	ToolCalls          []ToolCallItem
	DescriptionContent string
}

// ==== 对话缓存（database/messages.json） ====

// CachedAttachment 缓存附件
type CachedAttachment struct {
	Type  string `json:"type"`
	Src   string `json:"src,omitempty"`
	Label string `json:"label"`
	// Handle 音频转写悬空句柄（无实体音频，仅保留描述）
	Handle string `json:"handle,omitempty"`
	// Text 音频转写文本
	Text string `json:"text,omitempty"`
}

// CachedMessage 缓存消息（与前端 messages.js 结构一致）
type CachedMessage struct {
	ID            string             `json:"id"`
	Role          string             `json:"role"`
	Categories    []string           `json:"categories"`
	Content       string             `json:"content"`
	Sender        string             `json:"sender,omitempty"`
	Attachments   []CachedAttachment `json:"attachments,omitempty"`
	Quotes        []QuotedMessage    `json:"quotes,omitempty"`
	ImageSrc      string             `json:"imageSrc,omitempty"`
	ImageLabel    string             `json:"imageLabel,omitempty"`
	Audio         string             `json:"audio,omitempty"`
	MusicTitle    string             `json:"musicTitle,omitempty"`
	MusicSummary  string             `json:"musicSummary,omitempty"`
	MusicDuration float64            `json:"musicDuration,omitempty"`
	FileRef       string             `json:"fileRef,omitempty"`
	FileLabel     string             `json:"fileLabel,omitempty"`
	Timestamp     int64              `json:"timestamp"`
}

// ==== 记忆 ====

// MemorySearchHit 跨集合记忆检索命中
type MemorySearchHit struct {
	ID         string  `json:"id"`
	Type       string  `json:"type"`
	Role       string  `json:"role"`
	Content    string  `json:"content"`
	Similarity float64 `json:"similarity"`
	Timestamp  int64   `json:"timestamp,omitempty"`
	Collection string  `json:"collection,omitempty"`
	Source     string  `json:"source,omitempty"`
	URL        string  `json:"url,omitempty"`
	Base64     string  `json:"base64,omitempty"`
}

// MemorySearchOptions 记忆检索选项
type MemorySearchOptions struct {
	Mode        string
	Filters     []string
	Exclude     []string
	ForceGlobal bool
}

// ==== 统一资源句柄系统类型别名 ====

type ResourceEntry = resource.Resource

// ==== 句柄 ====

// ResourceHandle 解析后的资源句柄
type ResourceHandle struct {
	Kind      string
	Seq       int
	Section   int
	LineStart int
	LineEnd   int
	Anchor    string
}

// ==== HTTP 请求/响应 ====

// MessageBatchRequest 消息批量写入请求（统一时序队列唯一写入口）
type MessageBatchRequest struct {
	Messages []PostMessage `json:"messages"`
}

// BatchResponse 批量操作响应
type BatchResponse struct {
	Success bool `json:"success"`
	Length  int  `json:"length"`
	// Handles 本批音频落盘后生成的句柄（按消息内音频顺序；无音频时为空）
	Handles []string `json:"handles,omitempty"`
	// Invalid 本批被丢弃的无效信封数量
	Invalid int `json:"invalid,omitempty"`
}

// ProxyRequest 代理请求结构
type ProxyRequest struct {
	URL         string      `json:"url"`
	RequestInit RequestInit `json:"requestInit"`
}

// RequestInit 请求初始化参数
type RequestInit struct {
	Method      string            `json:"method,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Body        any               `json:"body,omitempty"`
	Redirect    string            `json:"redirect,omitempty"`
	Credentials string            `json:"credentials,omitempty"`
}

// ProxyResponse 代理响应结构
type ProxyResponse struct {
	Status     int               `json:"status"`
	StatusText string            `json:"statusText"`
	Headers    map[string]string `json:"headers"`
	Body       json.RawMessage   `json:"body"`
}

// ==== 推送数据 ====

// PushContextData 推送上下文数据
type PushContextData struct {
	Type    string `json:"type"`
	Content string `json:"content"`
	Audio   string `json:"audio,omitempty"`
	Auto    bool   `json:"auto,omitempty"`
	// Title 音乐消息标题（仅 type=music）
	Title string `json:"title,omitempty"`
	// Summary 音乐消息描述（仅 type=music）
	Summary string `json:"summary,omitempty"`
	// Duration 音频时长（秒，仅 type=music）
	Duration float64 `json:"duration,omitempty"`
}

// PushImageData 推送图片数据
type PushImageData struct {
	Type    string   `json:"type"`
	Sticker bool     `json:"sticker,omitempty"`
	Images  []string `json:"images"`
}

// ==== Tick 调度 ====

// TickSnapshot 月华侧 Tick 判定快照（引擎运行时通过泛型参数 S 使用）。
type TickSnapshot struct {
	Now            time.Time
	IdleTicks      int
	Unread         int
	SalvagePending int
}

// tickScheduler 月华调度器实例别名（泛型实参为月华快照）。
type tickScheduler = tickloop.Scheduler[TickSnapshot]

// RuntimeGlobalMessage 对话者运行时消息的全局共享数据。
// 最近生活记录与计划摘要不再主动注入对话上下文，需要时由 life_planning 按需读取。
type RuntimeGlobalMessage struct {
	Timestamp    int64    `json:"timestamp"`
	MoodLabel    string   `json:"mood_label"`
	EmotionMarks []string `json:"emotion_marks,omitempty"`
}

// TextChunk 正文切片
type TextChunk struct {
	Display string
	TTS     string
}

// ParsedContent 解析后的文本
type ParsedContent struct {
	ThinkingBlocks []string
	CodeBlocks     []string
	ActionBlocks   []string
	TextChunks     []TextChunk
	DisplayText    string
}

// ModelBuilder 模型构建器（角色共享的上下文与请求逻辑）
type ModelBuilder struct {
	RoleTag           string
	ReasoningStrength string
	EnableTools       bool
	DisableThinking   bool
	Messages          []PostMessage
	RagMessages       []PostMessage
	RuntimeMessages   []PostMessage
	SystemPrompt      string
	ContextLimit      int
}

// DialogueRole 对话者角色
type DialogueRole struct {
	*ModelBuilder
	LunarAppearance   string
	CrystalAppearance string
}

// ModelRunFunc 对话者单轮模型调用入口；测试可注入替身，生产默认走 ModelBuilder.run。
type ModelRunFunc func(builder *ModelBuilder, appendContext []PostMessage, tools []ToolCall) (*ModelResponse, error)

// MemorizerRole 记忆入库与检索流程（非独立智能体：不持有模型上下文，也不参与 function calling）
type MemorizerRole struct{}

// coreRoleSnapshot 对话者的上下文视图（拯救者读取与修复时使用）
type coreRoleSnapshot struct {
	ID      string
	Label   string
	Builder *ModelBuilder
}
