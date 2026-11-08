package storyboard

// variable.go 演绎链的常量、可注入钩子与进程内任务表。

import (
	"context"
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
	statusRevising    = "revising"
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
	stageRevising    = "revising"
	stagePainting    = "painting"
	stageWriting     = "writing"
	stageDone        = "done"

	// 分镜执行状态
	shotPending = "pending"
	shotPlanned = "planned"
	shotDrawing = "drawing"
	shotPainted = "painted"
	shotFailed  = "failed"

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
	eventRevising        = "revising"
	eventShotStarted     = "shot_started"
	eventShotCompleted   = "shot_completed"
	eventShotFailed      = "shot_failed"
	eventWriting         = "writing"
	eventPaused          = "paused"
	eventResumed         = "resumed"
	eventCanceled        = "canceled"
	eventCompleted       = "completed"
	eventFailed          = "failed"
	eventInterrupted     = "interrupted"

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
	// maxPaintAttempts 单镜最多绘制尝试次数（含首次）
	maxPaintAttempts = 2

	// defaultRatio 统一画幅
	defaultRatio = "16:9"

	// docRootRel 分镜文档在 LocalDir 下的物理根目录
	docRootRel = "documents/演绎"
	// docDisplayRoot 分镜文档相对 /document 的目录前缀
	docDisplayRoot = "演绎"

	// storyboardTasksFile 任务视图落盘路径：用于进程重启后标记中断，不做续跑。
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
	// TaskEventHook 任务状态推送，由装配层注入 WebSocket 广播。
	TaskEventHook func(view TaskView)
	// ArtworkMoveHook 图片迁移后的运行时画作路径同步，由装配层注入绘制智能体。
	ArtworkMoveHook func(oldRel, newRel string)

	// taskMu 保护任务表
	taskMu sync.Mutex
	// tasks 任务 ID → 状态
	tasks = make(map[string]*taskState)
	// taskOrder 任务创建顺序
	taskOrder []string
	// taskSeq 任务序号
	taskSeq uint64
)

var (
	// researchPlanTimeout 生成检索意图的模型调用超时
	researchPlanTimeout = 90 * time.Second
	// searchTimeout 单条网络搜索超时
	searchTimeout = 4 * time.Minute
	// planTimeout 分镜规划单次模型调用超时
	planTimeout = 3 * time.Minute
	// reviseTimeout 审查后修订分镜的单次模型调用超时
	reviseTimeout = 3 * time.Minute
	// shotPaintTimeout 单镜绘制超时
	shotPaintTimeout = 8 * time.Minute
	// runTimeout 整个演绎任务超时
	runTimeout = 90 * time.Minute
	// retryBackoff 单镜重试间隔
	retryBackoff = 2 * time.Second
)

// stagePaintTimeout 单镜原子步骤的总超时：覆盖全部重试与退避。
func stagePaintTimeout() time.Duration {
	return time.Duration(maxPaintAttempts)*shotPaintTimeout +
		time.Duration(maxPaintAttempts)*retryBackoff + time.Minute
}

// stagePlanTimeout 分镜规划原子步骤的总超时。
func stagePlanTimeout() time.Duration {
	return planTimeout + 30*time.Second
}

// stageResearchTimeout 资料准备原子步骤的总超时。
func stageResearchTimeout() time.Duration {
	return researchPlanTimeout + searchTimeout + 30*time.Second
}

// stageReviseTimeout 审查后修订原子步骤的总超时。
func stageReviseTimeout() time.Duration {
	return reviseTimeout + 30*time.Second
}
