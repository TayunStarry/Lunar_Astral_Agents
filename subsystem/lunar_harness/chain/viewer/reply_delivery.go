package viewer

// reply_delivery.go 回应发布：直接走评论接口，不再尝试页面定位。
//
// 从 notify.go 抽出，供观影 Tick 的被动回应与生活链的 B 站自动响应共用，
// 保证「怎么把一条回应发出去」只有一份实现。
//
// 页面定位（在评论区找目标评论再点回复）在真机上基本命中不了，已整体移除；
// 发布结果判定、失败重试与去重语义保持不变：失败原样返回，由调用方决定重试或计数。

// publishReplyWith 用指定会话直接调用评论接口发布一条回应。
func publishReplyWith(session replySession, item inboxItem, text string) error {
	return session.replyByAPI(item, text)
}
