package life

// wish.go 愿望判定：只在生活体系运行时调用。
//
// 到期提醒入未读队列、未来窗口摘要与心情结算，都由 lunar 包通过 SetWishHooks
// 注入，避免 chain/life 反向依赖具体的持久化实现。

import "time"

// JudgeWishes 到期愿望入未读队列，并刷新未来一小时摘要。
func JudgeWishes(now time.Time) string {
	return judgeWishes(now)
}

// UpcomingSummary 未来 window 内即将到期的愿望摘要；没有返回空串。
func UpcomingSummary(now time.Time, window time.Duration) string {
	mu.Lock()
	hook := wishUpcomingHook
	mu.Unlock()
	if hook == nil {
		return ""
	}
	return hook(now, window)
}

func judgeWishes(now time.Time) string {
	mu.Lock()
	dueHook := wishDueHook
	upcomingHook := wishUpcomingHook
	mu.Unlock()
	if dueHook != nil {
		dueHook()
	}
	if upcomingHook != nil {
		return upcomingHook(now, wishWindow)
	}
	return ""
}
