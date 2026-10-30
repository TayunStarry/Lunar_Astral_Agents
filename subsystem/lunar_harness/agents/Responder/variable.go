package Responder

// variable.go 响应者队列与固定参数。

import (
	"time"
)

const (
	// roleLabel 控制台显示名（同时作为日志模块名，归入「系统日志 · 后勤」）
	roleLabel = "通用应答"
	// queueCapacity 未处理请求上限（超出直接拒绝，避免无限堆积）
	queueCapacity = 64
	// maxAttempts 单条请求的最大尝试次数（首次 + 三次重试）
	maxAttempts = 4
	// requestTimeout 单次模型调用超时
	requestTimeout = 5 * time.Minute
	// llamaChatPath llama.cpp 对话端点路径
	llamaChatPath = "/chat/completions"
)

var (
	// inbox 待处理请求队列（FIFO）
	inbox = make(chan *Request, queueCapacity)
)
