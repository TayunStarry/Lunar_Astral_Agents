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

// ==== 断线重连策略 ====

// bridgeReconnectInterval 断联后重连的等待间隔
const bridgeReconnectInterval = 5 * time.Second

// bridgeMaxReconnectAttempts 单次断联允许的最大重连次数
const bridgeMaxReconnectAttempts = 3

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

// memberNameCache 群成员名称缓存：群号 → (QQ号 → 群名片/昵称)
// 供入站 @ 渲染为 [对 <用户名> 说]、出站 [对 <用户名> 说] 反向转换为真实 @ 段
var memberNameCache = make(map[int64]map[int64]string)

// memberListWarmed 已预取过成员列表的群（每进程最多拉取一次全量列表）
var memberListWarmed = make(map[int64]bool)

// groupMutex 保护群聊缓存池、群名称缓存与群成员名称缓存的并发访问
var groupMutex sync.Mutex

// ==== 当前登录账号信息缓存 ====

// botInfoMutex 保护当前登录账号信息缓存
var botInfoMutex sync.Mutex

// botInfoLoaded 当前登录账号信息是否已成功加载（失败时下次调用重试）
var botInfoLoaded bool

// botUserID 当前登录账号 QQ 号
var botUserID int64

// botNickname 当前登录账号昵称
var botNickname string

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

// SendAudioToAgent 向智能体推送语音/音频地址的回调函数，由服务器层注册
// 参数为语音 URL / 本地路径 / base64 data URI 列表，由智能体通过 pullAudioUrl 拉取并理解音频内容
var SendAudioToAgent func(urls []string)
