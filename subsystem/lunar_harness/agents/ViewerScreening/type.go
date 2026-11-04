package ViewerScreening

// type.go 观影者-阅片的输入输出类型。

// Impression 结构化「视频印象」：只看可采集的公开信息与画面，不做合规判定。
type Impression struct {
	Summary    string   `json:"summary"`
	Topics     []string `json:"topics"`
	Tone       string   `json:"tone"`
	Risky      bool     `json:"risky,omitempty"`
	RiskReason string   `json:"risk_reason,omitempty"`
}

// KeywordInput 本轮选片关键词的输入。
type KeywordInput struct {
	// Instruction 对话者本轮的想法
	Instruction string
	// Defaults 配置里的长期偏好
	Defaults []string
	// Mood 当前心情
	Mood string
	// Memory 长期观影记忆
	Memory string
}

// promptPayload 提示词模板的占位符替换对。
type promptPayload struct {
	Defaults string
	Mood     string
	Memory   string
}
