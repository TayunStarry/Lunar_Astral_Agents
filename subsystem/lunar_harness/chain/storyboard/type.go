package storyboard

// type.go 演绎链的全部类型定义：提交请求、分镜规划、逐镜结果、任务视图。

import (
	"context"
	"encoding/json"
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
	// ReviewRequired 为 true 时，分镜规划完成后进入人工审查；
	// 生活系统自动触发与月华命令触发的演绎跳过审查，直接开始分镜合成。
	ReviewRequired bool
	// OutputFolder 指定图片、配音与幻灯片共用的标题名；为空时按分镜标题自动生成。
	OutputFolder string
}

// ReviewShot 审查稿里的一镜：用户改写、删除、新增后由确认接口回传。
type ReviewShot struct {
	// Index 原稿镜头序号；新增镜头填 0
	Index int
	Act   string
	Scene string
	// Narration 本镜叙事
	Narration string
	// Original 本镜对应的故事原文（配音朗读用它；为空时继承原稿同序号镜头）
	Original string
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
	// Original 本镜对应的故事原文（配音朗读用它，而不是分镜叙事）
	Original string `json:"original,omitempty"`
	Prompt   string `json:"prompt"`
	Ratio    string `json:"ratio,omitempty"`
	// Status pending / planned / drawing / painted / failed
	Status string `json:"status"`
	// Handle 产出的图片资源句柄
	Handle string `json:"handle,omitempty"`
	// File 图片相对幻灯片文档目录的文件名（合成幻灯片时填写）
	File string `json:"file,omitempty"`
	// VoiceStatus 配音状态 pending / voicing / ready / failed
	VoiceStatus string `json:"voice_status,omitempty"`
	// VoiceFile 配音 WAV 相对 LocalDir 的路径（落盘后填写）
	VoiceFile string `json:"voice_file,omitempty"`
	// VoiceHandle 配音 WAV 的资源句柄
	VoiceHandle string `json:"voice_handle,omitempty"`
	// VoiceDurationMs 配音时长（毫秒）：幻灯片展示时长以它为准
	VoiceDurationMs int `json:"voice_duration_ms,omitempty"`
	// VoiceError 配音失败原因
	VoiceError string `json:"voice_error,omitempty"`
	// Error 画面合成失败原因
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
	// V 推送协议版本：2 表示增量推送。缺省（1）时 Shots 即全量镜头表。
	V int `json:"v,omitempty"`
	// ShotPatch 本次推送中发生变化的镜头（增量推送使用；为空表示镜头表未变）
	ShotPatch []ShotView `json:"shot_patch,omitempty"`
	// ShotCount 任务当前镜头总数：前端据此裁剪本地多出的镜头
	ShotCount int `json:"shot_count,omitempty"`
	// DocPath 幻灯片 HTML 相对 LocalDir 的路径（documents/lens/<标题>.html）
	DocPath string `json:"doc_path,omitempty"`
	// DocHandle 幻灯片 HTML 的资源句柄
	DocHandle string     `json:"doc_handle,omitempty"`
	Shots     []ShotView `json:"shots,omitempty"`
	Handles   []string   `json:"handles,omitempty"`
	Error     string     `json:"error,omitempty"`
	// Seq 同一 epoch 内单调递增的快照序号：前端据此丢弃迟到/乱序的推送与查询结果。
	Seq uint64 `json:"seq,omitempty"`
	// Epoch 进程启动标识：进程重启后序号从头开始，前端见到新 epoch 即重置序号。
	Epoch     string `json:"epoch,omitempty"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
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
	// OutputFolder 图片输出子文件夹（multimedia/generated/<OutputFolder>）
	OutputFolder string
	// Silent 静默产出：只登记资源并返回句柄，不推送前端、不写记忆、不发 QQ
	Silent bool
}

// PaintResult 图片绘制智能体的返回。
type PaintResult struct {
	Summary string
	Handles []string
}

// VoiceRequest 交给语音合成能力的结构化需求：朗读文本一律是本镜故事原文。
type VoiceRequest struct {
	// Text 要朗读的原文
	Text string
	// Task 任务说明（供实况与日志辨识）
	Task string
}

// VoiceResult 语音合成的返回：WAV 字节，时长由链内按 WAV 头计算。
type VoiceResult struct {
	// Audio WAV 音频字节
	Audio []byte
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
	// Original 本镜对应的故事原文（配音朗读文本）
	Original string `json:"original"`
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
	// sentThought 已推送给前端的过程文本字节偏移（始终落在 rune 起始处）：
	// 增量推送只补发其后的内容，避免每条消息重复携带累计全文。
	sentThought int
	// thoughtReset 待推送的「清空过程文本」标记（推送后立即清空）
	thoughtReset bool
	// dirtyShots 待推送的镜头下标（本次推送只带这些镜头）
	dirtyShots map[int]struct{}
	// dirtyAll 下一次推送必须携带完整镜头表
	dirtyAll bool
	// pushPending 存在尚未推送的变更
	pushPending bool
	// pushTimer 合并推送的定时器；非 nil 表示已安排
	pushTimer *time.Timer
	// research 资料准备进度
	research researchState
	// plan 分镜规划结果
	plan storyboardPlan
	// outputFolder 本次演绎的图片输出子文件夹（标题归一化后生成，图片与文档共用）
	outputFolder string
	// shots 逐镜执行视图（与 plan.Shots 顺序一致）
	shots []ShotView
	// nextShot 下一个待绘制镜头下标
	nextShot int
	// startedAt 任务实际推进的起点（暂停 / 审查等待时间不计入）
	startedAt time.Time
	// lastBeat 最近一次状态更新时刻：卡死判定只看它，不看运行总时长
	lastBeat time.Time
	// budgetAt 当前运行预算到期时刻：到期先做状态校验，仍在推进则顺延
	budgetAt time.Time
	// nextWarn 下一次长耗时提醒时刻
	nextWarn time.Time
	// restedAt 进入暂停/审查的起始时刻；为零表示任务正在推进
	restedAt time.Time
	// paused 用户暂停标记
	paused bool
	// canceled 用户终止标记
	canceled bool
	// held 重启恢复后等待放行：现场已还原但系统尚未完成初始化，
	// 不占用 Tick、不发起任何模型调用，避免开机即抢占资源导致界面卡死。
	held bool
	// cancel 取消当前步骤的上下文；绘制阶段据此立刻停手
	cancel context.CancelFunc
}

// persistedTasks 任务落盘结构。
//
// Tasks 保留为原始 JSON：旧版本落的是纯 TaskView，新版本落的是 persistedTask，
// 恢复时按能否解出 View.TaskID 区分，旧数据按「中断」处理。
type persistedTasks struct {
	Version int               `json:"version"`
	Tasks   []json.RawMessage `json:"tasks"`
}

// persistedTask 单个任务的完整现场：视图 + 续跑所需的内部状态。
type persistedTask struct {
	View         TaskView          `json:"view"`
	Request      SubmitRequest     `json:"request"`
	Stage        string            `json:"stage"`
	Story        string            `json:"story,omitempty"`
	Thought      string            `json:"thought,omitempty"`
	Research     persistedResearch `json:"research"`
	Plan         persistedPlan     `json:"plan"`
	OutputFolder string            `json:"output_folder,omitempty"`
	Shots        []ShotView        `json:"shots,omitempty"`
	NextShot     int               `json:"next_shot"`
	Paused       bool              `json:"paused"`
	Canceled     bool              `json:"canceled"`
}

// persistedResearch 资料准备进度的落盘形式。
type persistedResearch struct {
	Planned bool     `json:"planned"`
	Queries []string `json:"queries,omitempty"`
	Index   int      `json:"index"`
	Notes   []string `json:"notes,omitempty"`
}

// persistedPlan 分镜规划的落盘形式。
type persistedPlan struct {
	Title string          `json:"title,omitempty"`
	Style string          `json:"style,omitempty"`
	Shots []persistedShot `json:"shots,omitempty"`
}

// persistedShot 规划结果中单镜的落盘形式。
type persistedShot struct {
	Index      int    `json:"index"`
	Act        string `json:"act,omitempty"`
	Scene      string `json:"scene"`
	Narration  string `json:"narration"`
	Original   string `json:"original,omitempty"`
	Continuity string `json:"continuity,omitempty"`
	Prompt     string `json:"prompt"`
	Ratio      string `json:"ratio,omitempty"`
}
