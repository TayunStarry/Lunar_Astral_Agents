package thinking

// plan_bridge.go 计划表与生活体系的桥接：计划表存储已迁移到 engine/plans，
// 这里只负责到期项入未读队列与未来窗口摘要注入（life 体系钩子）。

import (
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/plans"
)

// initSchedules 初始化计划表（启动时调用）。
func initSchedules() {
	plans.Init()
}

// pushDueScheduleItems 消费到期计划项并推进未读上下文，返回入队条数。
// 只在生活体系运行时调用（计划表判定已从对话 Tick 迁移）。
func pushDueScheduleItems() int {
	due := plans.ConsumeDue()
	for _, item := range due {
		unreadMu.Lock()
		unreadContext = append(unreadContext, PostMessage{Role: RoleUser, Source: scheduleSource, Content: scheduleTriggerPrefix + " 预约时间已到，请执行以下计划：" + item.Content})
		unreadMu.Unlock()
	}
	if len(due) > 0 {
		logger.Info(lunarCoreModule, "[计划表] 生活体系推送 %d 条到期计划", len(due))
	}
	return len(due)
}

// upcomingPlanSummary 未来 window 内即将触发的计划摘要；没有则返回空串。
func upcomingPlanSummary(now time.Time, window time.Duration) string {
	return plans.Upcoming(now, window)
}

// normalizeTime 兼容旧的计划表时间归一化入口（测试与外部小工具使用）。
func normalizeTime(raw string) string {
	return plans.NormalizeTime(raw)
}
