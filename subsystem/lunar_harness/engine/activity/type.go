package activity

// type.go 月华「刚刚在做的事情」状态类型定义。

// State 活动状态持久化结构。
type State struct {
	Version   int    `json:"version"`
	Text      string `json:"text"`
	UpdatedAt int64  `json:"updated_at"`
}
