package loopguard

// variable.go 默认参数、流类型与统一错误。

import "errors"

const (
	// DefaultNodeRunes 判定节点长度：输出每增长这么多字检查一次。
	//
	// 节点检查而不是每秒轮询：节点内切 8 段求平均，衡量的是「持续复读」，
	// 短时间的模板化枚举（例如几帧 Same as Frame8）不会触发。
	DefaultNodeRunes = 1024
	// DefaultSegmentRunes 节点内单个片段的字符数（1024 / 128 = 8 段）
	DefaultSegmentRunes = 128
	// DefaultSimilarity 相邻片段平均相似度的警戒阈值（0~1）。
	//
	// 0.65 来自实际转写样本的标定：正常逐帧描述/长文的节点平均相似度约 0.30-0.53；
	// 真正的持续复读约 0.73-0.95。取 0.65 留出双向余量；误判多时可上调，
	// 漏判多时可下调（system.loop_similarity）。
	DefaultSimilarity = 0.65
)

// 流式文本种类：思考与正文分开检测，避免两类文本拼接造成误判。
const (
	KindThink   = "think"
	KindContent = "content"
)

// ErrLoop 模型陷入重复输出（由思考监管强制中断）。
// 调用方用 errors.Is 判断，并按各自任务编排选择重试、降级或跳过。
var ErrLoop = errors.New("模型陷入重复输出")
