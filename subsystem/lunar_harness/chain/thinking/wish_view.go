package thinking

// wish_view.go 心愿领域在对话运行时消息里的呈现文本。
//
// 领域规则（状态、配额、结算、持久化）在 engine/wishchain；这里只保留
// 运行时消息的措辞，避免引擎或智能体反向依赖链。

import (
	"sort"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/wishchain"
	"LunarSubsystem/LunarHarness/internal/textutil"
)

// dueWishMessage 到期提醒要推送给人格对话的文本，格式保持「XX愿望到期」。
func dueWishMessage(notice wishchain.DueNotice) string {
	return notice.Wish.Content + "愿望到期"
}

// obsessionText 返回运行时消息里的单一执念提示。
//
// 逾期愿望不销毁，全部保留到完成或删除；多条逾期时在这里整合为一条，
// 保证运行时消息只出现一个「执念」。
func obsessionText() string {
	overdue := wishchain.OverdueWishes(time.Now())
	if len(overdue) == 0 {
		return ""
	}
	sort.SliceStable(overdue, func(i, j int) bool {
		if overdue[i].Priority != overdue[j].Priority {
			return overdue[i].Priority < overdue[j].Priority
		}
		return overdue[i].ID < overdue[j].ID
	})
	parts := make([]string, 0, len(overdue))
	seen := make(map[string]bool, len(overdue))
	for _, item := range overdue {
		content := strings.TrimSpace(item.Content)
		if content == "" || seen[content] {
			continue
		}
		seen[content] = true
		parts = append(parts, "「"+content+"」")
	}
	return "请寻找办法完成已失约的 " + strings.Join(parts, "、") + " 愿望"
}

// upcomingWishText 返回未来 window 内即将到期的愿望摘要；没有则返回空串。
func upcomingWishText(now time.Time, window time.Duration) string {
	parts := make([]string, 0)
	for _, item := range wishchain.List() {
		due, ok := wishchain.ParseDue(item.Due)
		if !ok || !due.After(now) || due.After(now.Add(window)) {
			continue
		}
		parts = append(parts, due.Format("15:04")+" "+textutil.TruncateRunes(item.Content, 128))
	}
	return strings.Join(parts, "；")
}
