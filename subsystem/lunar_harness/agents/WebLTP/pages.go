package WebLTP

// pages.go 结果页处理：筛选近期未访问的链接，逐页滚动并同步摘要（步骤 2-4），
// 以及回访搜索结果主页面（步骤 5 的准备）。

import (
	"LunarSubsystem/LunarHarness/engine/browser"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"strings"
	"time"
)

// collectDetailPages 依次进入前 n 个近期未访问的结果页：滚动到底部并同步摘要。
// 页面打开失败或提取不到有效正文时直接舍弃，不进入摘要与报告流程。
func collectDetailPages(sess *browser.WebViewSession, outcome searchOutcome, query string,
	cfg Config, cache *PageCache, profile summaryProfile, n int, deadline time.Time) ([]PageIntel, bool) {
	intels := make([]PageIntel, 0, n)
	visited := map[string]bool{} // 跨结果页 URL 去重：同一页面只进入一次
	interrupted := false

	for _, hit := range outcome.Extract.Results {
		if len(intels) >= n {
			break
		}
		if !sess.IsAlive() {
			interrupted = true
			break
		}
		if time.Now().After(deadline) {
			break
		}

		key := normalizeURL(hit.URL)
		if key == "" || visited[key] {
			logger.Info("CrystalAstral", "Web-LTP 跳过重复/无效链接: %s", truncateRunes(hit.URL, 80))
			continue
		}
		visited[key] = true
		if cache.fresh(key) {
			logger.Info("CrystalAstral", "Web-LTP 跳过近期已访问页面: %s", truncateRunes(hit.URL, 80))
			continue
		}

		if err := navTo(sess, hit.URL, 15*time.Second, 1200*time.Millisecond); err != nil {
			logger.SubWarn("CrystalAstral", "WebLTP", "页面打开失败，舍弃该网页: %s (%v)", truncateRunes(hit.URL, 80), err)
			continue
		}

		// 滚动与摘要同步进行：每屏正文快照即时投递，后台协程逐段生成摘要；
		// Finish 在页面底部等待全部摘要完成后再进入下一页。
		summarizer := newPageSummarizer(query, hit, cfg, profile)
		walk := walkPage(sess, cfg, deadline, summarizer.Feed)
		summary, chunks, sumErr := summarizer.Finish()

		if strings.TrimSpace(walk.Text) == "" {
			logger.SubWarn("CrystalAstral", "WebLTP", "页面未提取到有效正文，舍弃该网页: %s", truncateRunes(hit.URL, 80))
			continue
		}
		if sumErr != nil {
			logger.SubWarn("CrystalAstral", "WebLTP", "页面摘要失败，以正文前段兜底: %v", sumErr)
			summary = hardCutRunes(walk.Text, cfg.SummaryMaxChars)
		} else {
			summary = hardCutRunes(summary, cfg.SummaryMaxChars)
		}

		intel := PageIntel{
			Title:     firstNonEmpty(walk.Title, hit.Title),
			URL:       firstNonEmpty(walk.URL, hit.URL),
			Summary:   summary,
			TextChars: walk.TextChars,
			Chunks:    chunks,
		}
		intel.Domain = domainOf(intel.URL)
		intels = append(intels, intel)

		// 以页面最终地址为缓存键；跳转型结果链接另存一份，保证下次能识别为「近期已访问」
		finalKey := normalizeURL(intel.URL)
		cache.put(finalKey, intel.Title, intel.Domain, intel.Summary)
		if key != finalKey {
			cache.put(key, intel.Title, intel.Domain, intel.Summary)
		}
		logger.Info("CrystalAstral", "Web-LTP 页面 %d/%d 完成: %s (正文 %d 字, 摘要 %d 字, 分段 %d, 滚动 %d 步/触底 %v)",
			len(intels), n, intel.Domain, intel.TextChars, len([]rune(intel.Summary)), intel.Chunks, walk.Steps, walk.AtBottom)
	}
	return intels, interrupted
}

// revisitSearchPage 返回搜索结果主页面并提取正文，供整体汇总使用；
// 回访失败时沿用首次搜索时抓取的正文，保证最终总结不丢。
func revisitSearchPage(sess *browser.WebViewSession, outcome searchOutcome, cfg Config, deadline time.Time) string {
	if !sess.IsAlive() || time.Now().After(deadline) {
		return outcome.SearchText
	}
	if err := navTo(sess, outcome.URL, 15*time.Second, 1200*time.Millisecond); err != nil {
		logger.SubWarn("CrystalAstral", "WebLTP", "回访搜索页失败，沿用首次抓取的正文: %v", err)
		return outcome.SearchText
	}
	page, err := extractPage(sess, cfg.TextMaxChars)
	if err != nil {
		logger.SubWarn("CrystalAstral", "WebLTP", "搜索页正文提取失败，沿用首次抓取的正文: %v", err)
		return outcome.SearchText
	}
	if strings.TrimSpace(page.Text) == "" {
		return outcome.SearchText
	}
	return page.Text
}
