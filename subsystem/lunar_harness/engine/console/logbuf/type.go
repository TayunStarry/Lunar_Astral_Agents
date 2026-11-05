package logbuf

// type.go 可观测核心的数据类型：日志环形缓冲、智能体轮次缓冲、模型调用计量与导出结构。
// 这些类型不依赖任何 TUI 组件，供 console 视图、debugsnap 与 HTTP 导出层共用。

import (
	"time"
)

// ==== 日志管线类型 ====

// LogLine 统一日志管线的单行记录
type LogLine struct {
	Time   time.Time `json:"time"`   // 到达时刻
	Source string    `json:"source"` // 来源通道：stdout / log
	Level  string    `json:"level"`  // 级别：INFO / WARN / ERROR / FATAL（stdout 原始行为 RAW）
	Module string    `json:"module"` // 模块名
	Sub    string    `json:"sub"`    // 子模块名
	Text   string    `json:"text"`   // 纯文本消息体
}

// LogSource 日志视图来源枚举（同时作为「系统日志」屏的分类编号）
type LogSource int

// ==== 智能体流式事件类型 ====

// StreamKind 流式增量类别
type StreamKind string

// StreamEvent 智能体流式事件（由 syncFetch 流式桥推送）
type StreamEvent struct {
	Role string     `json:"role"` // 角色标签：对话者/绘制者/演奏者/记忆者/视觉理解/文献解析/网络搜索/主循环
	Kind StreamKind `json:"kind"` // 事件类别
	Text string     `json:"text"` // 增量文本或工具信息
}

// AgentTurn 智能体实况屏的单轮渲染内容
type AgentTurn struct {
	Role       string    `json:"role"`       // 角色标签
	Thinking   string    `json:"thinking"`   // 思考累计文本（超限截断保留尾部）
	Content    string    `json:"content"`    // 输出累计文本
	Tools      []string  `json:"tools"`      // 工具调用记录
	Active     bool      `json:"active"`     // 是否进行中
	StartedAt  time.Time `json:"started_at"` // 开始时刻
	FinishedAt time.Time `json:"finished_at"`
}

// TurnStoreInfo 实况轮次缓冲的容量与淘汰情况（数据导出写进元信息用）
type TurnStoreInfo struct {
	HistoryCap int // 历史轮次保留上限
	HistoryLen int // 当前保留的历史轮次条数（不含进行中轮次）
	Created    int // 进程启动以来累计完成（归档）的轮数
	Dropped    int // 因上限被淘汰的轮数
	TextCap    int // 单轮思考/输出文本上限（字符）
	HasActive  bool
}

// ==== 运行状态类型 ====

// ModelCallStats 模型调用累计计量
type ModelCallStats struct {
	Calls               int64         `json:"calls"`                 // 调用次数
	PromptTokens        int64         `json:"prompt_tokens"`         // 输入（上下文）token 累计
	CompletionTokens    int64         `json:"completion_tokens"`     // 输出 token 累计
	CachedPromptTokens  int64         `json:"cached_prompt_tokens"`  // 命中提示缓存的输入 token 累计
	MaxPromptTokens     int64         `json:"max_prompt_tokens"`     // 单次调用输入 token 峰值
	MaxCompletionTokens int64         `json:"max_completion_tokens"` // 单次调用输出 token 峰值
	GenerationNanos     int64         `json:"generation_nanos"`      // 生成耗时累计（纳秒）
	EstimatedCalls      int64         `json:"estimated_calls"`       // 使用估算的调用次数
	LastCallAt          time.Time     `json:"last_call_at"`          // 最近一次调用时刻
	LastDuration        time.Duration `json:"last_duration"`         // 最近一次生成耗时
	LastCompletion      int64         `json:"last_completion"`       // 最近一次输出 token
}

// Total 消耗总量（prompt + completion）
func (s ModelCallStats) Total() int64 {
	return s.PromptTokens + s.CompletionTokens
}

// AvgPerCall 平均每次调用消耗
func (s ModelCallStats) AvgPerCall() float64 {
	if s.Calls == 0 {
		return 0
	}
	return float64(s.Total()) / float64(s.Calls)
}

// AvgContext 平均上下文 token 数
func (s ModelCallStats) AvgContext() float64 {
	if s.Calls == 0 {
		return 0
	}
	return float64(s.PromptTokens) / float64(s.Calls)
}

// AvgCompletionPerCall 平均每轮输出 token 数
func (s ModelCallStats) AvgCompletionPerCall() float64 {
	if s.Calls == 0 {
		return 0
	}
	return float64(s.CompletionTokens) / float64(s.Calls)
}

// CacheHitRate 提示缓存命中率（命中 token / 输入 token），0~1
func (s ModelCallStats) CacheHitRate() float64 {
	if s.PromptTokens == 0 {
		return 0
	}
	return float64(s.CachedPromptTokens) / float64(s.PromptTokens)
}

// SpeedTokensPerSec 模型输出平均速度（tokens/s）
func (s ModelCallStats) SpeedTokensPerSec() float64 {
	if s.GenerationNanos <= 0 {
		return 0
	}
	return float64(s.CompletionTokens) / (float64(s.GenerationNanos) / float64(time.Second))
}

// ==== 实况导出结构 ====

// TranscriptTurn 一轮模型调用的导出结构
type TranscriptTurn struct {
	Role        string   `json:"role"`
	Active      bool     `json:"active"`      // 进行中（未收到结束事件）
	StartedAt   string   `json:"started_at"`  // 本地时间（毫秒），如 2026-10-16 21:20:31.482
	FinishedAt  string   `json:"finished_at"` // 进行中时为空
	DurationMS  int64    `json:"duration_ms"`
	Thinking    string   `json:"thinking"`
	Content     string   `json:"content"`
	Tools       []string `json:"tools"`
	ToolCount   int      `json:"tool_count"`
	ThinkingLen int      `json:"thinking_len"` // 字符数（含首尾空白）
	ContentLen  int      `json:"content_len"`
	// Truncated 真表示该轮思考或输出达到 turnTextCap、只保留了尾部
	Truncated bool `json:"truncated"`
}

// TranscriptMeta 导出的元信息：说明这份数据的完整性与来源
type TranscriptMeta struct {
	Kind        string         `json:"kind"`
	GeneratedAt string         `json:"generated_at"`
	Source      string         `json:"source"`
	Note        string         `json:"note"`
	HistoryCap  int            `json:"history_cap"`
	HistoryLen  int            `json:"history_len"`
	Created     int            `json:"created_total"`
	Dropped     int            `json:"dropped"`
	TextCap     int            `json:"text_cap"`
	TurnCount   int            `json:"turn_count"`
	HasActive   bool           `json:"has_active_turn"`
	ModelCalls  ModelCallStats `json:"model_calls"`
}

// TranscriptDump 实况导出的完整载荷
type TranscriptDump struct {
	Meta  TranscriptMeta   `json:"meta"`
	Turns []TranscriptTurn `json:"turns"`
}
