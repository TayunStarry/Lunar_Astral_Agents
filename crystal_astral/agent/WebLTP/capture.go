package WebLTP

// Web-LTP 截图与页面元素提取（基于 BrowserClient.WebViewSession）

import (
	"LunarSubsystem/BrowserClient"
	"fmt"
	"image/jpeg"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	imageproc "LunarSubsystem/MultimodalAnalysis/module"
)

// shotCapture 会话窗口截图：优先 PrintWindow 内容截图（与窗口遮挡状态无关），
// 失败时回退屏幕 DC 区域截图，返回 JPEG 字节
func shotCapture(sess *BrowserClient.WebViewSession, quality int) ([]byte, error) {
	if quality <= 0 || quality > 100 {
		quality = 85
	}
	img, err := sess.ClientScreenshot()
	if err != nil {
		// 回退：屏幕区域截图（窗口被遮挡时会截到遮挡内容）
		x, y, w, h, rerr := sess.WindowRect()
		if rerr != nil {
			return nil, fmt.Errorf("内容截图失败: %w；区域截图失败: %w", err, rerr)
		}
		img, err = imageproc.CaptureScreenRegionRGBA(x, y, w, h)
		if err != nil {
			return nil, fmt.Errorf("区域截图失败: %w", err)
		}
	}
	buf := &bytes.Buffer{}
	if err := jpeg.Encode(buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return nil, fmt.Errorf("截图编码失败: %w", err)
	}
	return buf.Bytes(), nil
}

// shotSaver 截图落盘器（save_screenshots=false 时仅计数）
type shotSaver struct {
	enabled bool
	dir     string
	count   int
	files   []string
}

// save 保存一张截图（按顺序编号命名），返回序号
func (s *shotSaver) save(jpg []byte, label string) int {
	s.count++
	if !s.enabled {
		return s.count
	}
	name := fmt.Sprintf("%02d-%s.jpg", s.count, sanitizeFileName(label))
	path := filepath.Join(s.dir, name)
	if err := os.MkdirAll(s.dir, 0o755); err == nil {
		if err := os.WriteFile(path, jpg, 0o644); err == nil {
			s.files = append(s.files, path)
			return s.count
		}
	}
	return s.count
}

var fileNameUnsafe = regexp.MustCompile(`[\\/:*?"<>|\s]+`)

func sanitizeFileName(s string) string {
	s = fileNameUnsafe.ReplaceAllString(s, "-")
	if len(s) > 60 {
		r := []rune(s)
		if len(r) > 60 {
			s = string(r[:60])
		}
	}
	return strings.Trim(s, "-")
}

// scrollState 一次滚动后的页面状态
type scrollState struct {
	Before   float64 `json:"before"`
	After    float64 `json:"after"`
	AtBottom bool    `json:"atBottom"`
	Height   float64 `json:"height"`
}

// scrollByViewport 向下滚动约一屏，返回是否触底（EvalJSON 在页面上下文执行）
func scrollByViewport(sess *BrowserClient.WebViewSession) (scrollState, error) {
	var st scrollState
	raw, err := sess.EvalJSON(`return (function(){
        var doc = document.documentElement;
        var before = window.scrollY;
        window.scrollBy(0, Math.round(window.innerHeight * 0.9));
        var after = window.scrollY;
        return {before: before, after: after, atBottom: (window.innerHeight + after) >= (doc.scrollHeight - 4), height: doc.scrollHeight};
    })();`, 4*time.Second)
	if err != nil {
		return st, err
	}
	if err := json.Unmarshal([]byte(raw), &st); err != nil {
		return st, fmt.Errorf("解析滚动状态失败: %w", err)
	}
	return st, nil
}

// scrollToTop 回到页面顶部
func scrollToTop(sess *BrowserClient.WebViewSession) error {
	_, err := sess.EvalJSON(`window.scrollTo(0, 0); return JSON.stringify({y: window.scrollY});`, 4*time.Second)
	return err
}

// SearchHit 搜索结果条目
type SearchHit struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet,omitempty"`
}

// PageExtract 页面元素提取结果
type PageExtract struct {
	Kind    string      `json:"kind"`
	URL     string      `json:"url"`
	Title   string      `json:"title"`
	Results []SearchHit `json:"results"`
	Links   []SearchHit `json:"links"`
	Text    string      `json:"text"`
}

// extractPage 元素识别：必应结果页提取 li.b_algo；普通页提取正文与链接
func extractPage(sess *BrowserClient.WebViewSession, maxResults int) (PageExtract, error) {
	page := PageExtract{}
	raw, err := sess.EvalJSON(extractScript(maxResults), 8*time.Second)
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

func extractScript(maxResults int) string {
	return fmt.Sprintf(`return (function(){
    var out = { kind: 'page', url: location.href, title: document.title, results: [], links: [], text: '' };
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
    if (out.kind === 'page') {
        out.text = (document.body ? (document.body.innerText || '') : '').replace(/[\t ]+/g, ' ').replace(/\n{3,}/g, '\n\n').slice(0, 6000);
        var anchors = document.querySelectorAll('a[href]');
        for (var i = 0; i < anchors.length && out.links.length < 25; i++) {
            var a = anchors[i];
            var h = a.href;
            if (!/^https?:/.test(h) || seen[h]) continue;
            var t = (a.textContent || '').trim().replace(/\s+/g, ' ');
            if (t.length > 8 && t.length < 120) { seen[h] = 1; out.links.push({ title: t, url: h }); }
        }
    }
    return out;
})()`, maxResults)
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
