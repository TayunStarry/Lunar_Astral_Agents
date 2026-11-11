package WebCrawler

// crawl.go 执行体的对外入口：只做浏览器检索与正文提取，不发起任何模型调用。
// 搜索链负责调度与模型调用，本包只回答「抓到了什么」。

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/browser"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/internal/textutil"
)

// Crawl 执行「搜索引擎降级 → 详情页筛选 → 并发采集正文」的完整抓取流程。
// detailCount 为期望的详情页数量（≤0 时取配置默认值）。
func Crawl(query string, detailCount int, cfg Config, deadline time.Time) (CrawlResult, error) {
	var cache *PageCache
	if cfg.cacheEnabled() {
		pc, err := openPageCache(time.Duration(cfg.CacheTTLDays) * 24 * time.Hour)
		if err != nil {
			loggerWarn("%v", err)
		} else {
			cache = pc
			defer pc.close()
		}
	}

	n := detailCount
	if n <= 0 {
		n = cfg.DetailCount()
	}
	if n > webSearchPagesHardCap-1 {
		n = webSearchPagesHardCap - 1
	}

	if time.Now().After(deadline) {
		return CrawlResult{}, fmt.Errorf("检索阶段超时，未开始浏览器检索")
	}
	logger.Info("CrystalAstral", "Web-Search 开始检索，计划详情页 %d 个", n)

	home, err := browser.OpenWebViewSession("", "网页搜索:首页", 1280, 860)
	if err != nil {
		return CrawlResult{}, fmt.Errorf("打开浏览器会话失败: %w", err)
	}
	// 首页配额与详情页同一口径：总提取上限 ÷（详情页数 + 首页 1 条）
	outcome, searchErr := searchWithFallback(home, query, cfg.PerPageChars(n+1), cfg, deadline)
	home.Close()
	if searchErr != nil {
		return CrawlResult{}, searchErr
	}

	targets := planDetailPages(outcome, cache, n)
	if len(targets) == 0 {
		return CrawlResult{}, fmt.Errorf("搜索结果中没有可用详情页")
	}

	captures, interrupted := collectDetailPages(targets, cfg, deadline)
	cacheCaptures(cache, captures)

	return CrawlResult{
		Engine:      outcome.Engine.name,
		EngineName:  engineDisplayName(outcome.Engine.name),
		SearchURL:   outcome.URL,
		Extract:     outcome.Extract,
		SearchText:  outcome.SearchText,
		Pages:       captures,
		Interrupted: interrupted,
	}, nil
}

// FetchPage 读取单个指定页面并提取正文（供「读取指定链接」入口使用）。
func FetchPage(target string, budgetSeconds int, cfg Config, deadline time.Time) (PageCapture, error) {
	sess, err := browser.OpenWebViewSession("", "网页搜索:指定页面", 1280, 860)
	if err != nil {
		return PageCapture{}, fmt.Errorf("打开浏览器会话失败: %w", err)
	}
	defer sess.Close()

	readyTimeout := 20 * time.Second
	if remain := time.Until(deadline); remain < readyTimeout {
		readyTimeout = remain
	}
	if err := navTo(sess, target, readyTimeout, 1200*time.Millisecond); err != nil {
		return PageCapture{}, fmt.Errorf("打开页面失败: %w", err)
	}
	if time.Now().After(deadline) {
		return PageCapture{}, fmt.Errorf("页面读取超时（%d 秒）", budgetSeconds)
	}
	walk := walkPage(sess, cfg.PerPageChars(1), cfg, deadline, nil)
	if strings.TrimSpace(walk.Text) == "" {
		return PageCapture{}, fmt.Errorf("页面未提取到有效正文")
	}
	cap := PageCapture{
		Title:     textutil.FirstNonEmpty(walk.Title, target),
		URL:       textutil.FirstNonEmpty(walk.URL, target),
		Text:      walk.Text,
		TextChars: walk.TextChars,
	}
	cap.Domain = domainOf(cap.URL)
	return cap, nil
}

// RememberSummary 把指定页面的摘要写入「近期已访问」缓存（缓存关闭时为空操作）。
func RememberSummary(target, title, summary string, cfg Config) error {
	if !cfg.cacheEnabled() {
		return nil
	}
	cache, err := openPageCache(time.Duration(cfg.CacheTTLDays) * 24 * time.Hour)
	if err != nil {
		return err
	}
	defer cache.close()
	cache.put(normalizeURL(target), title, domainOf(target), summary)
	return nil
}

// PageCountFromInstruction 解析指令中的「前 N 个/页/条」数量要求（无则返回 0）。
func PageCountFromInstruction(instruction string) int {
	m := pageCountRe.FindStringSubmatch(instruction)
	if len(m) < 2 {
		return 0
	}
	digits := strings.Map(func(r rune) rune {
		if r >= '０' && r <= '９' {
			return '0' + (r - '０')
		}
		return r
	}, m[1])
	n, err := strconv.Atoi(digits)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}
