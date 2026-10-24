package lunar

import (
	"encoding/json"
	"strings"
	"time"

	persistence "LunarSubsystem/LunarHarness/engine/persistence"
)

// ==== 消息与内容 ====

// 消息内容类型码（与 OpenAI 兼容协议的内容项 type 一一对应）
const (
	ContentText       = "text"
	ContentImageURL   = "image_url"
	ContentInputAudio = "input_audio"
	ContentVideoURL   = "video_url"
	ContentAudioURL   = "audio_url"
)

// 消息角色
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleSystem    = "system"
	RoleTool      = "tool"
)

// URLPayload 单个 URL 载荷（图片 / 视频 / 音频共用）
type URLPayload struct {
	URL string `json:"url"`
}

// AudioPayload 内联音频载荷
type AudioPayload struct {
	Data   string `json:"data"`
	Format string `json:"format"`
}

// ContentItem 多模态内容项
type ContentItem struct {
	Type       string        `json:"type"`
	Text       string        `json:"text,omitempty"`
	ImageURL   *URLPayload   `json:"image_url,omitempty"`
	InputAudio *AudioPayload `json:"input_audio,omitempty"`
	VideoURL   *URLPayload   `json:"video_url,omitempty"`
	AudioURL   *URLPayload   `json:"audio_url,omitempty"`
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

// ToolResult 工具执行结果：文本 + 可选图片 data URI
type ToolResult struct {
	Text  string
	Image string
}

// ToolHandler 工具处理函数
type ToolHandler func(args map[string]any) ToolResult

// ==== 模型响应 ====

// ModelTimings 各阶段耗时（TUI 日志用）
type ModelTimings struct {
	PredictedPerSecond float64 `json:"predicted_per_second"`
	CacheN             int64   `json:"cache_n"`
}

// TokenUsage token 用量
type TokenUsage struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
}

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
	Timings *ModelTimings   `json:"timings,omitempty"`
	Usage   *TokenUsage     `json:"usage,omitempty"`
	Error   json.RawMessage `json:"error,omitempty"`
}

// ModelResponse 模型调用结果
type ModelResponse struct {
	Status int
	Body   ModelResponseBody
}

// ChatCache 单次应答周期的流式聚合缓存
type ChatCache struct {
	ToolCalls            []ToolCallItem
	CurrentToolCall      *ToolCallItem
	CurrentToolCallIndex int
	CurrentFunctionArgs  string
	CurrentFunctionName  string
	ThinkingContent      string
	DescriptionContent   string
}

// ==== 对话缓存（database/messages.json） ====

// CachedAttachment 缓存附件
type CachedAttachment struct {
	Type  string `json:"type"`
	Src   string `json:"src"`
	Label string `json:"label"`
}

// CachedMessage 缓存消息（与前端 messages.js 结构一致）
type CachedMessage struct {
	ID          string             `json:"id"`
	Role        string             `json:"role"`
	Categories  []string           `json:"categories"`
	Content     string             `json:"content"`
	Sender      string             `json:"sender,omitempty"`
	Attachments []CachedAttachment `json:"attachments,omitempty"`
	ImageSrc    string             `json:"imageSrc,omitempty"`
	ImageLabel  string             `json:"imageLabel,omitempty"`
	Audio       string             `json:"audio,omitempty"`
	ABCNotation string             `json:"abcNotation,omitempty"`
	ActionType  string             `json:"actionType,omitempty"`
	FileRef     string             `json:"fileRef,omitempty"`
	FileLabel   string             `json:"fileLabel,omitempty"`
	Timestamp   int64              `json:"timestamp"`
}

// ==== 计划表 ====

// ScheduleItem 计划表项
type ScheduleItem struct {
	ID            string `json:"id"`
	Time          string `json:"time"`
	Content       string `json:"content"`
	Type          string `json:"type,omitempty"`
	CompletedDate string `json:"completedDate,omitempty"`
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

// ==== 统一资源归档层类型别名 ====

type ResourceEntry = persistence.ArchiveResource
type ResourceReadResult = persistence.ReadResult
type ResourceSection = persistence.ArchiveSection
type ResourceSearchHit = persistence.ArchiveSearchHit
type ResourceStats = persistence.ArchiveStats

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

// ==== 绘制者 ====

// DimensionPlan 输出尺寸计划
type DimensionPlan struct {
	Width  int
	Height int
	Origin string
}

// GenerateImageParams 图像生成参数
type GenerateImageParams struct {
	Prompt          string
	NegativePrompt  string
	BatchSize       int
	Width           int
	Height          int
	Steps           int
	Strength        float64
	CfgScale        float64
	Seed            int64
	InitImg         string
	SuperResolution bool
}

// PaintingDetail 绘画作品详情
type PaintingDetail struct {
	ToolName      string
	PromptSummary string
	Character     string
	Chibi         bool
	Expression    string
	Posture       string
	Environment   string
	Roles         []string
	Planning      string
}

// CustomCharacterProfile 自定义角色档案
type CustomCharacterProfile struct {
	Name        string `json:"name"`
	Gender      string `json:"gender"`
	Appearance  string `json:"appearance"`
	OutfitFull  string `json:"outfitFull,omitempty"`
	OutfitChibi string `json:"outfitChibi,omitempty"`
}

// CharacterProfile 角色档案（内置 + 自定义统一视图）
type CharacterProfile struct {
	CustomCharacterProfile
	Builtin bool
}

// AppearanceParts 外观模板占位符填充参数
type AppearanceParts struct {
	Expression  string
	Posture     string
	Outfit      string
	Environment string
}

// PortraitPreset 立绘预设
type PortraitPreset struct {
	Kind         string
	Chibi        bool
	Label        string
	Template     string
	Base         string
	Outfit       string
	Gender       string
	DefaultRatio string
}

// ReferenceState 任务级参考图状态
type ReferenceState struct {
	Image  string
	Handle string
	Vision string
	Width  int
	Height int
}

// ImageReference 对话者传给绘制者的参考图
type ImageReference struct {
	Handle string
	Source string
	Desc   string
}

// PromptDocument 按标题路径读取的提示词文档
type PromptDocument struct {
	Sections map[string]string
	File     string
}

// PainterPromptLibrary 绘制者提示词库
type PainterPromptLibrary struct {
	Common     *PromptDocument
	Characters *PromptDocument
	Templates  map[string]string
	Builtin    map[string]CharacterProfile
}

// ==== 演奏者 ====

// MusicDraft 分阶段音乐创作草稿
type MusicDraft struct {
	Title                   string
	Structure               string
	Key                     string
	Meter                   string
	Tempo                   int
	Melody                  string
	MelodyInstrument        string
	MelodyBars              int
	Accompaniment           string
	AccompanimentInstrument string
	Chords                  string
	ChordInstrument         string
}

// MusicPieceDetail 音乐作品详情
type MusicPieceDetail struct {
	Title       string
	Instruments string
	Tempo       int
	Structure   string
	Key         string
	Meter       string
	ABCLength   int
}

// ==== 截图者 ====

// ScreenshotShot 一张已完成的截图
type ScreenshotShot struct {
	Note        string
	Mode        string
	WindowTitle string
	Width       int
	Height      int
	Base64      string
	URL         string
}

// ==== 引擎命令 ====

// EngineCommand 引擎命令消息格式（与 engine.js 的 handleChannelMessage 对齐）
type EngineCommand struct {
	Type      string `json:"type"`
	Source    string `json:"source"`
	Payload   any    `json:"payload"`
	Timestamp int64  `json:"timestamp"`
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
}

// PushImageData 推送图片数据
type PushImageData struct {
	Type    string   `json:"type"`
	Sticker bool     `json:"sticker,omitempty"`
	Images  []string `json:"images"`
}

// AgentPositionData 智能体 3D 位置
type AgentPositionData struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

// ==== LTPX 工具链 ====

// LTPXRemoteToolDef 聚合工具定义
type LTPXRemoteToolDef struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
}

// LTPXRemoteToolRef 可调用应用（名称 + 能力描述）
type LTPXRemoteToolRef struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// LTPXRemoteStatusResult 工具链同步结果
type LTPXRemoteStatusResult struct {
	Tool  *LTPXRemoteToolDef  `json:"tool,omitempty"`
	Tools []LTPXRemoteToolRef `json:"tools,omitempty"`
}

// ==== 数据导出 ====

// DumpRoleEntry 可导出角色
type DumpRoleEntry struct {
	Label   string
	Builder *ModelBuilder
}

// ==== 引擎自愈状态 ====

// EngineStatus 引擎自愈状态（TUI 展示用）
type EngineStatus struct {
	RuntimeReady        bool
	AgentReady          bool
	BootedAt            time.Time
	ReloadCount         int
	LastReloadAt        time.Time
	LastReloadReason    string
	LastReloadResult    string
	ConsecutiveFailures int
	LastFailure         string
	LastFailureAt       time.Time
	AutoReload          bool
	AutoReloadThreshold int
}

// ActionDefinition 渲染引擎上报的可用动作
type ActionDefinition struct {
	Name          string `json:"name"`
	MouseTracking bool   `json:"mouseTracking"`
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

// ==== 流式协议 ====

// sseToolCall 流式 tool_calls 单个分片
type sseToolCall struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// sseChunk SSE data 帧
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
	Usage *TokenUsage `json:"usage"`
}

// toolAccum 单个流式工具调用聚合缓冲
type toolAccum struct {
	ID   string
	Name string
	Args strings.Builder
}

// toolAccumulator 按 index 聚合流式 tool_calls
type toolAccumulator struct {
	Order []int
	Items map[int]*toolAccum
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
	LunarAppearance    string
	CrystalAppearance  string
	ToolRounds         int
	ActionClaimGuarded bool
	ImageWindow        int
	// PendingSpeeches 「边说边调工具」时随工具调用一起输出的正文，按时间顺序暂存；
	// 拿到工具回调后的正式回答时统一拼到前面，避免中途文本被下一轮响应覆盖而丢失。
	PendingSpeeches []string
}

// MemorizerRole 记忆者角色
type MemorizerRole struct{}

// PainterRole 绘制者角色
type PainterRole struct {
	*ModelBuilder
	Prompts          *PainterPromptLibrary
	CustomCharacters map[string]CustomCharacterProfile
	Reference        ReferenceState
	ResultPath       string
}

// MusicianRole 演奏者角色
type MusicianRole struct {
	*ModelBuilder
	Draft           *MusicDraft
	AssembledDetail *MusicPieceDetail
	MaxIterations   int
	MinMelodyBars   int
	DefaultTempo    int
}

// ScreenshoterRole 截图者角色
type ScreenshoterRole struct {
	*ModelBuilder
	PendingShot   *ScreenshotShot
	PrimaryImage  string
	MaxIterations int
}
