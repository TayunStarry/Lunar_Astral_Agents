package thinking

import (
	"context"
	"sync"
	"sync/atomic"

	"LunarSubsystem/LunarHarness/agents/DialogueBridging"
	"LunarSubsystem/LunarHarness/chain/search"
	"LunarSubsystem/LunarHarness/chain/viewer"
	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// ==== 日志标签 ====

const lunarCoreModule = "Dialogue"

// lunarLoopRetries 模型输出陷入重复时，单次调用的额外重试次数。
// 对话者的正式回答对完整性要求高，命中后先重试一次；仍失败交给思维链的失败处理。
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

	// fastAckSent 本轮思考链是否已经发过高耗时任务的快速应答
	fastAckSent bool
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
)

// ==== 工具注册表 ====

// dialogueModelRun 对话者单轮模型调用入口；测试可替换，生产默认走 ModelBuilder.run。
var dialogueModelRun ModelRunFunc = defaultModelRun

// 工具处理函数注册表已迁移到 protocol/toolreg：声明在智能体，执行器由本链装配时注册。

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

// ==== LTPX 工具链缓存 ====

var (
	// ltpMu 保护聚合工具缓存
	ltpMu sync.RWMutex
	// ltpSubTools 可调用应用清单
	ltpSubTools []LTPXRemoteToolRef
	// ltpLoggedNames 上次打日志的应用名清单
	ltpLoggedNames []string
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

// ==== 常量 ====

const (
	// conversationCacheFile 对话缓存文件（相对 LocalDir）
	conversationCacheFile = "database/messages.json"
	// maxCacheMessages 缓存条数上限
	maxCacheMessages = 48

	// affectionRoundWindow 情感认知评估时每个角色取用的最近轮数
	affectionRoundWindow = 5
	// imageWindowSize 对话者上下文中允许保留的真实图片数量
	imageWindowSize = 20
	// maxToolRounds 单次应答允许的工具调用轮次上限
	maxToolRounds = 5
	// dialogueMaxRepeats 同一响应签名（正文 + 工具名 + 参数）允许重复出现的次数上限。
	// 手写循环没有跨轮重复检测，接入基座后显式启用，避免模型反复发起同一调用耗空轮次。
	dialogueMaxRepeats = 3
	// dialogueAgentID 对话者在智能修复侧的故障归属 ID（与 agentrepair 注册的 ID 一致）
	dialogueAgentID = "dialogue"

	// visionReadPath 视觉读取端点路径
	visionReadPath = "/file/miniature/"
	// fileReadPath 前端可查看原图的读取端点
	fileReadPath = "/file/read/"

	// channelOrigin 渠道消息来源（资源目录的 origin 语义）
	channelOrigin = "channel"
	// clientUploadOrigin 前端直传来源
	clientUploadOrigin = "client-upload"

	// unavailableLinkText 链接无法登记时的中性占位（绝不回显地址与 BV 号）
	unavailableLinkText = "（这条链接暂时无法归档）"

	// scheduleTriggerPrefix 计划到期触发消息统一前缀
	scheduleTriggerPrefix = "[计划提醒]"
	// scheduleSource 计划到期触发消息的来源标记：只进模型上下文，不写入前端可见的对话缓存
	scheduleSource = "schedule"

	// tickModule Tick 调度日志模块名
	tickModule = "TickLoop"
	// tickStateFile 调度器状态文件（相对 LocalDir）
	tickStateFile = "database/life/tick_state.json"
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
var viewingPendingHook = viewer.HasPendingWork

// readPageHook 指定网页的现场读取入口；测试可替换，生产默认走搜索链的 ReadURL。
var readPageHook = search.ReadURL

// bilibiliImpressionHook B 站 URL 直驱印象（规范三：跳过选片/动态/切片，但会聚合评论与弹幕）
var bilibiliImpressionHook = viewer.DirectImpression

// notifyIdleHook 空闲 Tick 通知 QQ 桥接推进队列；测试可替换。
var notifyIdleHook = DialogueBridging.NotifyAgentIdle

// ==== 快速应答 ====

// fastAckPushHook 快速应答推送入口；测试可替换，生产默认写入上下文缓存。
var fastAckPushHook = pushAssistantContext

// assistantTextPushHook 模型在工具调用轮次中产出的正文推送入口；
// 测试可替换，生产默认写入上下文缓存。
var assistantTextPushHook = pushAssistantContext

// synthesizeSpeech 语音合成入口（前端不在线时返回空串）；测试可替换。
var synthesizeSpeech = synthesizeSpeechImpl

// fastAckOpenings 快速应答的开头候选（空串表示本次没有开头，直接说正文）
var fastAckOpenings = []string{
	"", "", "", // 有时没有开头，直接说正文
	"收到", "收到收到", "好嘞", "好嘞好嘞", "交给我吧", "这个交给我", "没问题",
	"没问题哈", "这个我来", "我来我来", "嗯嗯", "嗯呢", "好的", "好哒", "好呀",
	"行", "可以呀", "知道啦", "知道知道", "明白", "明白啦", "来啦", "来咯",
	"我看看", "这就安排", "马上安排", "安排上了", "OK", "OK啦", "这个简单",
	"放心交给我", "包在我身上", "我来接手", "我这边来", "好说好说", "哦哦",
	"嗯", "诶好", "可以可以", "我这就弄",
}

// fastAckEndings 快速应答的结尾候选（空串表示本次没有结尾，说完正文就收）
var fastAckEndings = []string{
	"", "", "", // 有时没有结尾，说完正文就收
	"稍等我一下", "稍等哈", "稍等片刻", "稍等哦", "等我一下下", "等我一会儿",
	"等我下哈", "等我下下", "等我好消息", "一会儿就好", "一会儿见", "马上回来",
	"马上就来", "马上搞定", "马上就好", "马上给你结果", "很快的", "很快哦",
	"很快搞定", "很快回来", "很快就有结果", "这就去办", "这就给你办",
	"不耽误你太久", "给我点时间", "我尽快", "别急哈", "耐心等我一小会儿",
	"这边很快就完事", "一下下就好", "马上哈", "稍等马上来",
}

// fastAckLines 复合工具的快速应答正文候选。
//
// 触发粒度是工具名而非分支参数：只要本轮请求了对应复合工具，就从该工具
// 的候选池随机取一条；每个工具至少两条，文案集中在这里便于后续调整。
var fastAckLines = map[string][]string{
	// dispatch_technique 技法创作（绘制 / 音乐 / 截图 / 文件操作共用一套口吻）
	techniqueToolName: {
		"这就开工", "我这就动手", "马上给你安排", "这就安排上", "我来接手",
		"这就去办", "包在我身上", "给我点时间，我这就弄", "马上开始", "这就处理",
	},
	// network_explore 联网探索（检索 / 观影 / 论坛 / 通讯共用一套口吻）
	networkExploreToolName: {
		"这就去网上看看", "我先查查看", "我这就去跑一趟", "稍等，我去了解一下",
		"这就去翻一翻", "我去打听打听", "马上帮你查", "让我去看看情况",
		"这就去联网瞧瞧", "我去搜一圈",
	},
	// use_the_program 扩展应用（打开 / 调用应用共用一套口吻）
	crystalToolName: {
		"这就帮你打开", "我这就去调它", "稍等，我去启动一下", "马上把程序调起来",
		"这就把应用打开", "我去运行看看", "让我先启动一下", "这就连接过去",
		"马上给你打开", "我这就安排应用",
	},
}
