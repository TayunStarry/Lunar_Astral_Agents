package main

import (
	"encoding/json"
	"time"
)

// 代码块状态枚举
type BlockState int

const (
	StateReady    BlockState = iota // 就绪状态
	StateWaiting                     // 等待状态
	StateTerminated                  // 终止状态
)

// 变量访问模式
type VarMode int

const (
	VarModeNormal VarMode = iota // 普通模式
	VarModeReadOnly              // 只读模式
	VarModeWriteOnly             // 只写模式
)

// 变量结构体
type Variable struct {
	Value string
	Mode  VarMode
}

// 指针类型
type PointerType int

const (
	PointerClass  PointerType = iota // Class 指针
	PointerSnapshot                  // Snapshot 指针
)

// 指针结构体
type Pointer struct {
	Type     PointerType
	Lines    []string      // Class 指针的代码行
	Snapshot SnapshotState // Snapshot 指针的状态
}

// Snapshot 状态
type SnapshotState struct {
	Variables map[string]Variable
	Tick      int64
}

// 等待条件类型
type WaitType int

const (
	WaitNone       WaitType = iota
	WaitVariable            // 等待变量非空
	WaitSleep               // 等待睡眠时间
	WaitProcess             // 等待进程退出
	WaitCatch               // 等待 @catch 捕获
)

// 等待条件
type WaitCondition struct {
	Type           WaitType
	VariableName   string
	SleepUntil     time.Time
	ProcessExitCh  chan int
	CatchVariable  string
	CatchSubstring string
}

// 代码块结构体
type CodeBlock struct {
	ID           string
	Lines        []string
	PC           int // Program Counter - 当前执行到第几行
	State        BlockState
	WaitCond     WaitCondition
	RetryConfig  *RetryConfig
	LimitTime    *time.Time // 超时时间
	LocalExitCode string     // 局部退出码 #?
}

// 重试配置
type RetryConfig struct {
	CooldownMs   int
	MaxRetries   int
	ErrorVar     string
	CurrentRetry int
	OriginalLines []string
}

// 运行时消息类型
type MessageType string

const (
	MsgLog     MessageType = "log"
	MsgError   MessageType = "error"
	MsgTick    MessageType = "tick"
	MsgResult  MessageType = "result"
)

// 运行时消息
type RuntimeMessage struct {
	Type    MessageType `json:"type"`
	Content string      `json:"content"`
	Time    int64       `json:"time"`
}

// WebSocket 请求
type WSRequest struct {
	Type  string          `json:"type"` // "inject", "invoke", "start", "stop"
	Data  json.RawMessage `json:"data"`
}

// Inject 请求数据
type InjectData struct {
	Lines []string `json:"lines"`
}

// Invoke 请求数据
type InvokeData struct {
	PointerName string `json:"pointerName"`
}
