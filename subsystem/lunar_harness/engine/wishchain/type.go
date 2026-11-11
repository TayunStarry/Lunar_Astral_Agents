package wishchain

// type.go 心愿链路的类型定义。
//
// 心愿链路把「愿望」「心情」「态度」三套数据收敛到同一个领域状态里，
// 并统一写入单一持久化文件；这里只放类型，不放业务逻辑。

import "time"

// 愿望状态。
const (
	// StatusPending 待实现
	StatusPending = "pending"
	// StatusCompleted 已完成
	StatusCompleted = "completed"
)

// 愿望容量与优先级约束。
const (
	// MaxPending 待实现愿望的最大数量。
	MaxPending = 16
	// MinPriority 最高优先级。
	MinPriority = 0
	// MaxPriority 最低优先级。
	MaxPriority = 15
)

// GraceDuration 到期后的宽限期：宽限期内不判定为逾期。
const GraceDuration = 30 * time.Minute

// NearWindow 临近截止的判定窗口：进入该窗口后心情向期待/开心偏移。
const NearWindow = time.Hour

// Wish 一条愿望。
type Wish struct {
	// ID 愿望唯一标识
	ID string `json:"id"`
	// Content 愿望内容
	Content string `json:"content"`
	// Due 期望达成的时间节点（ISO 文本，可空表示没有明确期限）
	Due string `json:"due,omitempty"`
	// Priority 优先级，0 最高、15 最低
	Priority int `json:"priority"`
	// Status pending / completed
	Status string `json:"status"`
	// CreatedAt 创建时间（Unix 秒）
	CreatedAt int64 `json:"created_at"`
	// UpdatedAt 最后修改时间（Unix 秒）
	UpdatedAt int64 `json:"updated_at"`
	// CompletedAt 完成时间（Unix 秒，未完成时为 0）
	CompletedAt int64 `json:"completed_at,omitempty"`

	// Notified 到期通知是否已发送
	Notified bool `json:"notified,omitempty"`
	// NearSettled 临近截止的心情联动是否已结算
	NearSettled bool `json:"near_settled,omitempty"`
	// CompletedSettled 完成的心情联动是否已结算
	CompletedSettled bool `json:"completed_settled,omitempty"`
	// PostponedCount 被延期的累计次数
	PostponedCount int `json:"postponed_count,omitempty"`
	// PostponedSettled 已结算的延期次数
	PostponedSettled int `json:"postponed_settled,omitempty"`
}

// DueNotice 一条到期提醒。
type DueNotice struct {
	// Wish 到期的愿望
	Wish Wish `json:"wish"`
	// Overdue 是否已超出宽限期
	Overdue bool `json:"overdue"`
	// At 判定时刻
	At time.Time `json:"at"`
}

// MoodChange 一次愿望引起的心情联动。
type MoodChange struct {
	// WishID 关联愿望
	WishID string `json:"wish_id"`
	// Reason postponed / near / completed / overdue
	Reason string `json:"reason"`
	// Label 心情偏移方向
	Label string `json:"label"`
}

// Settlement 一轮愿望结算的结果。
type Settlement struct {
	// Applied 本轮实际施加的心情联动
	Applied []MoodChange `json:"applied,omitempty"`
	// Removed 本轮清除的已完成愿望 ID
	Removed []string `json:"removed,omitempty"`
}

// LinkRecord 观影 / 创作等既有机制接入心愿链路后留下的联动记录。
type LinkRecord struct {
	// Kind 联动类型（观影列表变更 / 观影反馈 / 创作选题 / 创作完成）
	Kind string `json:"kind"`
	// Summary 联动摘要
	Summary string `json:"summary"`
	// At 记录时间（Unix 秒）
	At int64 `json:"at"`
}

// MoodState 心情状态：基础情绪锚点向量、当前多维心情与权重。
type MoodState struct {
	Base      map[string][]float32 `json:"base_vectors"`
	Vector    []float32            `json:"vector"`
	Weights   map[string]float64   `json:"weights"`
	Label     string               `json:"label"`
	UpdatedAt int64                `json:"updated_at"`
}

// MoodSnapshot 供对话运行时消息读取的只读快照。
type MoodSnapshot struct {
	Label   string
	Weights map[string]float64
	Vector  []float32
}

// UserAffection 月华对单个用户的态度积分与情感标签。
type UserAffection struct {
	// Points 态度积分，范围 [-50, 50]，新对话者默认 20
	Points int `json:"points"`
	// Label 情感标签
	Label string `json:"label"`
	// Samples 已执行的态度评估次数
	Samples int `json:"samples"`
	// WindowRounds 本次评估使用的新增轮数
	WindowRounds int `json:"window_rounds,omitempty"`
	// LastKey 最近一轮交互标识，用于去重
	LastKey string `json:"last_key,omitempty"`
	// UpdatedAt 最后更新时间（Unix 秒）
	UpdatedAt int64 `json:"updated_at"`
	// LegacyScore 旧版 [-1, 1] 好感度，仅用于兼容载入；换算为积分后清空
	LegacyScore *float64 `json:"score,omitempty"`
}

// AttitudeState 态度状态：两极锚点与按用户保存的情感认知。
type AttitudeState struct {
	Anchors   map[string][]float32     `json:"anchors"`
	Users     map[string]UserAffection `json:"users"`
	UpdatedAt int64                    `json:"updated_at"`
}

// State 心愿链路的完整持久化状态（单一文件）。
type State struct {
	Version   int           `json:"version"`
	Model     string        `json:"model"`
	Dim       int           `json:"dim"`
	Mood      MoodState     `json:"mood"`
	Attitude  AttitudeState `json:"attitude"`
	Wishes    []Wish        `json:"wishes"`
	Links     []LinkRecord  `json:"links,omitempty"`
	UpdatedAt int64         `json:"updated_at"`
}

// Round 一轮「用户发言 + 月华回应」的成对交互。
type Round struct {
	Key       string
	UserName  string
	UserText  string
	ReplyText string
}
