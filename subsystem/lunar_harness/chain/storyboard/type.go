package storyboard

// type.go 演绎链的全部类型定义：提交请求、分镜规划、逐镜结果、任务视图。

import (
	"context"
	"time"
)

// SubmitRequest 一次演绎任务提交。
//
// Text 与 Handle 二选一：Text 直接给故事正文，Handle 指向已有文档资源；
// Title / Style / Ratio 为可选的统一约束。
type SubmitRequest struct {
	// Text 故事正文
	Text string
	// Handle 故事正文所在的资源句柄（如 @D.001）
	Handle string
	// Title 作品标题（空时由分镜规划生成）
	Title string
	// Style 画风约束（可选，空时由绘制智能体按自身机制决定）
	Style string
	// Ratio 统一画幅（空时默认 16:9）
	Ratio string
}

// ReviewShot 审查稿里的一镜：用户改写、删除、新增后由确认接口回传。
type ReviewShot struct {
	// Index 原稿镜头序号；新增镜头填 0
	Index int
	Act   string
	Scene string
	// Narration 本镜叙事
	Narration string
	// Prompt 画面描述（为空时按叙事重新拼装）
	Prompt string
	// Ratio 单镜画幅（为空时用任务默认画幅）
	Ratio string
}

// ShotView 分镜的执行结果视图。
type ShotView struct {
	Index     int    `json:"index"`
	Act       string `json:"act,omitempty"`
	Scene     string `json:"scene"`
	Narration string `json:"narration"`
	Prompt    string `json:"prompt"`
	Ratio     string `json:"ratio,omitempty"`
	// Status pending / planned / drawing / painted / failed
	Status string `json:"status"`
	// Handle 产出的图片资源句柄
	Handle string `json:"handle,omitempty"`
	// File 图片相对分镜文档目录的文件名（成功迁移后填写）
	File string `json:"file,omitempty"`
	// Error 失败原因
	Error string `json:"error,omitempty"`
}

// TaskView 演绎任务对外状态（提交、查询、推送共用）。
type TaskView struct {
	TaskID   string `json:"task_id"`
	Status   string `json:"status"`
	Stage    string `json:"stage,omitempty"`
	Progress int    `json:"progress"`
	Summary  string `json:"summary,omitempty"`
	Title    string `json:"title,omitempty"`
	// Event 本次推送的增量事件名（accepted / shot_started / shot_completed ...）
	Event string `json:"event,omitempty"`
	// ShotIndex 当前事件对应的镜头序号（0 表示不指向具体镜头）
	ShotIndex int `json:"shot_index,omitempty"`
	// Thought 规划阶段主智能体的累计可读过程文本
	Thought string `json:"thought,omitempty"`
	// Delta 本次推送新增的过程文本（仅推送使用，状态查询不会返回）
	Delta string `json:"delta,omitempty"`
	// ThoughtReset 本次推送要求前端清空过程文本（阶段重跑时使用）
	ThoughtReset bool `json:"thought_reset,omitempty"`
	// DocPath 分镜文档相对 LocalDir 的路径（documents/演绎/.../分镜.md）
	DocPath string `json:"doc_path,omitempty"`
	// DocHandle 分镜文档的资源句柄
	DocHandle string     `json:"doc_handle,omitempty"`
	Shots     []ShotView `json:"shots,omitempty"`
	Handles   []string   `json:"handles,omitempty"`
	Error     string     `json:"error,omitempty"`
	CreatedAt string     `json:"created_at"`
	UpdatedAt string     `json:"updated_at"`
}

// ProgressEvent 分步骤执行中的增量进度。
type ProgressEvent struct {
	Status    string
	Stage     string
	Progress  int
	Summary   string
	Title     string
	Event     string
	ShotIndex int
	DocPath   string
	DocHandle string
	Shots     []ShotView
	Handles   []string
	// ThoughtDelta 本次新增的可读过程文本
	ThoughtDelta string
	// ThoughtReset 要求前端清空已显示的过程文本
	ThoughtReset bool
}

// researchState 资料准备阶段的进度：检索意图、当前检索下标与已获资料。
type researchState struct {
	// planned 是否已经生成检索意图
	planned bool
	// queries 检索意图（每条一次网络搜索）
	queries []string
	// index 下一条待执行的检索
	index int
	// notes 已获检索结果（送入规划提示词）
	notes []string
}

// PaintRequest 交给图片绘制智能体的结构化需求。
type PaintRequest struct {
	Prompt     string
	Ratio      string
	Style      string
	Change     string
	Output     string
	Task       string
	References []string
}

// PaintResult 图片绘制智能体的返回。
type PaintResult struct {
	Summary string
	Handles []string
}

// storyboardPlan 分镜规划结果。
type storyboardPlan struct {
	// Title 作品标题
	Title string
	// Style 用户显式提交的画风约束（为空时由绘制智能体按自身机制决定）
	Style string
	// Shots 分镜列表
	Shots []plannedShot
}

// plannedShot 模型给出的单个分镜。
type plannedShot struct {
	// Index 全局序号（归一化时填写，不要求模型返回）
	Index     int    `json:"-"`
	Act       string `json:"act"`
	Scene     string `json:"scene"`
	Narration string `json:"narration"`
	// Continuity 本镜与上一镜的画面衔接
	Continuity string `json:"continuity"`
	Prompt     string `json:"prompt"`
	Ratio      string `json:"ratio"`
	// Style 用户画风约束，仅作为绘图交接参数，不写入提示词
	Style string `json:"-"`
}

// taskState 任务内部状态（步骤机）。
type taskState struct {
	view TaskView
	req  SubmitRequest
	// stage 内部阶段：queued / planning / painting / writing / done
	stage string
	// story 已读取的故事正文
	story string
	// thought 可读过程文本（资料准备与规划阶段累计）
	thought string
	// delta 待推送的过程文本增量（推送后立即清空）
	delta string
	// thoughtReset 待推送的「清空过程文本」标记（推送后立即清空）
	thoughtReset bool
	// research 资料准备进度
	research researchState
	// plan 分镜规划结果
	plan storyboardPlan
	// shots 逐镜执行视图（与 plan.Shots 顺序一致）
	shots []ShotView
	// nextShot 下一个待绘制镜头下标
	nextShot int
	// previous 上一镜成功产出的图片句柄（作为下一镜参考图）
	previous string
	// deadline 整个任务的截止时间（暂停与审查期间会顺延）
	deadline time.Time
	// restedAt 进入暂停/审查的起始时刻；为零表示任务正在推进
	restedAt time.Time
	// paused 用户暂停标记
	paused bool
	// canceled 用户终止标记
	canceled bool
	// cancel 取消当前步骤的上下文；绘制阶段据此立刻停手
	cancel context.CancelFunc
}

// persistedTasks 任务视图落盘结构。
type persistedTasks struct {
	Version int        `json:"version"`
	Tasks   []TaskView `json:"tasks"`
}
