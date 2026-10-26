package QQMessenger

// type.go 专项通讯智能体的类型定义。

import "time"

// sendPlan 模型解析出的投递计划（支持一次指定多个目标，按顺序逐个投递）
type sendPlan struct {
	Deliveries []sendDelivery `json:"deliveries"` // 多目标投递清单（新格式）
	Reason     string         `json:"reason"`     // 无法执行时的整体说明

	// 兼容旧单目标格式（模型偶尔仍按旧提示词输出）
	TargetType string   `json:"target_type,omitempty"`
	Target     string   `json:"target,omitempty"`
	Text       string   `json:"text,omitempty"`
	Images     []string `json:"images,omitempty"`
	Files      []string `json:"files,omitempty"`
}

// sendDelivery 单个目标的投递内容
type sendDelivery struct {
	TargetType string   `json:"target_type"` // group / user
	Target     string   `json:"target"`      // 群号 / QQ 号 / 名称
	Text       string   `json:"text"`        // 直接投递的文本（可空）
	Images     []string `json:"images"`      // 图片句柄（@I.xxx）
	Files      []string `json:"files"`       // 文件句柄（@D./@V./@A./@T. 等）
	Reason     string   `json:"reason"`      // 该目标无法执行时的说明
}

// chatMessage OpenAI 兼容的文本消息
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// modelUsage 流式响应中的 token 用量
type modelUsage struct {
	Prompt              int64               `json:"prompt_tokens"`
	Completion          int64               `json:"completion_tokens"`
	PromptTokensDetails *promptTokenDetails `json:"prompt_tokens_details,omitempty"`
}

// promptTokenDetails 输入 token 的细分（提示缓存命中数）
type promptTokenDetails struct {
	CachedTokens int64 `json:"cached_tokens"`
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

// deliveryRecord 最近一次专项通讯需求（供数据导出）
type deliveryRecord struct {
	Instruction string         `json:"instruction"`
	Deliveries  []sendDelivery `json:"deliveries"`
	Report      string         `json:"report"`
	Success     bool           `json:"success"`
	At          time.Time      `json:"at"`
}
