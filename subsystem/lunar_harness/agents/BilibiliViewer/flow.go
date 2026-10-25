package BilibiliViewer

// flow.go 观影者的工作流定义：由对话者通过工具参数枚举选择。

import "strings"

// Flow 观影者工作流
type Flow string

const (
	// FlowStandard 标准流程：回复未读评论 + 看视频与评论 + 写日记动态 + 回复动态评论
	FlowStandard Flow = "standard"
	// FlowWatchComment 看视频与评论 + 写日记动态（不处理未读、不回复动态评论）
	FlowWatchComment Flow = "watch_comment"
	// FlowReplyOnly 仅回复未读消息
	FlowReplyOnly Flow = "reply_only"
)

// ParseFlow 解析工具传入的流程名；空值按标准流程处理。
func ParseFlow(raw string) (Flow, bool) {
	switch Flow(strings.TrimSpace(raw)) {
	case "", FlowStandard:
		return FlowStandard, true
	case FlowWatchComment:
		return FlowWatchComment, true
	case FlowReplyOnly:
		return FlowReplyOnly, true
	}
	return "", false
}

// Watches 该流程是否包含观看视频（计入每日刷视频次数）。
func (f Flow) Watches() bool {
	return f == FlowStandard || f == FlowWatchComment
}

// includesInbox 该流程是否先处理未读消息。
func (f Flow) includesInbox() bool {
	return f == FlowStandard || f == FlowReplyOnly
}

// publishesDynamic 该流程是否发布日记动态：只要看视频就写日记动态。
func (f Flow) publishesDynamic() bool {
	return f.Watches()
}
