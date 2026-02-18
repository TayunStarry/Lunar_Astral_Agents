package setup

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

// MessageItem 消息项结构体
type MessageItem struct {
	// 消息类型
	Type string `json:"type"`
	// 消息数据
	Data MessageItemData `json:"data"`
}

// MessageItemData 消息项数据结构体
type MessageItemData struct {
	// 文本消息
	Text string `json:"text,omitempty"`
	Url  string `json:"url,omitempty"`
	File string `json:"file,omitempty"`
	QQ   string `json:"qq,omitempty"`
	ID   string `json:"id,omitempty"`
}

// Config 配置结构体
type Config struct {
	// NapCat WebSocket 服务器地址
	NapCatWSServer string `json:"napcat_ws_server"`
	// NapCat WebSocket 服务器令牌
	NapCatWSToken string `json:"napcat_ws_token"`
	// OpenAI API URL
	OpenAIAPIUrl string `json:"openai_api_url"`
	// OpenAI API 令牌
	OpenAIAPIToken string `json:"openai_api_token"`
	// OpenAI API 模型
	OpenAIAPIModel string `json:"openai_api_model"`
	// 轮询间隔（秒）
	PollInterval int `json:"poll_interval"`
	// 监听的群 ID 列表
	ListenGroupIDs []int64 `json:"listen_group_ids"`
	// 触发关键词列表
	TriggerKeywords []string `json:"trigger_keywords"`
	// 默认回复
	DefaultReply string `json:"default_reply"`
}
