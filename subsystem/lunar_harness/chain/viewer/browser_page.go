package viewer

// browser_page.go 页面上下文里的评论读取与接口代发。
//
// 这些请求都在 B 站页面内用 fetch 发出，自动携带登录 cookie，
// 且不依赖 Go 侧读取 HttpOnly cookie。

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// scrollToComments 把页面滚动到评论区；用户明确要求读评论前滚到对应位置。
func (w *webSession) scrollToComments() {
	_, err := w.eval(`return (function(){
		var el = document.querySelector('#comment, .comment-container, .reply-list, .comment-list, .bili-comment');
		if (el) { el.scrollIntoView({block:'start'}); window.scrollBy(0, -80); return 'comment'; }
		window.scrollTo(0, document.body.scrollHeight * 0.55);
		return 'page';
	})();`, 8*time.Second)
	if err != nil {
		logger.Warn("BilibiliViewer", "滚动到评论区失败: %v", err)
	}
	w.wait(1500 * time.Millisecond)
}

// readComments 从页面 DOM 读评论（最多 15 条）；失败时回退到页面文本行。
func (w *webSession) readComments(max int) ([]commentItem, error) {
	w.scrollToComments()
	raw, err := w.eval(fmt.Sprintf(`return (function(){
		var out = [];
		var nodes = document.querySelectorAll('.reply-item, .comment-item, .sub-reply-item, .bili-comment-item');
		for (var i=0;i<nodes.length && out.length < %d;i++) {
			var n = nodes[i];
			var userEl = n.querySelector('.user-name, .reply-item__name, .name, .nickname');
			var textEl = n.querySelector('.reply-content, .comment-content, .content, .reply-item__content');
			var user = userEl ? (userEl.textContent || '').trim() : '';
			var text = textEl ? (textEl.textContent || '').trim() : '';
			if (!text) continue;
			var likeEl = n.querySelector('.like-count, .reply-like, .count');
			var like = likeEl ? parseInt((likeEl.textContent||'0').replace(/\D/g,''),10) || 0 : 0;
			out.push({user:user.slice(0,40), content:text.slice(0,500), like:like});
		}
		return out;
	})();`, max), 10*time.Second)
	if err != nil {
		return nil, err
	}
	var items []commentItem
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil, err
	}
	return items, nil
}

var csrfRe = regexp.MustCompile(`(?:^|;\s*)bili_jct=([^;]+)`)

func (w *webSession) csrf() string {
	raw, err := w.eval(`return document.cookie;`, 4*time.Second)
	if err != nil {
		return ""
	}
	var cookie string
	if err := json.Unmarshal([]byte(raw), &cookie); err != nil {
		cookie = raw
	}
	m := csrfRe.FindStringSubmatch(cookie)
	if len(m) < 2 {
		return ""
	}
	v, _ := url.QueryUnescape(m[1])
	return v
}

// pageFetchForm 在页面上下文里用 FormData 发 POST（B 站动态接口只认表单）。
func (w *webSession) pageFetchForm(target string, fields map[string]string, timeout time.Duration) (string, error) {
	if timeout <= 0 {
		timeout = 25 * time.Second
	}
	key := fmt.Sprintf("__lunarForm_%d", time.Now().UnixNano())
	targetJSON, _ := json.Marshal(target)
	fieldsJSON, _ := json.Marshal(fields)
	keyJSON, _ := json.Marshal(key)
	script := fmt.Sprintf(`(function(){
		window[%s] = null;
		var fd = new FormData();
		var fields = %s;
		Object.keys(fields).forEach(function(k){ if (fields[k] !== '') fd.append(k, fields[k]); });
		fetch(%s, {method:'POST', credentials:'include', body:fd})
			.then(function(r){ return r.text(); })
			.then(function(t){ window[%s] = t; })
			.catch(function(e){ window[%s] = JSON.stringify({__error:String(e)}); });
		return true;
	})();`, keyJSON, fieldsJSON, targetJSON, keyJSON, keyJSON)
	if _, err := w.eval(script, 8*time.Second); err != nil {
		return "", err
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		w.wait(300 * time.Millisecond)
		raw, err := w.eval(fmt.Sprintf(`return window[%s];`, keyJSON), 5*time.Second)
		if err != nil || strings.TrimSpace(raw) == "" || strings.TrimSpace(raw) == "null" {
			continue
		}
		var text string
		if err := json.Unmarshal([]byte(raw), &text); err != nil {
			return raw, nil
		}
		return text, nil
	}
	return "", fmt.Errorf("页面表单请求超时: %s", target)
}

// pageFetchText 在页面上下文里发异步请求，把响应文本写回 window，再轮询取回。
// 这样请求自动携带 B 站登录 cookie，且不依赖 Go 侧读取 HttpOnly cookie。
func (w *webSession) pageFetchText(method, target, contentType, body string, timeout time.Duration) (string, error) {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	key := fmt.Sprintf("__lunarFetch_%d", time.Now().UnixNano())
	methodJSON, _ := json.Marshal(method)
	targetJSON, _ := json.Marshal(target)
	ctypeJSON, _ := json.Marshal(contentType)
	bodyJSON, _ := json.Marshal(body)
	keyJSON, _ := json.Marshal(key)
	script := fmt.Sprintf(`(function(){
		window[%s] = null;
		var headers = {};
		if (%s) headers['Content-Type'] = %s;
		var opt = {method:%s, credentials:'include', headers:headers};
		if (%s) opt.body = %s;
		fetch(%s, opt)
			.then(function(r){ return r.text(); })
			.then(function(t){ window[%s] = t; })
			.catch(function(e){ window[%s] = JSON.stringify({__error:String(e)}); });
		return true;
	})();`, keyJSON, ctypeJSON, ctypeJSON, methodJSON, bodyJSON, bodyJSON, targetJSON, keyJSON, keyJSON)
	if _, err := w.eval(script, 8*time.Second); err != nil {
		return "", err
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		w.wait(300 * time.Millisecond)
		raw, err := w.eval(fmt.Sprintf(`return window[%s];`, keyJSON), 5*time.Second)
		if err != nil {
			continue
		}
		if strings.TrimSpace(raw) == "" || strings.TrimSpace(raw) == "null" {
			continue
		}
		var text string
		if err := json.Unmarshal([]byte(raw), &text); err != nil {
			return raw, nil
		}
		return text, nil
	}
	return "", fmt.Errorf("页面请求超时: %s", target)
}
