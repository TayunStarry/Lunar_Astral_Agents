package DialogueBridging

// variable.go 通讯桥接模块的全局变量与机制参数：上半部为智能体模型调用侧，
// 下半部为 QQ 桥接层（NapCat 协议）。

import (
	"sync"
	"time"
)

// ==================== 智能体模型调用 ====================

const (
	// roleLabel 该智能体在「推理实况」中的角色标签
	roleLabel = "通讯桥接"
	// maxDirectoryEntries 提示词里最多列出多少个群 / 好友，避免撑爆上下文
	maxDirectoryEntries = 80
	// maxSendImages 单次消息最多携带的图片数（超过则分批发送）
	maxSendImages = 9
	// maxSendFiles 单次请求最多投递的文件数
	maxSendFiles = 10
	// loopMaxRetries 模型重复输出时的额外重试次数
	loopMaxRetries = 2
)

// runMu 串行化投递：同一时间只处理一个需求，避免并发写 QQ 时报告互相覆盖
var runMu sync.Mutex

// 最近一次投递记录（数据导出用）
var (
	lastMu       sync.Mutex
	lastDelivery *deliveryRecord
)

// ==== 主动投递白名单 ====
//
// 默认启用白名单模式：主动发送前必须校验目标是否在 bridging_users 内。
const (
	// whitelistBlockedText 目标不在白名单时的固定提示文本（需求要求原样返回）
	whitelistBlockedText = "对方不在白名单中, 请勿随意打扰他人"
	// whitelistConfirmHint 绕过白名单的确认引导
	whitelistConfirmHint = "\n如需绕过白名单发送，请回复「确认绕过白名单」并重新说明目标与内容。"
	// whitelistBypassTTL 绕过确认的有效期：超时后必须重新发起并再次确认
	whitelistBypassTTL = 10 * time.Minute
)

// whitelistConfirmPhrases 明确确认绕过白名单的口令
var whitelistConfirmPhrases = []string{
	"确认绕过白名单",
	"确认绕过",
	"确认发送",
	"强制发送",
	"仍然发送",
	"同意发送",
}

// whitelistPending 记录已提示、等待对话者确认的目标（key → 过期时间）
var (
	whitelistPendingMu sync.Mutex
	whitelistPending   = map[string]time.Time{}
)

// 模型实况观察者：由 app 注入控制台的 StreamBegin/Delta/End 与计量。
var (
	observerMu sync.Mutex
	obsBegin   func(role string)
	obsDelta   func(role, kind, text string)
	obsEnd     func(role string)
	obsMeter   func(prompt, completion, cached int64, generation time.Duration, estimated bool)
)

// ==================== QQ 桥接层 ====================

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

// responseStarted 当前等待的请求是否已收到终稿回应（用于区分真实的回合结束与空闲信号；
// 快速应答、工具轮次正文、思考块以及中途的音乐 / 图片都不会置位）
var responseStarted bool

// currentTarget 当前等待回应的请求目标
var currentTarget BridgeTarget

// flowMutex 保护收发流程状态的并发访问
var flowMutex sync.Mutex

// ==== 群聊缓存池 ====

// groupPools 各群聊的消息缓存池
var groupPools = make(map[int64]*GroupPool)

// ==== 私聊缓存池 ====

// privatePools 各私聊用户的消息缓存池（私聊也先攒批再判定，避免图片与要求被拆成两轮）
var privatePools = make(map[int64][]GroupPoolEntry)

// privateTimers 各私聊用户的合并窗口定时器（新消息到达时重置）
var privateTimers = make(map[int64]*time.Timer)

// privateBatchWindow 私聊连续消息的合并窗口（默认值，可经配置覆盖）
const privateBatchWindow = 1500 * time.Millisecond

// privatePoolMaxSize 私聊缓存池上限
const privatePoolMaxSize = 20

// groupMutex 保护群聊缓存池的并发访问
var groupMutex sync.Mutex

// ==== 消息发送回调 ====

// SendMessageToAgent 向智能体推送消息的回调函数，由服务器层注册
// 参数为 OpenAI 格式消息列表 []map[string]any
var SendMessageToAgent func(messages []map[string]any)

// SendAudioToAgent 向智能体推送语音/音频地址的回调函数，由服务器层注册
// 参数为语音 URL / 本地路径 / base64 data URI 列表，包装为媒体内容项按到达顺序写入智能体统一未读队列
var SendAudioToAgent func(urls []string)
