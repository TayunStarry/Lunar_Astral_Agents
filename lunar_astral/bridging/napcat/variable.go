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

// ==== 消息缓存全局变量 ====

// messageCache 消息缓存实例
var messageCache = &MessageCache{}

// maxCacheSize 最大缓存容量
const maxCacheSize = 20

// ==== Napcat 客户端全局变量 ====

// httpClient Napcat HTTP API 客户端
var httpClient = &http.Client{Timeout: 10 * time.Second}

// ==== 消息发送回调 ====

// SendMessageToAgent 向智能体推送消息的回调函数，由服务器层注册
// 参数为 OpenAI 格式消息列表 []map[string]interface{}
var SendMessageToAgent func(messages []map[string]interface{})
