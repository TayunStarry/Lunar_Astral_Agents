package life

// plan.go 计划表判定：只在生活体系运行时调用。
//
// 具体读取计划文件、把到期项推入未读队列、计算未来一小时摘要，
// 都由 lunar 包通过 SetPlanHooks 注入，避免 engine/life 反向依赖 lunar。

import "time"

// JudgePlans 到期项入未读队列，并刷新未来一小时摘要。
func JudgePlans(now time.Time) string {
	return judgePlans(now)
}

// UpcomingSummary 未来 window 内即将触发的计划摘要；没有返回空串。
func UpcomingSummary(now time.Time, window time.Duration) string {
	mu.Lock()
	hook := planUpcomingHook
	mu.Unlock()
	if hook == nil {
		return ""
	}
	return hook(now, window)
}

func judgePlans(now time.Time) string {
	mu.Lock()
	dueHook := planDueHook
	upcomingHook := planUpcomingHook
	mu.Unlock()
	if dueHook != nil {
		dueHook()
	}
	if upcomingHook != nil {
		return upcomingHook(now, planWindow)
	}
	return ""
}
