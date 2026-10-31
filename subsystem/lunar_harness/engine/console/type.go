package console

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

const (
	// SourceRaw 原始：展示全部完整日志（stdout 拦截行 + 全部结构化日志）
	SourceRaw LogSource = iota
	// SourceWarn 警告：仅系统警告类信息（WARN / ERROR / FATAL）
	SourceWarn
	// SourceDraw 绘图：sd.cpp 引擎输出与绘图模块编排日志
	SourceDraw
	// SourceDialogue 对话：月华-对话系统（LunarCore）日志
	SourceDialogue
	// SourceLife 生活：月华-生活系统（生活 / 观影 / 创作 / js 运行时）日志
	SourceLife
	// SourceLogistics 后勤：月华-后勤系统（Tick 调度 / 通用应答 / 智能修复）日志
	SourceLogistics
	// SourceTask 功能任务回显（代理/打包/端口释放/更新包）
	SourceTask
	// sourceCount 来源总数（环形缓冲数组长度）
	sourceCount
)

// logCategories 「系统日志」屏的分类顺序（左右方向键依次切换）
var logCategories = []LogSource{
	SourceRaw, SourceWarn, SourceDraw, SourceDialogue, SourceLife, SourceLogistics,
}

// logCategoryTitles 分类显示名
var logCategoryTitles = map[LogSource]string{
	SourceRaw:       "原始",
	SourceWarn:      "警告",
	SourceDraw:      "绘图",
	SourceDialogue:  "对话",
	SourceLife:      "生活",
	SourceLogistics: "后勤",
}

// ==== 智能体流式事件类型 ====

// StreamKind 流式增量类别
type StreamKind string

const (
	// KindThink 思考增量（reasoning_content）
	KindThink StreamKind = "think"
	// KindContent 输出增量（content）
	KindContent StreamKind = "content"
	// KindTool 工具调用（完整聚合事件：名称+参数）
	KindTool StreamKind = "tool"
	// KindToolResult 工具返回摘要
	KindToolResult StreamKind = "tool_result"
	// KindBegin 轮次开始
	KindBegin StreamKind = "begin"
	// KindEnd 轮次结束
	KindEnd StreamKind = "end"
)

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

// Snapshot 运行状态快照（运行状态屏渲染数据源）
type Snapshot struct {
	BuildVersion string         `json:"build_version"`  // 构建版本号（宿主注入，取自端点自述文档）
	Uptime       time.Duration  `json:"uptime"`         // 运行时间
	CPUPercent   float64        `json:"cpu_percent"`    // 进程 CPU 占用（%）
	WorkingSetMB float64        `json:"working_set_mb"` // 进程工作集内存（MB）
	HeapMB       float64        `json:"heap_mb"`        // Go 堆占用（MB）
	SysAvailMB   float64        `json:"sys_avail_mb"`   // 系统可用物理内存（MB）
	Goroutines   int            `json:"goroutines"`     // goroutine 数量
	Stats        ModelCallStats `json:"stats"`          // 模型调用计量
}
