package WebSearch

// pages.go 两套页面的详情采集：
//   - 「网页搜索:首页」由 host.go 打开并完成搜索与结果提取，拿到可用详情页链接后关闭；
//   - 「网页搜索:详情」按筛选结果同时打开全部窗口，全部等待加载完成后统一提取正文，
//     提取完成后统一关闭窗口。
//
// 打开失败或提取不到有效正文的页面直接舍弃，不进入摘要与报告流程。

import (
	"LunarSubsystem/LunarHarness/engine/browser"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"fmt"
	"strings"
	"sync"
	"time"
)

// detailWindowTitles 详情页窗口标题（含序号，便于在多窗口间辨认）
func detailWindowTitles(n int) []string {
	titles := make([]string, n)
	for i := 0; i < n; i++ {
		titles[i] = fmt.Sprintf("网页搜索:详情 %d/%d", i+1, n)
	}
	return titles
}

// planDetailPages 从搜索结果中筛选出需要同时打开的详情页链接：
// 无效链接与页内重复舍弃，近期已访问的跳过，最多返回 n 条。
func planDetailPages(outcome searchOutcome, cache *PageCache, n int) []SearchHit {
	targets := make([]SearchHit, 0, n)
	visited := map[string]bool{} // 结果列表内去重：同一页面只打开一次

	for _, hit := range outcome.Extract.Results {
		if len(targets) >= n {
			break
		}
		key := normalizeURL(hit.URL)
		if key == "" || visited[key] {
			logger.Info("CrystalAstral", "Web-Search 跳过重复/无效链接: %s", truncateRunes(hit.URL, 80))
			continue
		}
		visited[key] = true
		if cache.fresh(key) {
			logger.Info("CrystalAstral", "Web-Search 跳过近期已访问页面: %s", truncateRunes(hit.URL, 80))
			continue
		}
		targets = append(targets, hit)
	}
	return targets
}

// collectDetailPages 用「网页搜索:详情」窗口同时打开全部目标页面：
// 阶段一并发创建窗口并导航（各自等待加载完成）；阶段二在全部窗口就绪后
// 统一滚动提取正文；最后统一关闭窗口。返回按打开顺序排列的采集结果。
func collectDetailPages(targets []SearchHit, cfg Config, deadline time.Time) ([]PageCapture, bool) {
	titles := detailWindowTitles(len(targets))
	sessions := make([]*browser.WebViewSession, len(targets))
	navErrs := make([]error, len(targets))
	captures := make([]PageCapture, len(targets))
	ok := make([]bool, len(targets))
	interrupted := false

	// 阶段一：同时打开全部详情窗口并导航，各自等待页面加载完成
	var wg sync.WaitGroup
	for i := range targets {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sess, err := browser.OpenWebViewSession("", titles[i], 960, 720)
			if err != nil {
				navErrs[i] = fmt.Errorf("打开窗口失败: %w", err)
				return
			}
			sessions[i] = sess
			navErrs[i] = navTo(sess, targets[i].URL, 15*time.Second, 1200*time.Millisecond)
		}(i)
	}
	wg.Wait()

	for i, err := range navErrs {
		if err == nil {
			continue
		}
		logger.SubWarn("CrystalAstral", "WebSearch", "详情页打开失败，舍弃该网页: %s (%v)", truncateRunes(targets[i].URL, 80), err)
		if sessions[i] != nil {
			sessions[i].Close()
			sessions[i] = nil
		}
		if !time.Now().After(deadline) {
			interrupted = interrupted || strings.Contains(err.Error(), "会话已关闭")
		}
	}

	// 阶段二：全部窗口加载完成后，统一并发滚动提取正文
	// （单页配额 = 总提取上限 ÷ 计划页面条目数，与首页配额同一口径均分）
	perPage := cfg.perPageChars(cfg.detailCount() + 1)
	for i := range sessions {
		if sessions[i] == nil {
			continue
		}
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			walk := walkPage(sessions[i], perPage, cfg, deadline, nil)
			if strings.TrimSpace(walk.Text) == "" {
				logger.SubWarn("CrystalAstral", "WebSearch", "页面未提取到有效正文，舍弃该网页: %s", truncateRunes(targets[i].URL, 80))
				return
			}
			captures[i] = PageCapture{
				Title:     firstNonEmpty(walk.Title, targets[i].Title),
				URL:       firstNonEmpty(walk.URL, targets[i].URL),
				Text:      walk.Text,
				TextChars: walk.TextChars,
			}
			captures[i].Domain = domainOf(captures[i].URL)
			ok[i] = true
		}(i)
	}
	wg.Wait()

	// 用户在截止时间前手动关窗视为中断（以已采集信息收尾）
	for _, sess := range sessions {
		if sess != nil && !sess.IsAlive() && !time.Now().After(deadline) {
			interrupted = true
		}
	}

	// 统一关闭全部详情窗口
	for _, sess := range sessions {
		if sess != nil {
			sess.Close()
		}
	}

	intels := make([]PageCapture, 0, len(targets))
	for i := range captures {
		if ok[i] {
			intels = append(intels, captures[i])
		}
	}
	return intels, interrupted
}

// cacheCaptures 把成功采集的页面写入「近期已访问」缓存。
// 统一摘要尚未产出，此处以正文前段作为记录内容，保证下次能识别为近期已访问。
func cacheCaptures(cache *PageCache, captures []PageCapture) {
	for _, cap := range captures {
		summary := hardCutRunes(cap.Text, 300)
		key := normalizeURL(cap.URL)
		cache.put(key, cap.Title, cap.Domain, summary)
	}
}
