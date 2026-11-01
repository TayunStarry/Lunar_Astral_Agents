package affection

// type.go 情感认知（对每个用户的好感认知）的类型定义。

// State 情感认知持久化状态。
//
// Anchors 保存两极锚点的标准嵌入向量；Users 按用户名保存好感度与标签。
// 与全局心情完全独立：独立锚点集、独立文件，不共享心情向量空间。
type State struct {
	Version   int                      `json:"version"`
	Model     string                   `json:"model"`
	Dim       int                      `json:"dim"`
	Anchors   map[string][]float32     `json:"anchors"`
	Users     map[string]UserAffection `json:"users"`
	UpdatedAt int64                    `json:"updated_at"`
}

// UserAffection 月华对单个用户的情感认知（好感度与标签）。
type UserAffection struct {
	Score        float64 `json:"score"`                   // 好感度，范围 [-1, 1]
	Label        string  `json:"label"`                   // 好感度对应的情感标签
	Samples      int     `json:"samples"`                 // 累计更新次数
	WindowRounds int     `json:"window_rounds,omitempty"` // 最近一次评估用到的对话轮数
	LastKey      string  `json:"last_key,omitempty"`      // 最近一次评估窗口的最新一轮标识，用于避免重复计算
	UpdatedAt    int64   `json:"updated_at"`
}

// Round 一轮「用户发言 + 月华回应」的成对交互。
type Round struct {
	Key       string // 去重标识（用户消息 ID）
	UserName  string // 发言者名
	UserText  string // 用户发言
	ReplyText string // 月华回应
}
