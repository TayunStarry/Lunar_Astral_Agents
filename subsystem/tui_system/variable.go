package TuiSystem

import (
	"errors"
	"sync"
	"time"
)

// ==== 管线容量常量 ====

const (
	// logStoreCap 每类日志环形缓冲最大条数
	logStoreCap = 1000
	// eventChanCap 流式事件通道容量（TUI 未消费时丢弃最旧事件由轮次缓冲兜底）
	eventChanCap = 512
	// turnTextCap 单轮思考/输出文本的最大保留字符数（超出保留尾部）
	turnTextCap = 48 * 1024
	// turnHistoryCap 历史轮次最大保留数
	turnHistoryCap = 30
	// statsInterval 运行状态采样间隔
	statsInterval = time.Second
)

// ==== 日志管线状态 ====

var (
	// pipelineOnce 保证 Install 只执行一次
	pipelineOnce sync.Once

	// storeMu 保护全部日志环形缓冲
	storeMu sync.RWMutex
	// logStores 各来源日志缓冲（下标对应 LogSource）
	logStores [sourceCount][]LogLine

	// notifyCh 日志/事件到达通知（TUI 唤醒信号，缓冲 1 防阻塞）
	notifyCh = make(chan struct{}, 1)

	// realStdout / realStderr 原始终端输出句柄（TUI 渲染目标，Install 时捕获）
	realStdout interface{ Write([]byte) (int, error) }
	realStderr interface{ Write([]byte) (int, error) }

	// intercepted 标记 stdout 是否已被管道接管
	intercepted bool
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
	// turnDropped 因 turnHistoryCap 上限被淘汰的轮数（导出时据此如实标注数据完整性）
	turnDropped int
)

// ==== 模型调用计量 ====

var (
	// meterMu 保护计量数据
	meterMu sync.Mutex
	// meterStats 计量数据
	meterStats ModelCallStats
)

// ==== 运行状态采样 ====

var (
	// startTime 进程运行起点（Install 时刻）
	startTime = time.Now()
	// statsMu 保护最新快照
	statsMu sync.RWMutex
	// latestStats 最新状态快照
	latestStats Snapshot
	// buildVersionMu 保护宿主注入的构建版本号
	buildVersionMu sync.RWMutex
	// buildVersion 宿主注入的构建版本号（显示于「运行状态」屏）
	buildVersion string
	// lastProcTimes 上次采样的进程 CPU 时间（用户态+内核态，100ns 单位）
	lastProcTimes int64
	// lastSampleAt 上次采样时刻
	lastSampleAt time.Time
	// winProcHandle 当前进程伪句柄（Windows 采样用）
	winProcHandle uintptr
)

// ==== 前端管理钩子（宿主注入，TUI 不反向依赖宿主） ====

var (
	// frontendStatusMu 保护前端状态/唤起钩子的并发读写
	frontendStatusMu sync.RWMutex

	// statusLunar 月华前端在线探针
	statusLunar func() bool
	// statusCrystal 琉璃前端在线探针
	statusCrystal func() bool
	// relaunchLunar 月华前端唤起器
	relaunchLunar func() error
	// relaunchCrystal 琉璃前端唤起器
	relaunchCrystal func() error
)

// ==== 功能子包日志/进度钩子（子包经 SetLogHook 注入，避免反向依赖） ====

// errNotWired 宿主未注入对应钩子时的统一错误
var errNotWired = errors.New("宿主程序未注入该能力（仅独立运行 TUI 模块时出现）")
