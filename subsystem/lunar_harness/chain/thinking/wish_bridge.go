package thinking

// wish_bridge.go 心愿链路与生活体系的桥接：愿望到期入未读队列、未来窗口摘要注入。
//
// 愿望存储、心情与态度统一收敛在 chain/wishchain；这里只负责把到期提醒
// 翻译成对话者运行时消息，以及提供未来窗口摘要。

import (
	"time"

	"LunarSubsystem/LunarHarness/chain/wishchain"
	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// initWishes 初始化心愿链路（启动时调用）。
func initWishes() {
	wishchain.Load()
	logger.Info(lunarCoreModule, "[愿望] 初始化完成，待实现 %d 条", wishchain.CountPending())
}

// pushDueWishItems 消费到期愿望并推进未读上下文，返回入队条数。
//
// 提醒文本保持「XX愿望到期」格式，便于对话直接理解。
func pushDueWishItems() int {
	notices := wishchain.ConsumeDue(time.Now())
	for _, notice := range notices {
		unreadMu.Lock()
		unreadContext = append(unreadContext, PostMessage{
			Role:    RoleUser,
			Source:  wishSource,
			Content: wishTriggerPrefix + " " + notice.Message(),
		})
		unreadMu.Unlock()
	}
	if len(notices) > 0 {
		logger.Info(lunarCoreModule, "[愿望] 生活体系推送 %d 条到期愿望", len(notices))
	}
	return len(notices)
}

// upcomingWishSummary 未来 window 内即将到期的愿望摘要；没有则返回空串。
func upcomingWishSummary(now time.Time, window time.Duration) string {
	return wishchain.Upcoming(now, window)
}

// normalizeTime 兼容旧的时间归一化入口（测试与外部小工具使用）。
func normalizeTime(raw string) string {
	return wishchain.NormalizeTime(raw)
}
