package napcat

import (
	"net/http"
	"sync"
	"time"
)

// ==== 桥接器全局变量 ====

// bridgeConfig 桥接器配置
var bridgeConfig BridgingConfig

// bridgeState 桥接器当前连接状态
var bridgeState BridgeState = BridgeDisconnected

// bridgeStateMutex 保护桥接器状态的并发访问
var bridgeStateMutex sync.RWMutex

// scanRetryCount 当前扫描重试次数
var scanRetryCount int

// scanTimer 扫描定时器
var scanTimer *time.Timer

// scanMutex 保护扫描相关变量的并发访问
var scanMutex sync.Mutex

// ==== 消息缓存全局变量 ====

// messageCache 消息缓存实例
var messageCache = &MessageCache{}

// maxCacheSize 最大缓存容量
const maxCacheSize = 20

// ==== Napcat 客户端全局变量 ====

// httpClient Napcat HTTP API 客户端
var httpClient = &http.Client{Timeout: 10 * time.Second}

// ==== 桥接器扫描配置常量 ====

const (
	// scanInterval 扫描间隔
	scanInterval = 10 * time.Second
	// maxScanRetries 最大扫描重试次数
	maxScanRetries = 10
)

// ==== 消息发送回调 ====

// SendMessageToAgent 向智能体推送消息的回调函数，由服务器层注册
var SendMessageToAgent func(content string, senderName string)

// SendImageToAgent 向智能体推送图片消息的回调函数，由服务器层注册
var SendImageToAgent func(images []string)
