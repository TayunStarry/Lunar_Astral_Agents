package Musician

// variable.go 演奏者的提示词、角色标签、缺省篇幅与音乐模型切换参数。

import "time"

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
	// modelSwapWait 切换「主模型 ↔ 音乐模型」时等待对端释放显存的上限
	modelSwapWait = 60 * time.Second
	// modelLoadWait 音乐模型从触发加载到权重可用的等待上限
	modelLoadWait = 90 * time.Second
	// musicTokenBudget 单次演奏的 MIDI token 上限（AMT 每 3 个 token 一个音符）
	musicTokenBudget = 2046
	// candidateCount 单次演奏最多生成的候选数（首个为原描述，其后为带诊断的重写）
	candidateCount = 3
	// candidateTemperature 首个候选的采样温度（沿用模型卡片推荐值 1.0）
	candidateTemperature = 1.0
	// candidateTopP 首个候选的 nucleus 采样阈值（沿用模型卡片推荐值 0.98）
	candidateTopP = 0.98
	// retryTemperature 带诊断重写时降低温度，收敛到更规整的结构段
	retryTemperature = 0.9
	// retryTopP 带诊断重写时收窄 nucleus 采样范围
	retryTopP = 0.9
)
