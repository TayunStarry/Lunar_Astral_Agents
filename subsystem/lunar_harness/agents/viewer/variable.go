package viewer

// ==== 族级全局变量与常量集中定义 ====
// 「观影」族的成员各自持有自己的 AgentID / 显示名 / 提示词，
// 这里只保留族级共享的部分。

import "time"

const (
	// defaultAttempts 文本类调用的默认尝试次数（含首次）
	defaultAttempts = 3
	// defaultTimeout 单次调用超时
	defaultTimeout = 5 * time.Minute
	// retryBackoff 瞬时错误重试的基础退避
	retryBackoff = time.Second
	// loggerCategory 日志分类（沿用观影链既有分类）
	loggerCategory = "BilibiliViewer"
)
