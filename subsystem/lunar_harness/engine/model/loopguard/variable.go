package loopguard

// variable.go 默认参数、流类型与统一错误。

import (
	"errors"
	"time"
)

const (
	// DefaultWindowRunes 单个滑动窗口的字符数
	DefaultWindowRunes = 64
	// DefaultWindowCount 判定重复所需的连续窗口数
	DefaultWindowCount = 3
	// DefaultSimilarity 相似度阈值（0~1，超过即视为同一个循环）
	DefaultSimilarity = 0.5
	// DefaultInterval 后台检查间隔
	DefaultInterval = time.Second
)

// 流式文本种类：思考与正文分开检测，避免两类文本拼接造成误判。
const (
	KindThink   = "think"
	KindContent = "content"
)

// ErrLoop 模型陷入重复输出（由思考监管强制中断）。
// 调用方用 errors.Is 判断，并按各自任务编排选择重试、降级或跳过。
var ErrLoop = errors.New("模型陷入重复输出")
