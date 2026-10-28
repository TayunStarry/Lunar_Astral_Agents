package lunar

// url_extract.go @U 未缓存链接的即时提炼
//
// 用户发来普通网页地址时，资源目录与缓存里都没有摘要，这里让 WebSearch 现场提炼，
// 让句柄一进上下文就带着内容。
//
// 网络视频 / BV 号不再现场提取内容：它们统一登记为 @U 链接句柄，
// 模型需要时直接读取句柄里的地址、标题与缓存摘要即可；不再走视频内容理解管线。

import (
	"strings"
	"sync"

	"LunarSubsystem/LunarHarness/agents/WebSearch"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/resource"
)

// urlAttempted 本进程内已尝试提炼过的地址（失败的不再反复开页面）
var (
	urlAttemptMu sync.Mutex
	urlAttempted = map[string]bool{}
)

// markURLAttempted 标记一个地址的提炼尝试；返回 true 表示这是本进程第一次尝试
func markURLAttempted(url string) bool {
	urlAttemptMu.Lock()
	defer urlAttemptMu.Unlock()
	key := strings.TrimSpace(url)
	if key == "" || urlAttempted[key] {
		return false
	}
	urlAttempted[key] = true
	return true
}

// refreshURLCache 现场提炼一个未缓存链接；预算耗尽或失败时返回空串
func refreshURLCache(url string, budget *urlExtractBudget, focus string) (title, summary string) {
	if budget == nil || budget.used >= urlExtractLimit {
		return "", ""
	}
	budget.used++
	logger.Info(lunarCoreModule, "[资源转换] 链接无缓存摘要，现场提炼（%d/%d）: %s",
		budget.used, urlExtractLimit, truncateRunes(url, 80))

	// 网络视频 / BV 号不再现场理解内容：只保留 @U 链接句柄。
	if resource.BilibiliBvid(url) != "" || isVideoSource(url) {
		return "", ""
	}
	if !isHTTPURL(url) {
		return "", ""
	}
	pageTitle, pageSummary, err := WebSearch.ReadURL(url, focus)
	if err != nil {
		logger.Warn(lunarCoreModule, "[资源转换] 页面读取失败: %v", err)
		return "", ""
	}
	return pageTitle, pageSummary
}

// hasURLSummary 链接是否已有可用摘要（资源目录已登记，或网页 / 观影缓存在期）
func hasURLSummary(url string) bool {
	if resource.URLSummaryOf(url) != "" {
		return true
	}
	summary, _ := resource.LookupURLCache(url)
	return strings.TrimSpace(summary) != ""
}

// isHTTPURL 是否为可直接打开的 http(s) 地址
func isHTTPURL(raw string) bool {
	lower := strings.ToLower(strings.TrimSpace(raw))
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

// batchURLFocus 本轮消息里对这次引用的文字诉求（去掉链接本身），作为提炼指令
func batchURLFocus(messages []PostMessage) string {
	var builder strings.Builder
	for _, message := range messages {
		if text, ok := message.Content.(string); ok {
			builder.WriteString(text)
			builder.WriteString("\n")
			continue
		}
		for _, item := range asItems(message.Content) {
			if item.Type == ContentText {
				builder.WriteString(item.Text)
				builder.WriteString("\n")
			}
		}
	}
	focus := textURLPattern.ReplaceAllString(builder.String(), " ")
	focus = textBVPattern.ReplaceAllString(focus, " ")
	focus = strings.Join(strings.Fields(focus), " ")
	if focus == "" {
		return ""
	}
	return truncateRunes(focus, urlFocusRemainRunes)
}
