package SearchPlanner

// type.go 全部类型定义（结构体、接口、别名）集中于此。

// Decision 规划智能体一步产出的结构化决策
type Decision struct {
	Pipeline   string         `json:"pipeline"`             // 命中管线（search / weather）
	Query      string         `json:"query"`                // 检索词（天气管线用作展示）
	Args       map[string]any `json:"args,omitempty"`       // 管线参数（如天气的 city）
	Confidence *float64       `json:"confidence,omitempty"` // 模型自评把握
}

// routePipeline 可路由管线（提示词候选列表由注册表生成）
type routePipeline struct {
	ID          string // 管线标识（模型只能从中选）
	Description string // 给模型看的用途说明与参数约定
}

// chatMessage 聊天消息
type chatMessage struct {
	Role    string `json:"role"`    // 角色（user/assistant/system）
	Content any    `json:"content"` // 字符串内容
}
