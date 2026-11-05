package console

import (
	"errors"
	"sync"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logbuf"
)

// ==== 管线容量常量 ====

const (
	// logStoreCap 每类日志环形缓冲最大条数（实际存储位于 logbuf）
	logStoreCap = logbuf.LogStoreCap
	// eventChanCap 流式事件通道容量（实际存储位于 logbuf）
	eventChanCap = logbuf.EventChanCap
	// turnTextCap 单轮思考/输出文本的最大保留字符数（实际存储位于 logbuf）
	turnTextCap = logbuf.TurnTextCap
	// turnHistoryCap 历史轮次最大保留数（实际存储位于 logbuf）
	turnHistoryCap = logbuf.TurnHistoryCap
	// statsInterval 运行状态采样间隔
	statsInterval = time.Second
)

// ==== 日志管线状态 ====

var (
	// pipelineOnce 保证 Install 只执行一次
	pipelineOnce sync.Once

	// notifyCh 日志/事件到达通知（TUI 唤醒信号，缓冲 1 防阻塞）
	notifyCh = make(chan struct{}, 1)

	// realStdout / realStderr 原始终端输出句柄（TUI 渲染目标，Install 时捕获）
	realStdout interface{ Write([]byte) (int, error) }
	realStderr interface{ Write([]byte) (int, error) }

	// intercepted 标记 stdout 是否已被管道接管
	intercepted bool
)

// ==== 运行状态采样 ====

var (
	// startTime 进程运行起点（Install 时刻）
	startTime = time.Now()
	// statsMu 保护最新快照
	statsMu sync.RWMutex
	// latestStats 最新状态快照
	latestStats Snapshot
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
	// statusLibrary 资源库前端在线探针
	statusLibrary func() bool
	// relaunchLunar 月华前端唤起器
	relaunchLunar func() error
	// relaunchCrystal 琉璃前端唤起器
	relaunchCrystal func() error
	// relaunchLibrary 资源库前端唤起器
	relaunchLibrary func() error
)

// ==== 功能子包日志/进度钩子（子包经 SetLogHook 注入，避免反向依赖） ====

// errNotWired 宿主未注入对应钩子时的统一错误
var errNotWired = errors.New("宿主程序未注入该能力（仅独立运行 TUI 模块时出现）")
