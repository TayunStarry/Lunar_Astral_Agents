package WebSearch

// extract.go 页面正文提取
//
// 提取目标是「可读正文」而不是整页 DOM：优先 article/main/[role=main]，
// 缺失时回退 body；正文按 TextMaxChars 截断，同时回报截断前的总字数。

import (
	"LunarSubsystem/LunarHarness/engine/browser"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// extractPage 提取当前页面地址、标题与正文
func extractPage(sess *browser.WebViewSession, textMax int) (PageExtract, error) {
	page := PageExtract{}
	if textMax <= 0 {
		textMax = 24000
	}
	raw, err := sess.EvalJSON(extractScript(textMax), 8*time.Second)
	if err != nil {
		return page, err
	}
	if err := json.Unmarshal([]byte(raw), &page); err != nil {
		return page, fmt.Errorf("解析提取结果失败: %w", err)
	}
	return page, nil
}

// extractScript 正文提取脚本
func extractScript(textMax int) string {
	return fmt.Sprintf(`return (function(){
    var root = document.querySelector('article') || document.querySelector('main') || document.querySelector('[role="main"]') || document.body;
    var text = root ? (root.innerText || root.textContent || '') : '';
    text = text.replace(/\u00a0/g, ' ').replace(/[ \t]+/g, ' ').replace(/\n{3,}/g, '\n\n').trim();
    return {
        kind: 'page',
        url: location.href,
        title: document.title,
        textChars: text.length,
        text: text.slice(0, %d)
    };
})();`, textMax)
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
