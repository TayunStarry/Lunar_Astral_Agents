package viewer

// auto_respond_source.go 把消息中心条目归到三类回应来源。
//
// 来源判定只看条目自身字段（消息中心已给出 business_id 与 uri）：
//   - 带 @ 的条目一律算「@我的」，即使它落在视频或动态评论区；
//   - 视频评论区（type=1）算「我的视频评论」；
//   - 动态/图文动态（type=11、17）算「我的动态评论」。
//
// 其余类型（专栏等）没有对应页面能力，本轮不处理，也不会进入重试队列。

// autoRespondSource 返回条目所属来源；空串表示本轮不支持。
func autoRespondSource(item inboxItem) string {
	if item.Kind == "at" || item.Category == notifyAt {
		return sourceAt
	}
	switch {
	case item.Type == 11, item.Type == 17, item.Venue == "dynamic":
		return sourceDynamic
	case item.Type == 1, item.Venue == "video":
		return sourceVideo
	}
	return ""
}
