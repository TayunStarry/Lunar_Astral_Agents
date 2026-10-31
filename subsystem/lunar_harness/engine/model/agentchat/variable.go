package agentchat

// variable.go 共享模型调用层的常量与全局变量（业务逻辑不得写在本文件）

import (
	"net/http"
	"regexp"
	"sync"
	"time"
)

const (
	// retryWait 模型调用失败后的重试等待
	retryWait = 10 * time.Second
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

	// promptMu 保护提示词缓存
	promptMu sync.Mutex
	// promptCache 提示词缓存（文件名 → 文本）
	promptCache = map[string]string{}
	// placeholderPattern 提示词占位符形态（{key}）
	placeholderPattern = regexp.MustCompile(`\{[A-Za-z0-9_\-]+\}`)

	// toolCallSeq 为缺失 ID 的工具调用分配唯一序号
	toolCallSeq int64
)
