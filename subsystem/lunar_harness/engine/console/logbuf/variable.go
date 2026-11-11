package logbuf

// variable.go 可观测核心的常量与全局状态。

import (
	"sync"
	"time"
)

// ==== 缓冲容量常量 ====

const (
	// LogStoreCap 每类日志环形缓冲最大条数
	LogStoreCap = 1000
	// EventChanCap 流式事件通道容量（TUI 未消费时丢弃最旧事件由轮次缓冲兜底）
	EventChanCap = 512
	// TurnTextCap 单轮思考/输出文本的最大保留字符数（超出保留尾部）
	TurnTextCap = 48 * 1024
	// TurnHistoryCap 历史轮次最大保留数
	// 调度者一轮规划会连续产生多次模型调用（每次工具往返算一轮），30 轮很容易被
	// 执行体挤出导出记录，事后无法核对规划提示是否带上了路径快照。
	TurnHistoryCap = 120
)

// ==== 日志来源枚举 ====

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
	// SourceCount 来源总数（环形缓冲数组长度）
	SourceCount
)

// ==== 流式事件类别 ====

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

// ==== 日志环形缓冲状态 ====

var (
	// storeMu 保护全部日志环形缓冲
	storeMu sync.RWMutex
	// logStores 各来源日志缓冲（下标对应 LogSource）
	logStores [SourceCount][]LogLine
	// logStoreVersions 各来源日志追加版本号：环形缓冲满后长度恒为 LogStoreCap，
	// 不能再用 len(logs) 判断“有没有新内容”（否则视图缓存永远不刷新）。
	logStoreVersions [SourceCount]uint64
)

// ==== 智能体流式状态 ====

var (
	// streamMu 保护轮次缓冲
	streamMu sync.Mutex
	// currentTurn 进行中的轮次
	currentTurn *AgentTurn
	// turnHistory 历史轮次（旧 → 新）
	turnHistory []*AgentTurn
	// streamCh 流式事件通道（TUI 激活时消费；未激活时仅写入轮次缓冲）
	streamCh chan StreamEvent
	// streamChMu 保护 streamCh 的注册/读取
	streamChMu sync.RWMutex
	// turnCreated 进程启动以来累计完成（归档）的轮数
	turnCreated int
	// turnDropped 因 TurnHistoryCap 上限被淘汰的轮数（导出时据此如实标注数据完整性）
	turnDropped int
)

// ==== 模型调用计量 ====

const (
	// meterFileName 模型调用计量的持久化文件名（保存在 <LocalDir>/database 下）
	meterFileName = "model_stats.json"
	// meterSaveInterval 入账后的自动落盘间隔：意外关闭最多丢该间隔内的累计值
	meterSaveInterval = 5 * time.Second
)

var (
	// meterMu 保护计量数据
	meterMu sync.Mutex
	// meterStats 计量数据
	meterStats ModelCallStats
	// meterDirty 是否存在尚未落盘的累计值
	meterDirty bool
	// meterSaving 是否已有后台落盘任务在执行（避免并发写同一文件）
	meterSaving bool
	// meterLastSave 上次成功进入落盘流程的时刻
	meterLastSave time.Time
)

// ==== 构建版本 ====

var (
	// buildVersionMu 保护宿主注入的构建版本号
	buildVersionMu sync.RWMutex
	// buildVersion 宿主注入的构建版本号（显示于「运行状态」屏与自述文档）
	buildVersion string
)
