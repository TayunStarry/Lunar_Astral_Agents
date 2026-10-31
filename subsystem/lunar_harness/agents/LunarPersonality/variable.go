package LunarPersonality

import (
	"context"
	"sync"
	"sync/atomic"

	"LunarSubsystem/LunarHarness/agents/BilibiliViewer"
	"LunarSubsystem/LunarHarness/agents/DialogueBridging"
	"LunarSubsystem/LunarHarness/agents/WebSearch"
	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// ==== 日志标签 ====

const lunarCoreModule = "LunarCore"

// lunarLoopRetries 模型输出陷入重复时，单次调用的额外重试次数。
// 对话者的正式回答对完整性要求高，命中后先重试一次；仍失败交给对话链的失败处理。
const lunarLoopRetries = 1

// desktopActorStopHook 对话者开始推理前需终止的外部桌面智能体中断器（宿主注入，返回是否确有任务被中断）。
var desktopActorStopHook atomic.Pointer[func() bool]

// ==== 队列与思考状态 ====

var (
	// unreadMu 保护统一未读队列与淘汰消息缓冲池
	unreadMu sync.Mutex
	// unreadContext 统一未读队列：文本与视频/音频 URL 内容项按到达时序混排
	unreadContext []PostMessage
	// unreadRecords 对话者淘汰的历史消息（待记忆流程写入记忆库）
	unreadRecords []PostMessage
	// unreadEmotionMarks 本轮未读消息命中的情感标识（用户名 → 情感标签）
	unreadEmotionMarks = map[string]string{}
	// lastUserTextMu 保护最新用户发言
	lastUserTextMu sync.Mutex
	// lastUserText 最新用户发言（供生活体系更新心情/情感取向）
	lastUserText string

	// reasoningInProgress 对话模型是否正在跑（为 true 时下一个 tick 直接跳过）
	reasoningInProgress bool
	// fastAckSent 本轮思考链是否已经发过高耗时任务的快速应答
	fastAckSent bool
	// turnFromQQ 本轮是否由 QQ 适配器消息触发
	turnFromQQ bool
	// finalResponse 最终响应
	finalResponse string
	// lastFailure 本轮应答失败原因（空串表示本轮没有失败）
	lastFailure string
	// memoryReady 记忆库是否已初始化
	memoryReady bool

	// stateMu 保护上述思考链状态字段
	stateMu sync.Mutex
)

// ==== 运行时实例 ====

var (
	// runtimeMu 串行化启动/停止
	runtimeMu sync.Mutex
	// running 循环是否已启动
	running atomic.Bool
	// agentReady 智能体是否已就绪（数据导出等宿主侧入口据此判断）
	agentReady atomic.Bool
	// stopCh 循环停止信号
	stopCh chan struct{}
	// loopWG 循环退出等待
	loopWG sync.WaitGroup
	// responderCtx 响应者独立 Tick 的上下文（停止时取消，中止在跑的外部推理）
	responderCtx context.Context
	// responderCancel 取消响应者 Tick
	responderCancel context.CancelFunc

	// dialogueRole 对话者角色实例
	dialogueRole *DialogueRole
	// memorizerRole 记忆流程实例（不是独立智能体：只做记忆入库与检索摘要）
	memorizerRole *MemorizerRole
	// painterRole 绘制者角色实例
	painterRole *PainterRole
	// musicianRole 演奏者角色实例
	musicianRole *MusicianRole
	// screenshoterRole 截图者角色实例
	screenshoterRole *ScreenshoterRole
)

// ==== 工具注册表 ====

var (
	// toolMu 保护工具处理函数注册表
	toolMu sync.RWMutex
	// toolFunctions 工具名 → 处理函数（对应 TS 的 GlobalConfig.LTPfunction）
	toolFunctions = map[string]ToolHandler{}
)

// ==== 广播 ====

var (
	// PushMessageFunc 结构化广播函数，由装配层注入为统一集线器的 BroadcastMessage
	PushMessageFunc func(msgType string, data any)
)

func init() {
	PushMessageFunc = func(msgType string, data any) {
		logger.Error(lunarCoreModule, "PushMessageFunc 未初始化, 消息类型: %s", msgType)
	}
}

// ==== 智能体 3D 位置 ====

var (
	// agentPosition 缓存的智能体最新 3D 位置
	agentPosition AgentPositionData
	// agentPositionMu 保护 agentPosition
	agentPositionMu sync.RWMutex
)

// ==== 动画列表缓存 ====

var (
	// animMu 保护动画列表缓存
	animMu sync.RWMutex
	// animActions 当前可用动作
	animActions []ActionDefinition
	// animUpdatedAt 最近更新时间（毫秒）
	animUpdatedAt int64
)

// ==== LTPX 工具链缓存 ====

var (
	// ltpMu 保护聚合工具缓存
	ltpMu sync.RWMutex
	// ltpSubTools 可调用应用清单
	ltpSubTools []LTPXRemoteToolRef
	// ltpLoggedNames 上次打日志的应用名清单
	ltpLoggedNames []string
)

// ==== 立面与数据导出注册 ====

var (
	// dumpMu 保护可导出角色注册表
	dumpMu sync.RWMutex
	// dumpRoles 可导出角色注册表
	dumpRoles = map[string]DumpRoleEntry{}
)

// ==== 对话缓存 ====

var (
	// cacheMu 保护近期记忆缓存
	cacheMu sync.Mutex
	// cachedMessages 缓存内存副本（与 messages.json 一一对应）
	cachedMessages []CachedMessage
	// lastSnapshot 上次成功写盘的序列化快照
	lastSnapshot string
	// animatedHistoryCache 历史动图判定缓存
	animatedHistoryCache = map[string]bool{}
)

// ==== 计划表 ====

var (
	// scheduleMu 保护计划表内存缓存
	scheduleMu sync.Mutex
	// scheduleCache 计划表内存缓存
	scheduleCache []ScheduleItem
)

// ==== 常量 ====

const (
	// engineCommandSource 引擎命令来源标识
	engineCommandSource = "lunar-agent"

	// conversationCacheFile 对话缓存文件（相对 LocalDir）
	conversationCacheFile = "database/messages.json"
	// maxCacheMessages 缓存条数上限
	maxCacheMessages = 48

	// imageWindowSize 对话者上下文中允许保留的真实图片数量
	imageWindowSize = 20
	// maxToolRounds 单次应答允许的工具调用轮次上限
	maxToolRounds = 5

	// visionReadPath 视觉读取端点路径
	visionReadPath = "/file/miniature/"
	// fileReadPath 前端可查看原图的读取端点
	fileReadPath = "/file/read/"

	// asrFallbackText 语音转写失败时的兜底置换文本
	asrFallbackText = "月华听不懂这段语音呢"

	// channelOrigin 渠道消息来源（资源目录的 origin 语义）
	channelOrigin = "channel"
	// clientUploadOrigin 前端直传来源
	clientUploadOrigin = "client-upload"

	// urlExtractLimit 一次未读转换内现场提炼陌生链接的次数上限
	urlExtractLimit = 2
	// unavailableLinkText 链接无法登记时的中性占位（绝不回显地址与 BV 号）
	unavailableLinkText = "（这条链接暂时无法归档）"
	// urlFocusRemainRunes 提炼指令里保留的诉求文字长度（去掉链接后）
	urlFocusRemainRunes = 160

	// scheduleFilePath 计划表存储路径
	scheduleFilePath = "database/schedule.json"
	// scheduleTriggerPrefix 计划到期触发消息统一前缀
	scheduleTriggerPrefix = "[计划提醒]"
	// scheduleSource 计划到期触发消息的来源标记：只进模型上下文，不写入前端可见的对话缓存
	scheduleSource = "schedule"

	// tickModule Tick 调度日志模块名
	tickModule = "TickLoop"
	// tickStateFile 调度器状态文件（相对 LocalDir）
	tickStateFile = "database/tick_state.json"
	// tickPersistEveryN 空闲计数每 N 个 Tick 落盘一次（触发/释放时立即落盘）
	tickPersistEveryN = 10

	// 各体系启动时的空闲进度重置值（阈值为 life.IdleThreshold，默认 600）：
	//   - 对话结束 → 600 剩余 = 10 分钟空闲后进入生活；
	//   - 观影结束 → 600-300 = 5 分钟空闲后进入生活；
	//   - 生活（休息）结束 → 600-(-300) = 15 分钟空闲后再次进入生活。
	dialogueIdleReset = 0
	viewingIdleReset  = 300
	lifeIdleReset     = -300
)

// tickSchedulerInstance 全局 Tick 调度器（Start 时创建）
var tickSchedulerInstance *tickScheduler

// viewingPendingHook 观影待办探测；测试可替换，生产默认走 BilibiliViewer。
var viewingPendingHook = BilibiliViewer.HasPendingWork

// readPageHook 指定网页的现场读取入口；测试可替换，生产默认走 WebSearch.ReadURL。
var readPageHook = WebSearch.ReadURL

// bilibiliDigestHook B 站视频的速览入口；测试可替换，生产默认走 BilibiliViewer.QuickDigest。
var bilibiliDigestHook = BilibiliViewer.QuickDigest

// bilibiliImpressionHook B 站 URL 直驱印象（规范三：跳过选片/动态/切片，但会聚合评论与弹幕）
var bilibiliImpressionHook = BilibiliViewer.DirectImpression

// notifyIdleHook 空闲 Tick 通知 QQ 桥接推进队列；测试可替换。
var notifyIdleHook = DialogueBridging.NotifyAgentIdle
