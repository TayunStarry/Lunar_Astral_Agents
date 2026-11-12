package storyboard

// variable.go 演绎链的常量、可注入钩子与进程内任务表。

import (
	"context"
	"strconv"
	"sync"
	"time"
)

const (
	// Module 日志与故障归属名
	Module = "演绎链"

	// 任务对外状态
	statusQueued      = "queued"
	statusResearching = "researching"
	statusPlanning    = "planning"
	statusReviewing   = "reviewing"
	statusPainting    = "painting"
	statusWriting     = "writing"
	statusPaused      = "paused"
	statusCanceled    = "canceled"
	statusCompleted   = "completed"
	statusFailed      = "failed"

	// 任务内部阶段（步骤机）
	stageQueued      = "queued"
	stageResearching = "researching"
	stagePlanning    = "planning"
	stageReviewing   = "reviewing"
	stagePainting    = "painting"
	stageWriting     = "writing"
	stageDone        = "done"

	// 分镜执行状态
	shotPending = "pending"
	shotPlanned = "planned"
	shotDrawing = "drawing"
	shotPainted = "painted"
	shotFailed  = "failed"

	// 配音执行状态
	voicePending = "pending"
	voiceRunning = "voicing"
	voiceReady   = "ready"
	voiceFailed  = "failed"

	// 增量事件名（前端据此逐条追加渲染）
	eventAccepted          = "accepted"
	eventResearchStarted   = "research_started"
	eventResearchCompleted = "research_completed"
	// eventThinking 资料准备与规划阶段的可读推论文本增量
	eventThinking        = "thinking"
	eventPlanning        = "planning_started"
	eventShotPlanned     = "shot_planned"
	eventPlanCompleted   = "plan_completed"
	eventReviewReady     = "review_ready"
	eventReviewConfirmed = "review_confirmed"
	eventShotStarted     = "shot_started"
	eventShotPainted     = "shot_painted"
	eventVoiceStarted    = "voice_started"
	eventShotCompleted   = "shot_completed"
	eventShotFailed      = "shot_failed"
	eventWriting         = "writing"
	// eventPaused 用户暂停
	eventPaused = "paused"
	// eventResumed 用户继续
	eventResumed = "resumed"
	// eventLongRunning 长耗时提醒：任务仍在推进，预算已顺延（不会失败）
	eventLongRunning = "long_running"
	eventCanceled    = "canceled"
	eventCompleted   = "completed"
	eventFailed      = "failed"
	eventInterrupted = "interrupted"

	// maxTasks 进程内保留的任务数
	maxTasks = 60
	// maxTaskList 单次列表查询上限
	maxTaskList = 20

	// maxStoryRunes 故事正文硬上限（超出直接拒绝，不截断）
	maxStoryRunes = 4000
	// maxReviewShots 审查稿允许的最大镜头数
	maxReviewShots = 200
	// maxResearchQueries 单个任务最多执行的检索条数
	maxResearchQueries = 4
	// maxResearchNoteRunes 单条检索结果送入提示词的上限
	maxResearchNoteRunes = 1200
	// maxPaintPromptRunes 单镜画面提示词上限
	maxPaintPromptRunes = 1600
	// maxErrorRunes 失败原因上限
	maxErrorRunes = 300
	// maxThoughtBytes 过程文本保留上限（按 UTF-8 字节）：超出后只保留尾部，
	// 防止超长推理文本把任务视图、落盘文件与查询响应撑爆。
	maxThoughtBytes = 48000
	// maxPaintAttempts 单镜最多绘制尝试次数（含首次）
	maxPaintAttempts = 2
	// maxVoiceAttempts 单镜最多配音尝试次数（含首次）
	maxVoiceAttempts = 2
	// maxVoiceRunes 单镜朗读原文上限
	maxVoiceRunes = 1200

	// defaultRatio 统一画幅
	defaultRatio = "16:9"

	// docRootRel 幻灯片文档在 LocalDir 下的物理根目录
	docRootRel = "documents/lens"
	// docDisplayRoot 幻灯片文档相对 /document 的目录前缀
	docDisplayRoot = "lens"
	// slideFileExt 演绎最终产物：documents/lens/<标题>.html
	slideFileExt = ".html"
	// imageOutputRoot 分镜图片的产出根目录
	imageOutputRoot = "multimedia/generated"
	// voiceOutputRoot 分镜配音的产出根目录：音频自成内容树，不混进 multimedia
	voiceOutputRoot = "audios/generated"

	// fallbackSlideMs 无配音时单镜幻灯片的最小展示时长
	fallbackSlideMs = 3200
	// maxFallbackSlideMs 无配音时单镜幻灯片的最大展示时长
	maxFallbackSlideMs = 30000
	// perRuneSpeechMs 无配音时按字数估算朗读时长（毫秒/字）
	perRuneSpeechMs = 240

	// storyboardTasksFile 任务现场落盘路径：重启后据此恢复未完成任务。
	storyboardTasksFile = "database/storyboard/tasks.json"
	// recoverReason 重启遗留任务的失败原因。
	recoverReason = "系统重启，演绎任务已中断；请重新提交"
)

// WebSearchHook 网络搜索能力，由装配层注入现有搜索链。
// 返回人类可读的检索摘要；失败时返回错误，由资料准备阶段降级处理。
var WebSearchHook func(query string) (string, error)

var (
	// PaintHook 图片绘制能力，由装配层注入现有绘制智能体。
	PaintHook func(ctx context.Context, req PaintRequest) (PaintResult, error)
	// VoiceHook 语音合成能力，由装配层注入统一播报链路。
	// 传入的文本一定是本镜故事原文；返回 WAV 字节。
	VoiceHook func(ctx context.Context, req VoiceRequest) (VoiceResult, error)
	// TaskEventHook 任务状态推送，由装配层注入 WebSocket 广播。
	TaskEventHook func(view TaskView)

	// taskMu 保护任务表
	taskMu sync.Mutex
	// tasks 任务 ID → 状态
	tasks = make(map[string]*taskState)
	// taskOrder 任务创建顺序
	taskOrder []string
	// taskSeq 任务序号
	taskSeq uint64
	// viewSeq 任务视图快照序号：推送与查询共用同一条单调序列，前端据此判定新旧。
	viewSeq uint64
)

// taskEpoch 进程启动标识：进程重启后 viewSeq 归零，前端据此重置本地序号。
var taskEpoch = strconv.FormatInt(time.Now().UnixNano(), 36)

var (
	// researchPlanTimeout 生成检索意图的模型调用超时
	// 资料准备是可降级阶段：判不出来就直接进入分镜规划，不该长时间占住 Tick。
	researchPlanTimeout = 30 * time.Second
	// searchTimeout 单条网络搜索超时
	searchTimeout = 4 * time.Minute
	// planTimeout 分镜规划单次模型调用超时
	planTimeout = 3 * time.Minute
	// shotPaintTimeout 单镜绘制超时
	shotPaintTimeout = 8 * time.Minute
	// shotVoiceTimeout 单镜配音超时
	shotVoiceTimeout = 3 * time.Minute
	// runWarnTimeout 长耗时预警阈值：到达后只提醒，绝不终止
	runWarnTimeout = 60 * time.Minute
	// runWarnInterval 持续运行时的提醒间隔（每次都只提醒）
	runWarnInterval = 30 * time.Minute
	// runBudget 单轮运行预算：到期先做状态校验，仍在推进就自动顺延
	runBudget = 90 * time.Minute
	// runStallTimeout 判定「卡死」的唯一依据：连续这么久没有任何状态更新。
	// 取值必须大于最长的一次合法静默（单镜绘制含重试约 17 分钟）。
	runStallTimeout = 20 * time.Minute
	// stepStallGrace 单步兜底宽限：步骤自身超时后再多等这么久仍无更新才判卡死
	stepStallGrace = 2 * time.Minute
	// queuedStepTimeout 受理步骤（读取故事正文）兜底
	queuedStepTimeout = 2 * time.Minute
	// writingStepTimeout 收尾步骤（迁移图片 + 写文档）兜底
	writingStepTimeout = 10 * time.Minute
	// retryBackoff 单镜重试间隔
	retryBackoff = 2 * time.Second
)

// stagePaintTimeout 单镜原子步骤的总超时：覆盖全部重试与退避。
func stagePaintTimeout() time.Duration {
	return time.Duration(maxPaintAttempts)*shotPaintTimeout +
		time.Duration(maxPaintAttempts)*retryBackoff + time.Minute
}

// stageVoiceTimeout 单镜配音原子步骤的总超时：覆盖全部重试与退避。
func stageVoiceTimeout() time.Duration {
	return time.Duration(maxVoiceAttempts)*shotVoiceTimeout +
		time.Duration(maxVoiceAttempts)*retryBackoff + 30*time.Second
}

// stagePlanTimeout 分镜规划原子步骤的总超时。
func stagePlanTimeout() time.Duration {
	return planTimeout + 30*time.Second
}

// stageResearchTimeout 资料准备原子步骤的总超时。
func stageResearchTimeout() time.Duration {
	return researchPlanTimeout + searchTimeout + 30*time.Second
}
