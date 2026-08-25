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

// ==== 单线程收发流程状态 ====

// requestQueue 待推送给月华的请求队列（FIFO）
var requestQueue []BridgeRequest

// awaitingResponse 是否存在已推送、正等待月华回应的请求
var awaitingResponse bool

// responseStarted 当前等待的请求是否已收到月华回应（用于区分真实的回合结束与空闲信号）
var responseStarted bool

// currentTarget 当前等待回应的请求目标
var currentTarget BridgeTarget

// flowMutex 保护收发流程状态的并发访问
var flowMutex sync.Mutex

// ==== 群聊缓存池 ====

// groupPools 各群聊的消息缓存池
var groupPools = make(map[int64]*GroupPool)

// groupNameCache 群名称缓存
var groupNameCache = make(map[int64]string)

// groupMutex 保护群聊缓存池与群名称缓存的并发访问
var groupMutex sync.Mutex

// ==== 消息缓存全局变量（供智能体主动拉取，兼容既有接口） ====

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

// SendVideoToAgent 向智能体推送视频地址的回调函数，由服务器层注册
// 参数为视频 URL / 本地路径列表，由智能体通过 pullVideoUrl 拉取并理解视频内容
var SendVideoToAgent func(urls []string)
