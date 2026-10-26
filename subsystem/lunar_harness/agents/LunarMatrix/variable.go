package LunarMatrix

// variable.go 常量与全局变量集中声明。

import (
	"net/http"
	"sync"
	"time"
)

const (
	// roleLabel 该智能体在「核心智能 实况」中的角色标签（中文译名；模块名仍为 Lunar Matrix）
	roleLabel = "互助论坛"
	// defaultModelName 未配置多模态模型时的兜底名称
	defaultModelName = "primary_reasoning"
	// maxToolRounds 单次需求允许的工具调用轮次上限
	maxToolRounds = 8
	// historyLimitMessages 独立上下文保留的消息条数（约 20 轮问答）
	historyLimitMessages = 40
	// runTimeout 单次需求的整体超时
	runTimeout = 2 * time.Minute
	// modelTimeout 单次模型调用超时
	modelTimeout = 90 * time.Second
	// maxPostPreview 工具结果里的发言摘要长度
	maxPostPreview = 1200
	// maxBodyPreview 工具结果里的话题正文摘要长度
	maxBodyPreview = 240
)

var (
	// runMu 串行化自然语言需求，避免同一身份并发发言导致轮次互相覆盖
	runMu sync.Mutex

	// httpClient 模型调用客户端
	httpClient = &http.Client{Timeout: modelTimeout}

	// 模型端点只解析一次
	modelOnce sync.Once
	modelName string
	modelURL  string
	modelKey  string

	// historyMu 保护独立上下文
	historyMu sync.Mutex
	// history 最近若干轮「用户需求 + 最终答复」
	history []chatMessage

	// observerMu 保护实况观察者
	observerMu sync.Mutex
	obsBegin   func(role string)
	obsDelta   func(role, kind, text string)
	obsEnd     func(role string)
	obsMeter   func(prompt, completion, cached int64, generation time.Duration, estimated bool)
)
