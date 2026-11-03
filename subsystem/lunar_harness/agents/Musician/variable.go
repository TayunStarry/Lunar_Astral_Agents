package Musician

// variable.go 演奏者的提示词、角色标签与缺省篇幅参数。

const (
	// module 日志模块名
	module = "Musician"
	// roleLabel 推理实况与请求头角色标签
	roleLabel = "演奏者"
	// systemPromptPath 演奏者提示词（相对 prompts/）
	systemPromptPath = "musicianRole.md"
	// minMelodyBars 缺省篇幅（小节数）
	minMelodyBars = 12
	// defaultTempo 缺省速度（BPM）
	defaultTempo = 100
	// retryAttempts 输出陷入重复时的总尝试次数（含首次）
	retryAttempts = 2
)
