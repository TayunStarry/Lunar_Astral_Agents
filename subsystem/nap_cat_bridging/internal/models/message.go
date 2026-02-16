package models

// GroupInfo 群信息结构体
type GroupInfo struct {
	GroupID        int64  `json:"group_id"`
	GroupName      string `json:"group_name"`
	MemberCount    int    `json:"member_count"`
	MaxMemberCount int    `json:"max_member_count"`
}

// Sender 发送者结构体
type Sender struct {
	UserID   int64  `json:"user_id"`
	Nickname string `json:"nickname"`
	Card     string `json:"card"`
	Role     string `json:"role"`
}

// MsgItem 消息项结构体
type MsgItem struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

// GroupMessage 群消息结构体
type GroupMessage struct {
	SelfID        int64     `json:"self_id"`
	UserID        int64     `json:"user_id"`
	Time          int64     `json:"time"`
	MessageID     int64     `json:"message_id"`
	MessageSeq    int64     `json:"message_seq"`
	RealID        int64     `json:"real_id"`
	RealSeq       string    `json:"real_seq"`
	MessageType   string    `json:"message_type"`
	Sender        Sender    `json:"sender"`
	RawMessage    string    `json:"raw_message"`
	Font          int       `json:"font"`
	SubType       string    `json:"sub_type"`
	Message       []MsgItem `json:"message"`
	MessageFormat string    `json:"message_format"`
	PostType      string    `json:"post_type"`
	GroupID       int64     `json:"group_id"`
	GroupName     string    `json:"group_name"`
}

// GetGroupListParams 获取群列表请求参数
type GetGroupListParams struct {
	NoCache bool `json:"no_cache"`
}

// SendGroupMsgParams 发送群消息请求参数
type SendGroupMsgParams struct {
	GroupID int64     `json:"group_id"`
	Message []MsgItem `json:"message"`
}
