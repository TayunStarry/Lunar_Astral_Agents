package file

// variable.go 工具推理循环的默认运行参数。
// 成员各自的 AgentID / 角色 / 提示词 / 轮次上限放在各自包的 variable.go，不回流到这里。

import "time"

const (
	// defaultMaxRounds 默认工具循环轮次上限
	defaultMaxRounds = 24
	// defaultTimeout 默认单次模型调用超时
	defaultTimeout = 90 * time.Second
	// defaultMaxTokens 默认单次输出上限（足以生成一整章正文）
	defaultMaxTokens = 8192
	// defaultSummaryMaxTokens 修改完成后的总结输出上限
	defaultSummaryMaxTokens = 320
	// defaultPostToolMaxTokens 工具执行后、尚未发生修改时的输出上限。
	// 足够发起一次带正文的工具调用，又能阻止模型在工具报错后无限生成。
	defaultPostToolMaxTokens = 2048
	// maxRetryTokens 工具参数被截断后允许放大到的输出上限
	maxRetryTokens = 16384
	// maxToolResultRunes 单条工具结果回填模型的字符上限
	maxToolResultRunes = 6000
)
