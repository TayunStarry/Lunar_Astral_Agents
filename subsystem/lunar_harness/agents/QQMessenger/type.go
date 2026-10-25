package QQMessenger

// type.go QQ 投递智能体的类型定义。

// sendPlan 模型解析出的投递计划（一次只投递一个目标）
type sendPlan struct {
	TargetType string   `json:"target_type"` // group / user
	Target     string   `json:"target"`      // 群号 / QQ 号 / 名称
	Text       string   `json:"text"`        // 直接投递的文本（可空）
	Images     []string `json:"images"`      // 图片句柄（@I.xxx）
	Files      []string `json:"files"`       // 文件句柄（@D./@V./@A./@T. 等）
	Reason     string   `json:"reason"`      // 无法执行时的说明
}

// chatMessage OpenAI 兼容的文本消息
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// modelUsage 流式响应中的 token 用量
type modelUsage struct {
	Prompt     int64 `json:"prompt_tokens"`
	Completion int64 `json:"completion_tokens"`
}

// resolvedTarget 解析后的投递目标
type resolvedTarget struct {
	Kind string // group / user
	ID   int64
	Name string
}

// deliveryOutcome 本轮投递的累计结果
type deliveryOutcome struct {
	TextSent   bool
	ImagesSent int
	FilesSent  int
	Failures   []string
}
