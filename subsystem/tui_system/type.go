package TuiSystem

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

// LogSource 日志视图来源枚举
type LogSource int

const (
	// SourceRaw 原始日志（stdout 拦截行 + 全部结构化日志）
	SourceRaw LogSource = iota
	// SourceLunarJS 月华智能体 JS 层日志（LunarCore-JavaScript）
	SourceLunarJS
	// SourceLTP9JS ltp9 沙箱 JS 层日志（StarLTP console）
	SourceLTP9JS
	// SourceDraw sd.cpp 绘图引擎日志（引擎 stdout/stderr 按行接入 + 绘图模块编排日志）
	SourceDraw
	// SourceGoWarn Go 模块警告与错误
	SourceGoWarn
	// SourceTask 功能任务回显（代理/打包/端口释放/更新包）
	SourceTask
	// sourceCount 来源总数（环形缓冲数组长度）
	sourceCount
)

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
	Role string     `json:"role"` // 角色标签：对话者/绘制者/演奏者/记忆者/感知者/描述者/主循环
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
	Calls            int64         `json:"calls"`              // 调用次数
	PromptTokens     int64         `json:"prompt_tokens"`      // 上下文 token 累计
	CompletionTokens int64         `json:"completion_tokens"`  // 输出 token 累计
	GenerationNanos  int64         `json:"generation_nanos"`   // 生成耗时累计（纳秒）
	EstimatedCalls   int64         `json:"estimated_calls"`    // 使用估算的调用次数
	LastCallAt       time.Time     `json:"last_call_at"`       // 最近一次调用时刻
	LastDuration     time.Duration `json:"last_duration"`      // 最近一次生成耗时
	LastCompletion   int64         `json:"last_completion"`    // 最近一次输出 token
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

// SpeedTokensPerSec 模型输出平均速度（tokens/s）
func (s ModelCallStats) SpeedTokensPerSec() float64 {
	if s.GenerationNanos <= 0 {
		return 0
	}
	return float64(s.CompletionTokens) / (float64(s.GenerationNanos) / float64(time.Second))
}

// Snapshot 运行状态快照（运行状态屏渲染数据源）
type Snapshot struct {
	Uptime       time.Duration  `json:"uptime"`        // 运行时间
	CPUPercent   float64        `json:"cpu_percent"`   // 进程 CPU 占用（%）
	WorkingSetMB float64        `json:"working_set_mb"`// 进程工作集内存（MB）
	HeapMB       float64        `json:"heap_mb"`       // Go 堆占用（MB）
	SysAvailMB   float64        `json:"sys_avail_mb"`  // 系统可用物理内存（MB）
	Goroutines   int            `json:"goroutines"`    // goroutine 数量
	Stats        ModelCallStats `json:"stats"`         // 模型调用计量
}
