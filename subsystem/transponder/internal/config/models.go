package config

// GroupInfo 群信息结构体
type GroupInfo struct {
	// 群ID
	GroupID int64 `json:"group_id"`
	// 群名称
	GroupName string `json:"group_name"`
	// 成员数量
	MemberCount int `json:"member_count"`
	// 最大成员数量
	MaxMemberCount int `json:"max_member_count"`
}

// MsgItem 消息项结构体
type MsgItem struct {
	// 消息类型
	Type string `json:"type"`
	// 消息数据
	Data any `json:"data"`
}
