package WebLTP

// Web-LTP 页面文本与元素提取
//
// 提取目标是「可读正文」而不是整页 DOM：优先 article/main/[role=main]，
// 缺失时回退 body；保留链接与检索结果，正文按 TextMaxChars 截断，
// 同时回报截断前的总字数与视觉元素数量，供摘要策略判断。

import (
	"LunarSubsystem/LunarHarness/engine/browser"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// extractPage 元素识别：结果页顺带提取检索条目；普通页提取正文与链接
func extractPage(sess *browser.WebViewSession, maxResults, textMax int) (PageExtract, error) {
	page := PageExtract{}
	if textMax <= 0 {
		textMax = 24000
	}
	raw, err := sess.EvalJSON(extractScript(maxResults, textMax), 8*time.Second)
	if err != nil {
		return page, err
	}
	if err := json.Unmarshal([]byte(raw), &page); err != nil {
		return page, fmt.Errorf("解析提取结果失败: %w", err)
	}
	if page.Kind == "" {
		page.Kind = "page"
	}
	if page.Results == nil {
		page.Results = []SearchHit{}
	}
	if page.Links == nil {
		page.Links = []SearchHit{}
	}
	return page, nil
}

func extractScript(maxResults, textMax int) string {
	return fmt.Sprintf(`return (function(){
    var out = { kind: 'page', url: location.href, title: document.title, results: [], links: [], text: '', textChars: 0, images: 0, visuals: 0 };
    var lis = document.querySelectorAll('#b_results > li.b_algo');
    if (lis.length > 0) { out.kind = 'search'; }
    var seen = {};
    lis.forEach(function (li) {
        if (out.results.length >= %d) return;
        var a = li.querySelector('h2 a');
        if (!a || !a.href || seen[a.href]) return;
        var cap = li.querySelector('.b_caption p') || li.querySelector('.b_caption') || li.querySelector('p');
        out.results.push({
            title: (a.textContent || '').trim(),
            url: a.href,
            snippet: cap ? (cap.textContent || '').trim().slice(0, 320) : ''
        });
        seen[a.href] = 1;
    });
    var root = document.querySelector('article') || document.querySelector('main') || document.querySelector('[role="main"]') || document.body;
    var text = root ? (root.innerText || root.textContent || '') : '';
    text = text.replace(/\u00a0/g, ' ').replace(/[ \t]+/g, ' ').replace(/\n{3,}/g, '\n\n').trim();
    out.textChars = text.length;
    out.text = text.slice(0, %d);
    out.images = document.images ? document.images.length : 0;
    out.visuals = document.querySelectorAll('canvas,svg,video,figure').length;
    var anchors = document.querySelectorAll('a[href]');
    for (var i = 0; i < anchors.length && out.links.length < 25; i++) {
        var a = anchors[i];
        var h = a.href;
        if (!/^https?:/.test(h) || seen[h]) continue;
        var t = (a.textContent || '').trim().replace(/\s+/g, ' ');
        if (t.length > 8 && t.length < 120) { seen[h] = 1; out.links.push({ title: t, url: h }); }
    }
    return out;
})()`, maxResults, textMax)
}

// domainOf 提取 URL 的域名（用于报告来源标注）
func domainOf(raw string) string {
	u := strings.TrimSpace(raw)
	u = strings.TrimPrefix(u, "https://")
	u = strings.TrimPrefix(u, "http://")
	if i := strings.IndexAny(u, "/?#"); i >= 0 {
		u = u[:i]
	}
	return u
}
