package WebLTP

// Web-LTP 搜索引擎降级链：bing → baidu → sogou。
// 每个引擎一组「检索页 URL 模板 + 结果页 DOM 选择器」；依序尝试，
// 首个满足「导航成功、识别为搜索结果页、过滤后仍有结果、非工具站兜底」的引擎胜出。
// 单引擎失败（导航失败/识别失败/无结果/工具站兜底）记日志后尝试下一个；
// 全部失败时返回错误，由月华侧感知明确失败（可换检索词重新下发），而非空报告。
// 结果页先做 DOM 识别再滚动截图：无效结果页不浪费截图与时间。

import (
	"LunarSubsystem/BrowserClient"
	"LunarSubsystem/LoggerGeneral"
	"encoding/json"
	"fmt"
	"net/url"
	"time"
)


// engineFallbackOrder 搜索引擎降级顺序
var engineFallbackOrder = []searchEngine{
	{
		name:      "bing",
		searchURL: "https://cn.bing.com/search?q=",
		container: "#b_results > li.b_algo",
		link:      "h2 a",
		snippet:   ".b_caption p, .b_caption, p",
	},
	{
		name:      "baidu",
		searchURL: "https://www.baidu.com/s?wd=",
		container: "#content_left > div",
		link:      "h3 a",
		snippet:   `.c-abstract, span[class*="content-right"], [class*="abstract"], p`,
	},
	{
		name:      "sogou",
		searchURL: "https://www.sogou.com/web?query=",
		container: "div.rb, div.vrwrap",
		link:      "h3 a",
		snippet:   ".str-text-info, .str_info, p",
	},
}


// searchWithFallback 依序尝试搜索引擎降级链，返回首个有效结果页。
// 有效 = 导航成功 + 识别为搜索结果页 + 过滤字典站/去重后仍有结果 + 非工具站兜底。
// 全部引擎无效时返回错误。
func searchWithFallback(sess *BrowserClient.WebViewSession, query string, cfg Config, saver *shotSaver, deadline time.Time) (searchOutcome, error) {
	var lastErr error
	for _, e := range engineFallbackOrder {
		searchURL := e.searchURL + url.QueryEscape(query)

		if err := navTo(sess, searchURL, 20*time.Second, 1500*time.Millisecond); err != nil {
			lastErr = fmt.Errorf("%s 导航失败: %w", e.name, err)
			LoggerGeneral.Info("CrystalAstral", "Web-LTP [%s] 导航失败，尝试下一引擎: %v", e.name, err)
			continue
		}

		page, err := extractSerp(sess, e, cfg.MaxResults)
		if err != nil {
			lastErr = fmt.Errorf("%s 结果识别失败: %w", e.name, err)
			LoggerGeneral.Info("CrystalAstral", "Web-LTP [%s] 结果识别失败，尝试下一引擎: %v", e.name, err)
			continue
		}
		if page.Kind != "search" || len(page.Results) == 0 {
			lastErr = fmt.Errorf("%s 未识别到搜索结果（%d 条）", e.name, len(page.Results))
			LoggerGeneral.Info("CrystalAstral", "Web-LTP [%s] 无搜索结果，尝试下一引擎", e.name)
			continue
		}

		results := dedupeResults(filterDictionarySites(page.Results))
		if len(results) == 0 {
			lastErr = fmt.Errorf("%s 结果全部为字典站", e.name)
			LoggerGeneral.Info("CrystalAstral", "Web-LTP [%s] 结果全部为字典站，尝试下一引擎", e.name)
			continue
		}
		if detectToolFallback(results) {
			lastErr = fmt.Errorf("%s 疑似工具站兜底（多数结果为固定工具站）", e.name)
			LoggerGeneral.Info("CrystalAstral", "Web-LTP [%s] 检测到工具站兜底，尝试下一引擎", e.name)
			continue
		}
		page.Results = results

		// 结果有效 → 滚动截图（首屏供报告视觉印证）后回到页顶
		captures, shot := scrollCapture(sess, cfg.ResultsScrollCaptures, "搜索结果页", saver, deadline)
		_ = scrollToTop(sess)
		LoggerGeneral.Info("CrystalAstral", "Web-LTP 结果页识别完成 [%s]: %d 条结果, 截图 %d 张", e.name, len(page.Results), captures)
		return searchOutcome{Engine: e, URL: searchURL, Extract: page, FirstShot: shot}, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("搜索引擎降级链为空")
	}
	return searchOutcome{}, fmt.Errorf("所有搜索引擎（bing/baidu/sogou）均未获得有效结果页: %w", lastErr)
}

// extractSerp 在当前页面执行引擎结果提取（页内按 href 去重）
func extractSerp(sess *BrowserClient.WebViewSession, e searchEngine, maxResults int) (PageExtract, error) {
	page := PageExtract{}
	raw, err := sess.EvalJSON(serpScript(e, maxResults), 8*time.Second)
	if err != nil {
		return page, err
	}
	if err := json.Unmarshal([]byte(raw), &page); err != nil {
		return page, fmt.Errorf("解析结果页提取失败: %w", err)
	}
	if page.Results == nil {
		page.Results = []SearchHit{}
	}
	return page, nil
}

// serpScript 结果页提取脚本：按引擎选择器抓取标题/链接/摘要
func serpScript(e searchEngine, maxResults int) string {
	return fmt.Sprintf(`return (function(){
    var out = { kind: 'page', url: location.href, title: document.title, results: [] };
    var nodes = document.querySelectorAll('%s');
    var seen = {};
    nodes.forEach(function (node) {
        if (out.results.length >= %d) return;
        var a = node.querySelector('%s');
        if (!a || !a.href || seen[a.href]) return;
        var cap = node.querySelector("%s");
        out.results.push({
            title: (a.textContent || '').trim(),
            url: a.href,
            snippet: cap ? (cap.textContent || '').trim().slice(0, 320) : ''
        });
        seen[a.href] = 1;
    });
    if (out.results.length > 0) { out.kind = 'search'; }
    return out;
})()`, e.container, maxResults, e.link, e.snippet)
}

// engineDisplayName 引擎中文名（报告叙述用）
func engineDisplayName(name string) string {
	switch name {
	case "bing":
		return "必应"
	case "baidu":
		return "百度"
	case "sogou":
		return "搜狗"
	}
	return name
}
