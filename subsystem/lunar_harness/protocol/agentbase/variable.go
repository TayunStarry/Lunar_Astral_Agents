package agentbase

// variable.go 共享模型调用层的常量与全局变量（业务逻辑不得写在本文件）

import (
	"net/http"
	"regexp"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// retryWait 模型调用失败后的重试等待
	retryWait = 10 * time.Second
	// defaultStreamTimeout 单次模型调用默认超时
	defaultStreamTimeout = 5 * time.Minute
	// maxResponseBodyBytes 模型响应体读取上限
	maxResponseBodyBytes = 8 << 20
	// defaultMaxRounds 默认工具循环上限
	defaultMaxRounds = 10
	// defaultMaxRepeats 默认工具调用重复上限
	defaultMaxRepeats = 3
	// defaultMaxToolResultRunes 默认单条工具结果回填上限
	defaultMaxToolResultRunes = 6000
)

var (
	// modelOnce 模型配置只解析一次
	modelOnce sync.Once
	// modelName 多模态模型名
	modelName string
	// modelURL 模型服务地址（以 /v1 结尾）
	modelURL string
	// httpClient 模型请求客户端（影像理解可能持续较久）
	httpClient = &http.Client{Timeout: 10 * time.Minute}

	// observerMu 保护实况观察者
	observerMu sync.Mutex
	// observer 当前注入的实况观察者与计量回调
	observer Observer

	// repairMu 保护智能修复上报回调
	repairMu sync.Mutex
	// repairHook 智能修复上报回调（由装配层注入，避免 protocol 反向依赖 agents）
	repairHook func(agent string, err error)

	// promptMu 保护提示词缓存
	promptMu sync.Mutex
	// promptCache 提示词缓存（文件名 → 文本）
	promptCache = map[string]string{}
	// placeholderPattern 提示词占位符形态（{key}）
	placeholderPattern = regexp.MustCompile(`\{[A-Za-z0-9_\-]+\}`)

	// toolCallSeq 为缺失 ID 的工具调用分配唯一序号
	toolCallSeq atomic.Int64
)
