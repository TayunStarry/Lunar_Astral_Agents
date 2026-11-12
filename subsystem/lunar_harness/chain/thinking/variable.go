package thinking

import (
	"sync"
	"sync/atomic"
	"time"

	"LunarSubsystem/LunarHarness/agents/Dialogue"
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

// 远程图片回源：超时与大小上限（本机文件与同源地址不受此限制）
const (
	// remoteImageTimeout 远程图片回源超时
	remoteImageTimeout = 30 * time.Second
	// maxRemoteImageBytes 远程图片回源上限（32 MiB）：
	// 网页原图与聊天图片通常远小于此值，留足余量同时避免无上限读入内存。
	maxRemoteImageBytes = 32 * 1024 * 1024
	// maxPainterRefDocBytes 从文档片段解析参考图时最多读取的字节数
	maxPainterRefDocBytes = 256 * 1024
	// maxPainterReferences 单次绘图任务最多携带的参考图数量
	maxPainterReferences = 4
)

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
	// PushMessageFunc 结构化广播函数，由装配层注入为统一集线器的 BroadcastMessage。
	// 未注入时打一条明确错误，避免推送静默丢失让对话表现为「模型说了但前端没收到」。
	PushMessageFunc func(msgType string, data any) = func(msgType string, data any) {
		logger.Error(lunarCoreModule, "PushMessageFunc 未初始化, 消息类型: %s", msgType)
	}
)

// ==== 对话缓存 ====

var (
	// cacheMu 保护近期记忆缓存
	cacheMu sync.Mutex
	// cachedMessages 缓存内存副本（与 messages.json 一一对应）
	cachedMessages []CachedMessage
	// lastSnapshot 上次成功写盘的序列化快照
	lastSnapshot string
)

// ==== 心愿链路 ====

// ==== 常量 ====

const (
	// conversationCacheFile 对话缓存文件（相对 LocalDir）
	conversationCacheFile = "database/messages.json"
	// maxCacheMessages 缓存条数上限
	maxCacheMessages = 48

	// affectionRoundWindow 情感认知评估时每个角色取用的最近轮数
	affectionRoundWindow = 5
	// maxToolRounds 单次应答允许的工具调用轮次上限
	maxToolRounds = 5
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

	// missingImageHandleText 远期记忆里没有句柄的图片的兜底：宁可留一条说明，也不静默丢弃
	missingImageHandleText = "（一张未能登记句柄的图片）"

	// wishTriggerPrefix 愿望到期触发消息统一前缀
	wishTriggerPrefix = "[愿望提醒]"
	// wishSource 愿望到期触发消息的来源标记：只进模型上下文，不写入前端可见的对话缓存
	wishSource = "wish"

	// tickModule Tick 调度日志模块名
	tickModule = "TickLoop"
	// tickStateFile 调度器状态文件（相对 LocalDir）
	tickStateFile = "database/life/tick_state.json"
	// tickPersistEveryN 空闲计数每 N 个 Tick 落盘一次（触发/释放时立即落盘）
	tickPersistEveryN = 10

	// 各体系启动时的空闲进度重置值（阈值为 life.IdleThreshold，默认 600）：
	//   - 对话结束 → 600 剩余 = 10 分钟空闲后进入生活；
	//   - 生活（休息）结束 → 600-(-300) = 15 分钟空闲后再次进入生活。
	dialogueIdleReset = 0
	lifeIdleReset     = -300
)

// tickSchedulerInstance 全局 Tick 调度器（Start 时创建）
var tickSchedulerInstance *tickScheduler

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

// 快速应答三段的舍弃概率：先各自独立抽签决定这一段是否缺席，再从对应候选池取词。
// 首尾更容易被舍弃（开口一句「好嘞」和结尾一句「稍等」都可有可无），正文是语义主干，
// 保留概率最高；三段同时缺席时由正文兜底，保证不会推出空消息。
var (
	fastAckOpeningDrop = 0.45
	fastAckActionDrop  = 0.15
	fastAckEndingDrop  = 0.55
)

// fastAckOpenings 快速应答的开场候选：短促的应答词，像随口应一声
var fastAckOpenings = []string{
	"收到", "行", "成", "好嘞", "可以", "嗯嗯", "哦哦", "诶", "啊这", "得嘞",
	"明白", "知道啦", "没问题", "交给我", "我来我来", "我看看", "好说", "在的",
	"OK", "可以呀", "嗯", "安排", "这我熟", "小事", "马上办",
}

// fastAckEndings 快速应答的收尾候选：口语化的安抚，别把人晾着
var fastAckEndings = []string{
	"稍等哈", "等我一会儿", "别急，很快的", "马上就有结果", "一会儿就好",
	"等我下", "马上回来", "给我一小会儿", "立马就好", "等我消息",
	"这就好了", "很快搞定", "一会儿给你信儿", "稍等一小会儿", "马上哈",
	"别急哈", "再等我一下", "很快就回",
}

// fastAckLines 复合工具的快速应答正文候选。
//
// 触发粒度是工具名而非分支参数：只要本轮请求了对应复合工具，就从该工具
// 的候选池随机取一条；每个工具至少两条，文案集中在这里便于后续调整。
var fastAckLines = map[string][]string{
	// creative_work 创作聚合（故事 / 文件 / 演绎 / 绘画 / 音乐 / 截图共用一套口吻）
	Dialogue.ToolCreativeWork: {
		"这就动手", "我这就弄", "给我点时间搞一下", "我上手试试", "这就给你整",
		"先弄起来", "我这就开工", "我摸摸看", "这就开整", "我这就去做",
	},
	// network_explore 联网探索（检索 / 观影 / 论坛 / 通讯共用一套口吻）
	Dialogue.ToolNetworkExplore: {
		"我去网上翻翻", "让我查查", "我先去搜一下", "去打听打听", "我这就跑一趟",
		"上网瞅一圈", "让我看看情况", "我这就去翻一翻", "去查点资料", "我上网摸摸",
	},
	// use_the_program 扩展应用（打开 / 调用应用共用一套口吻）
	Dialogue.ToolUseTheProgram: {
		"我去把它打开", "这就给你开", "让我调出来", "我这就启动一下", "去跑一下这个程序",
		"我把应用调起来", "我这就去连一下", "先开起来看看", "我去操作一下", "这就给你调出来",
	},
}
