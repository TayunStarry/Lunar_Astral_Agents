package creation

// plan_type.go 自主创作事项的类型定义。

import "math/rand"

// ItemKind 创作事项类型。
type ItemKind string

const (
	// KindNovel 写小说
	KindNovel ItemKind = "novel"
	// KindResearch 分析研究特定事件
	KindResearch ItemKind = "research"
	// KindProject 编写有趣的 JS / HTML 小项目
	KindProject ItemKind = "project"
	// KindStory 创作小故事
	KindStory ItemKind = "story"
	// KindTidy 整理本地文件夹
	KindTidy ItemKind = "tidy"
	// KindJoke 向随机 QQ 用户或群聊推送小笑话
	KindJoke ItemKind = "joke"
	// KindPaint 自主创作一组图片
	KindPaint ItemKind = "paint"
)

// PlanItem 本轮自主选定的创作事项：类型 + 具体参数 + 可执行指令。
type PlanItem struct {
	// ID 事项唯一标识，用于续做 / 完成对齐
	ID string `json:"id"`
	// Kind 事项类型
	Kind ItemKind `json:"kind"`
	// Title 事项名（如「写一篇科幻悬疑小说」）
	Title string `json:"title"`
	// Params 生成的具体参数（题材 / 篇幅 / 事件 / 目录 / 目标对象等）
	Params map[string]string `json:"params"`
	// Task 交给调度者的可执行要求
	Task string `json:"task"`
	// Activity 完成后写入「我刚刚在干的事情」的文案
	Activity string `json:"activity"`
	// Reason 本次选定依据（心情极性、续做还是换新）
	Reason string `json:"reason,omitempty"`
	// CreatedAt 选定时间
	CreatedAt int64 `json:"created_at"`
}

// planState 创作事项的持久化状态。
type planState struct {
	Version int `json:"version"`
	// Active 当前选中且尚未完成的事项
	Active *PlanItem `json:"active,omitempty"`
	// Parked 被心情切走但未完成的事项（后进先出，心情转正后继续）
	Parked []PlanItem `json:"parked,omitempty"`
	// Recent 最近选中的事项类型（排除窗口，避免马上重抽到同一类）
	Recent []string `json:"recent,omitempty"`
	// RecentTargets 最近选中的具体目标（如绘画题材），避免短时间内重复同一目标
	RecentTargets []string `json:"recent_targets,omitempty"`
	// UpdatedAt 最后写入时间
	UpdatedAt int64 `json:"updated_at"`
}

// MoodPolarity 心情极性。
type MoodPolarity int

const (
	// MoodNegative 偏负面
	MoodNegative MoodPolarity = -1
	// MoodNeutral 极性不明确（按偏正向处理，保守续做未完成事项）
	MoodNeutral MoodPolarity = 0
	// MoodPositive 偏正向
	MoodPositive MoodPolarity = 1
)

// qqTarget 一个可用的 QQ 投递目标（群或好友）。
type qqTarget struct {
	// Group 是否为群聊
	Group bool
	// ID 群号或 QQ 号
	ID int64
	// Name 群名或昵称
	Name string
}

// qqTargets 当前可用的 QQ 投递目标集合；为空表示适配器不可用。
type qqTargets struct {
	Items []qqTarget
}

// itemSpec 预设创作事项的结构化定义：类型 + 参数生成方式。
type itemSpec struct {
	// Kind 事项类型
	Kind ItemKind
	// Label 中文类型名
	Label string
	// NeedsQQ 是否需要 QQ 适配器可用
	NeedsQQ bool
	// Build 从预设参数池随机组合出一个可执行事项；资源不可用时返回 false
	Build func(r *rand.Rand, qq qqTargets) (PlanItem, bool)
}
