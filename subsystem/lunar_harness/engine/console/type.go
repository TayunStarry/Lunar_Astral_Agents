package console

import (
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logbuf"
)

// ==== 日志管线类型（实现位于 logbuf 子包，这里保留类型别名供视图使用） ====

// LogLine 统一日志管线的单行记录
type LogLine = logbuf.LogLine

// LogSource 日志视图来源枚举（同时作为「系统日志」屏的分类编号）
type LogSource = logbuf.LogSource

const (
	// SourceRaw 原始：展示全部完整日志（stdout 拦截行 + 全部结构化日志）
	SourceRaw = logbuf.SourceRaw
	// SourceWarn 警告：仅系统警告类信息（WARN / ERROR / FATAL）
	SourceWarn = logbuf.SourceWarn
	// SourceDraw 绘图：sd.cpp 引擎输出与绘图模块编排日志
	SourceDraw = logbuf.SourceDraw
	// SourceDialogue 对话：月华-对话系统（LunarCore）日志
	SourceDialogue = logbuf.SourceDialogue
	// SourceLife 生活：月华-生活系统（生活 / 观影 / 创作 / js 运行时）日志
	SourceLife = logbuf.SourceLife
	// SourceLogistics 后勤：月华-后勤系统（Tick 调度 / 通用应答 / 智能修复）日志
	SourceLogistics = logbuf.SourceLogistics
	// SourceTask 功能任务回显（代理/打包/端口释放/更新包）
	SourceTask = logbuf.SourceTask
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

// ==== 智能体流式事件类型（实现位于 logbuf 子包） ====

// StreamKind 流式增量类别
type StreamKind = logbuf.StreamKind

const (
	// KindThink 思考增量（reasoning_content）
	KindThink = logbuf.KindThink
	// KindContent 输出增量（content）
	KindContent = logbuf.KindContent
	// KindTool 工具调用（完整聚合事件：名称+参数）
	KindTool = logbuf.KindTool
	// KindToolResult 工具返回摘要
	KindToolResult = logbuf.KindToolResult
	// KindBegin 轮次开始
	KindBegin = logbuf.KindBegin
	// KindEnd 轮次结束
	KindEnd = logbuf.KindEnd
)

// StreamEvent 智能体流式事件
type StreamEvent = logbuf.StreamEvent

// AgentTurn 智能体实况屏的单轮渲染内容
type AgentTurn = logbuf.AgentTurn

// TurnStoreInfo 实况轮次缓冲的容量与淘汰情况
type TurnStoreInfo = logbuf.TurnStoreInfo

// ==== 运行状态类型 ====

// ModelCallStats 模型调用累计计量
type ModelCallStats = logbuf.ModelCallStats

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
