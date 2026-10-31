package LunarPersonality

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/resource"
)

// ==== 消息与内容 ====

// 消息内容类型码（与 OpenAI 兼容协议的内容项 type 一一对应）
const (
	ContentText       = "text"
	ContentImageURL   = "image_url"
	ContentInputAudio = "input_audio"
	ContentVideoURL   = "video_url"
	ContentAudioURL   = "audio_url"
	// ContentAudioHandle 音频转写悬空句柄内容项（接入阶段完成 ASR 后由服务端替换）
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

// URLPayload 单个 URL 载荷（图片 / 视频 / 音频共用）
type URLPayload struct {
	URL string `json:"url"`
}

// AudioPayload 内联音频载荷
type AudioPayload struct {
	Data   string `json:"data"`
	Format string `json:"format"`
}

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
	Type        string              `json:"type"`
	Text        string              `json:"text,omitempty"`
	ImageURL    *URLPayload         `json:"image_url,omitempty"`
	InputAudio  *AudioPayload       `json:"input_audio,omitempty"`
	VideoURL    *URLPayload         `json:"video_url,omitempty"`
	AudioURL    *URLPayload         `json:"audio_url,omitempty"`
	AudioHandle *AudioHandlePayload `json:"audio_handle,omitempty"`
	Quote       *QuotePayload       `json:"quote,omitempty"`
}

// MarshalJSON 序列化内容项：文本项必须始终带 text 键。
// 严格解析的 OpenAI 兼容服务端（如 llama.cpp）会对缺失的 text 键直接报错，
// 空文本项若因 omitempty 省略该键，整次请求都会 500。
func (c ContentItem) MarshalJSON() ([]byte, error) {
	type contentItemJSON struct {
		Type        string              `json:"type"`
		Text        *string             `json:"text,omitempty"`
		ImageURL    *URLPayload         `json:"image_url,omitempty"`
		InputAudio  *AudioPayload       `json:"input_audio,omitempty"`
		VideoURL    *URLPayload         `json:"video_url,omitempty"`
		AudioURL    *URLPayload         `json:"audio_url,omitempty"`
		AudioHandle *AudioHandlePayload `json:"audio_handle,omitempty"`
		Quote       *QuotePayload       `json:"quote,omitempty"`
	}
	out := contentItemJSON{
		Type:        c.Type,
		ImageURL:    c.ImageURL,
		InputAudio:  c.InputAudio,
		VideoURL:    c.VideoURL,
		AudioURL:    c.AudioURL,
		AudioHandle: c.AudioHandle,
		Quote:       c.Quote,
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

// ResourceAudioTask 统一资源转换中的音频批处理单元：
// 先登记句柄并转写，全部转写完成后再统一卸载 ASR 模型并回填文本。
type ResourceAudioTask struct {
	// Source 音频来源（data URI / 本地地址 / 远程地址）
	Source string
	// Name 展示文件名
	Name string
	// Origin 来源语义
	Origin string
	// Message 所属消息下标
	Message int
	// Slot 文本回填位置（该消息内容项数组的下标）
	Slot int
}

// urlExtractBudget 一次未读转换内的链接提炼配额：同一条消息里多条陌生链接时，
// 只对前几条现场提炼，其余保留句柄与地址，等月华按需用 use_resource 取回。
type urlExtractBudget struct {
	// used 已提炼次数
	used int
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
	PromptTokens        int64               `json:"prompt_tokens"`
	CompletionTokens    int64               `json:"completion_tokens"`
	TotalTokens         int64               `json:"total_tokens"`
	PromptTokensDetails *promptTokenDetails `json:"prompt_tokens_details,omitempty"`
}

// promptTokenDetails 输入 token 的细分（提示缓存命中数）
type promptTokenDetails struct {
	CachedTokens int64 `json:"cached_tokens"`
}

// cachedTokens 命中提示缓存的输入 token 数（未回传时为 0）
func (u *TokenUsage) cachedTokens() int64 {
	if u == nil || u.PromptTokensDetails == nil {
		return 0
	}
	return u.PromptTokensDetails.CachedTokens
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
	Src   string `json:"src,omitempty"`
	Label string `json:"label"`
	// Handle 音频转写悬空句柄（无实体音频，仅保留描述）
	Handle string `json:"handle,omitempty"`
	// Text 音频转写文本
	Text string `json:"text,omitempty"`
}

// CachedMessage 缓存消息（与前端 messages.js 结构一致）
type CachedMessage struct {
	ID          string             `json:"id"`
	Role        string             `json:"role"`
	Categories  []string           `json:"categories"`
	Content     string             `json:"content"`
	Sender      string             `json:"sender,omitempty"`
	Attachments []CachedAttachment `json:"attachments,omitempty"`
	Quotes      []QuotedMessage    `json:"quotes,omitempty"`
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

// ==== 统一资源句柄系统类型别名 ====

type ResourceEntry = resource.Resource
type ResourceReadResult = resource.ReadResult
type ResourceSection = resource.Section
type ResourceSearchHit = resource.SearchHit
type ResourceStats = resource.StatsReport

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
	// Handles 本批音频转写生成的悬空句柄（按消息内音频顺序；无音频时为空）
	Handles []string `json:"handles,omitempty"`
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

// LTPXRemoteToolRef 可调用应用（名称 + 能力描述）
type LTPXRemoteToolRef struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ==== 数据导出 ====

// DumpRoleEntry 可导出角色
type DumpRoleEntry struct {
	Label   string
	Builder *ModelBuilder
}

// ==== Tick 调度 ====

// TickSnapshot 一次 Tick 判定所需的只读快照。
type TickSnapshot struct {
	Now            time.Time
	IdleTicks      int
	Unread         int
	ViewingPending bool
	SalvagePending int
}

// TickSystem 可注册的 Tick 体系。
//
// 新增体系只需实现该接口并注册，主调度逻辑不需要改动：
//   - Priority 越大越先判定（对话 300 / 后勤 250 / 观影 200 / 生活 100）；
//   - Ready 只读判定，不得消费消息或改状态；
//   - IdleReset 本次运行结束后把空闲进度重置到该值（决定下一次生活冷却）；
//   - Run 执行一个原子工作单元，返回即表示释放 Tick。
type TickSystem interface {
	ID() string
	Priority() int
	Ready(TickSnapshot) (bool, string)
	IdleReset() int
	Run(ctx context.Context) error
}

// idleKeeper 可选能力：把本次运行当作一格空闲推进（冷却 +1 而不是重置）。
// 后勤体系代表「系统在空闲中工作」，因此实现该接口。
type idleKeeper interface {
	KeepIdle() bool
}

// TickState 调度器持久化状态。
type TickState struct {
	Version       int    `json:"version"`
	IdleTicks     int    `json:"idle_ticks"`
	LastSystem    string `json:"last_system"`
	LastReason    string `json:"last_reason"`
	LastTriggerAt int64  `json:"last_trigger_at"`
	UpdatedAt     int64  `json:"updated_at"`
}

// RuntimeGlobalMessage 对话者运行时消息的全局共享数据。
type RuntimeGlobalMessage struct {
	Timestamp    int64    `json:"timestamp"`
	MoodLabel    string   `json:"mood_label"`
	LifeRecord   string   `json:"life_record"`
	PlanSummary  string   `json:"plan_summary"`
	EmotionMarks []string `json:"emotion_marks,omitempty"`
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
}

// MemorizerRole 记忆入库与检索流程（非独立智能体：不持有模型上下文，也不参与 function calling）
type MemorizerRole struct{}

// coreRoleSnapshot 核心人格内某个角色的上下文视图（拯救者读取与修复时使用）
type coreRoleSnapshot struct {
	ID      string
	Label   string
	Builder *ModelBuilder
}

// PainterRole 绘制者角色
type PainterRole struct {
	*ModelBuilder
	Prompts          *PainterPromptLibrary
	CustomCharacters map[string]CustomCharacterProfile
	Reference        ReferenceState
	// RuntimeArtwork 最近一次产出的画作：新一轮任务未显式指定参考图时自动沿用它做图生图基座
	RuntimeArtwork ReferenceState
	// referenceFromRuntime 当前 Reference 是否来自运行时画作（此时参考图已由运行时消息挂载，任务正文不再重复附图）
	referenceFromRuntime bool
	ResultPath           string
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
