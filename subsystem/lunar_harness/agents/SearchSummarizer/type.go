package SearchSummarizer

// type.go 全部类型定义（结构体、接口、别名）集中于此。

// Source 一页参与统一摘要的正文来源（首页或详情页）
type Source struct {
	Label string // 来源标签（如「搜索结果页」「详情页 1」）
	Title string // 页面标题
	URL   string // 页面地址
	Text  string // 正文
}

// chatMessage 聊天消息
type chatMessage struct {
	Role    string `json:"role"`    // 角色（user/assistant/system）
	Content any    `json:"content"` // 字符串内容
}
