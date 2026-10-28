package BilibiliViewer

// 生活链 · B 站真实浏览器版：webview 会话、登录、选片与页面读取。
//
// 复用 WebSearch 的 browser.WebViewSession：真窗口 + 真实 B 站页面，
// Go 通过 EvalJSON 执行页面脚本。登录二维码由 B 站页面自己渲染，用户直接扫。

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"LunarSubsystem/LunarHarness/engine/browser"
	"LunarSubsystem/LunarHarness/engine/console/logger"
)

const (
	biliHome     = "https://www.bilibili.com/"
	biliLoginURL = "https://passport.bilibili.com/login"
	// 登录检测间隔与总超时：用户要求 10 秒确认一次、60 秒放弃
	loginPollInterval = 10 * time.Second
	loginTimeout      = 60 * time.Second
)

// testLoginWait 仅供手动测试覆盖；生产默认仍为 60 秒。
var testLoginWait = loginTimeout

type webSession struct {
	sess *browser.WebViewSession
}

func openBilibiliBrowser() (*webSession, error) {
	sess, err := browser.OpenWebViewSession(biliHome, "哔哩观影", 1280, 900)
	if err != nil {
		return nil, fmt.Errorf("打开浏览器失败: %w", err)
	}
	w := &webSession{sess: sess}
	// 首页可能很快加载完；给页面一点渲染时间
	w.wait(2500 * time.Millisecond)
	return w, nil
}

func (w *webSession) close() {
	if w != nil && w.sess != nil {
		w.sess.Close()
	}
}

func (w *webSession) alive() bool {
	return w != nil && w.sess != nil && w.sess.IsAlive()
}

func (w *webSession) wait(d time.Duration) {
	time.Sleep(d)
}

func (w *webSession) eval(script string, timeout time.Duration) (string, error) {
	if !w.alive() {
		return "", fmt.Errorf("浏览器已关闭")
	}
	return w.sess.EvalJSON(script, timeout)
}

func (w *webSession) navigate(target string, timeout time.Duration) error {
	if !w.alive() {
		return fmt.Errorf("浏览器已关闭")
	}
	prev := ""
	if st, err := w.sess.Snapshot(); err == nil {
		prev = st.URL
	}
	if err := w.sess.Navigate(target); err != nil {
		return err
	}
	if _, err := w.sess.WaitReady(prev, timeout); err != nil {
		logger.Warn("BilibiliViewer", "页面就绪等待超时（继续）: %v", err)
	}
	return nil
}

type pageLoginState struct {
	LoggedIn bool   `json:"loggedIn"`
	HasLogin bool   `json:"hasLogin"`
	URL      string `json:"url"`
	Title    string `json:"title"`
}

func (w *webSession) loginState() (pageLoginState, error) {
	raw, err := w.eval(`return (function(){
		var avatar = document.querySelector('.header-avatar-wrap, .bili-avatar, .header-entry-avatar, .avatar');
		var login = null;
		var all = document.querySelectorAll('a,button,div,span');
		for (var i=0;i<all.length;i++) {
			var e = all[i];
			if (!e || e.offsetParent === null) continue;
			var t = (e.textContent || '').trim();
			if (t === '登录' || t === '立即登录') { login = e; break; }
		}
		var hasUID = document.cookie.indexOf('DedeUserID=') >= 0;
		return {
			loggedIn: !!avatar && !login,
			hasLogin: !!login,
			url: location.href,
			title: document.title,
			cookieLogin: hasUID
		};
	})();`, 8*time.Second)
	if err != nil {
		return pageLoginState{}, err
	}
	var state pageLoginState
	_ = json.Unmarshal([]byte(raw), &state)
	// cookie 兜底：有些页面不渲染头像但会话已经生效
	if !state.LoggedIn && strings.Contains(raw, `"cookieLogin":true`) {
		state.LoggedIn = true
	}
	return state, nil
}

// clickLogin 点击页面上的“登录”；找不到时直接打开 passport 登录页。
func (w *webSession) clickLogin() error {
	raw, err := w.eval(`return (function(){
		var all = document.querySelectorAll('a,button,div,span');
		for (var i=0;i<all.length;i++) {
			var e = all[i];
			if (!e || e.offsetParent === null) continue;
			var t = (e.textContent || '').trim();
			if (t === '登录' || t === '立即登录') {
				e.click();
				return 'clicked';
			}
		}
		location.href = 'https://passport.bilibili.com/login';
		return 'navigated';
	})();`, 8*time.Second)
	if err != nil {
		return err
	}
	logger.Info("BilibiliViewer", "B站登录入口: %s", strings.TrimSpace(raw))
	w.wait(2 * time.Second)
	return nil
}

// ensureLogin：未登录时进入登录页，10 秒一次检测，60 秒未登录则放弃。
func (w *webSession) ensureLogin() error {
	state, err := w.loginState()
	if err == nil && state.LoggedIn {
		logger.Info("BilibiliViewer", "B站已登录，跳过扫码")
		return nil
	}
	if err := w.clickLogin(); err != nil {
		return fmt.Errorf("打开登录入口失败: %w", err)
	}
	w.wait(2 * time.Second)
	// 点击登录有时只是弹遮罩，不一定把二维码露出来；
	// 如果 URL 没变、仍未登录，直接进 passport 登录页，保证二维码可见。
	if state, err := w.loginState(); err != nil || (!state.LoggedIn && !strings.Contains(state.URL, "passport.bilibili.com")) {
		if err := w.navigate(biliLoginURL, 25*time.Second); err != nil {
			return fmt.Errorf("打开 B 站登录页失败: %w", err)
		}
		w.wait(2 * time.Second)
	}
	deadline := time.Now().Add(testLoginWait)
	for time.Now().Before(deadline) {
		w.wait(loginPollInterval)
		if !w.alive() {
			return fmt.Errorf("登录等待期间浏览器被关闭")
		}
		state, err := w.loginState()
		if err == nil && state.LoggedIn {
			logger.Info("BilibiliViewer", "B站登录成功，继续刷视频")
			// 登录页可能停在 passport；回首页并等渲染
			_ = w.navigate(biliHome, 25*time.Second)
			w.wait(2 * time.Second)
			return nil
		}
		logger.Info("BilibiliViewer", "等待扫码登录…（剩余 %s）", time.Until(deadline).Truncate(time.Second))
	}
	return fmt.Errorf("60 秒内未完成 B 站登录，放弃本次刷视频")
}

// collectVideoLinks 从当前页面收集视频卡片（首页/搜索结果页通用）。
func (w *webSession) collectVideoLinks(max int) ([]videoLink, error) {
	if max <= 0 {
		max = 40
	}
	raw, err := w.eval(fmt.Sprintf(`return (function(){
		var out = [];
		var seen = {};
		function cleanTitle(card, a) {
			var selectors = ['.bili-video-card__info--tit', '.bili-video-card__info--tit a', '.video-card__info--tit', '.title', 'h3'];
			for (var s=0;s<selectors.length;s++) {
				var el = card.querySelector(selectors[s]);
				if (!el) continue;
				var t = (el.getAttribute('title') || el.textContent || '').trim().replace(/\s+/g,' ');
				if (t.length >= 4 && t.indexOf('添加至稍后再看') < 0) return t;
			}
			var fallback = (a.getAttribute('title') || a.textContent || '').trim().replace(/\s+/g,' ');
			fallback = fallback.replace('添加至稍后再看','').trim();
			if (fallback.length < 4) fallback = (card.textContent || '').trim().replace(/\s+/g,' ').replace('添加至稍后再看','').trim();
			return fallback;
		}
		var as = document.querySelectorAll('a[href*="/video/BV"]');
		for (var i=0;i<as.length;i++) {
			var a = as[i];
			if (!a || !a.href) continue;
			var key = a.href.split('?')[0];
			if (seen[key]) continue;
			if (a.closest('.ad, .bili-video-card__info--ad')) continue;
			var card = a.closest('.bili-video-card, .video-card, li, .feed-card') || a;
			var title = cleanTitle(card, a);
			if (!title || title.length < 4) continue;
			// 纯数据行（播放量/时长）不是标题
			if (/^[\d\.万亿]+[\s]*[\d\.万亿]*[\s]*[\d:]+$/.test(title)) continue;
			seen[key] = 1;
			out.push({href:key, title:title.slice(0,180)});
			if (out.length >= %d) break;
		}
		return out;
	})();`, max), 10*time.Second)
	if err != nil {
		return nil, err
	}
	var links []videoLink
	if err := json.Unmarshal([]byte(raw), &links); err != nil {
		return nil, err
	}
	return links, nil
}

// filterAndScore 给候选标题打分：本轮关键词与配置默认偏好共同参与，
// 本轮关键词权重更高，但不再硬过滤——预选需要凑够 3 倍候选，过滤过严会直接断粮。
func filterAndScore(links []videoLink, cfg Config) []videoLink {
	out := make([]videoLink, 0, len(links))
	for _, link := range links {
		link.Score = titleScore(link.Title, cfg)
		out = append(out, link)
	}
	return out
}

// titleScore 标题得分：本轮关键词权重为 2 份，配置默认词 1 份。
func titleScore(title string, cfg Config) int {
	score := 0
	if len(cfg.DesireKeywords) > 0 {
		score += 2 * scoreOf(title, cfg.DesireKeywords)
	}
	score += scoreOf(title, cfg.PreferenceKeywords)
	return score
}

// ==== 选片打分参数 ====

const (
	// searchKeywordTries 首页没有参考词命中时，随机挑几个参考词去搜索
	searchKeywordTries = 3
	// tagScoreLimit 最多为多少条候选补拉标签参与打分（控制请求量）
	tagScoreLimit = 12
	// tagFetchWorkers 标签请求并发数
	tagFetchWorkers = 5
)

// selectVideo 选片：标题 + 标签打分，并跳过保留期内看过的视频。
//
// 流程：首页候选（标题打分 → 标签补分 → 剔除已看）→ 没有命中就随机挑几个参考词搜索，
// 搜索页候选走同一套流程 → 在得分最高的一组里随机，避免每天都是同一条。
func (w *webSession) selectVideo(cfg Config, seen map[string]bool) (videoLink, error) {
	home, err := w.collectVideoLinks(45)
	if err != nil {
		return videoLink{}, fmt.Errorf("读取首页视频卡片失败: %w", err)
	}
	home = prepareCandidates(home, cfg, seen)
	if bestScore(home) == 0 {
		// 优先按本轮关键词搜索；没有本轮关键词才使用配置默认偏好
		keywords := cfg.PreferenceKeywords
		if len(cfg.DesireKeywords) > 0 {
			keywords = cfg.DesireKeywords
		}
		for _, keyword := range pickSearchKeywords(keywords, searchKeywordTries) {
			searchURL := "https://search.bilibili.com/all?keyword=" + url.QueryEscape(keyword)
			logger.Info("BilibiliViewer", "首页没有参考词命中，搜索: %s", keyword)
			if err := w.navigate(searchURL, 25*time.Second); err != nil {
				continue
			}
			w.wait(2 * time.Second)
			found, ferr := w.collectVideoLinks(30)
			if ferr != nil {
				continue
			}
			home = append(home, prepareCandidates(found, cfg, seen)...)
			if bestScore(home) > 0 {
				break
			}
		}
	}
	// 过滤 + 已看去重后可能全空；再回首页触发一次懒加载后重试
	if len(home) == 0 {
		_ = w.navigate(biliHome, 25*time.Second)
		w.wait(2 * time.Second)
		if links, err := w.collectVideoLinks(45); err == nil {
			home = prepareCandidates(links, cfg, seen)
		}
	}
	if len(home) == 0 {
		if len(cfg.DesireKeywords) > 0 {
			return videoLink{}, fmt.Errorf("没有找到符合本轮要求（%s）的视频",
				strings.Join(cfg.DesireKeywords, "、"))
		}
		return videoLink{}, fmt.Errorf("没有可看的视频（30 天已看记录可能把候选全部排除了）")
	}

	sort.SliceStable(home, func(i, j int) bool { return home[i].Score > home[j].Score })
	top := home[0].Score
	pool := make([]videoLink, 0, 8)
	for _, link := range home {
		if link.Score < top {
			break
		}
		pool = append(pool, link)
		if len(pool) >= 8 {
			break
		}
	}
	chosen := pool[rand.Intn(len(pool))]
	if err := w.navigate(chosen.Href, 30*time.Second); err != nil {
		return videoLink{}, fmt.Errorf("打开视频详情页失败: %w", err)
	}
	w.wait(2 * time.Second)
	logger.Info("BilibiliViewer", "选中视频（参考词得分 %d）: %s | %s", chosen.Score, chosen.Title, chosen.Href)
	return chosen, nil
}

// prepareCandidates 候选统一处理：过滤 → 标题打分 → 标签补分 → 剔除已看
func prepareCandidates(links []videoLink, cfg Config, seen map[string]bool) []videoLink {
	out := filterAndScore(links, cfg)
	out = scoreWithTags(out, cfg)
	return dropSeen(out, seen)
}

// scoreWithTags 为前 tagScoreLimit 条候选补拉内容标签，命中参考词同样加分。
//
// 首页卡片的 DOM 只有标题，没有内容标签；标签要走视频标签接口。为了让「标签」真正
// 参与打分又不打爆请求量，只取前若干条候选、并发拉取；失败项按 0 分处理。
func scoreWithTags(links []videoLink, cfg Config) []videoLink {
	if len(cfg.PreferenceKeywords) == 0 && len(cfg.DesireKeywords) == 0 {
		return links
	}
	if len(links) == 0 {
		return links
	}
	limit := len(links)
	if limit > tagScoreLimit {
		limit = tagScoreLimit
	}
	type scored struct{ idx, score int }
	results := make(chan scored, limit)
	var wg sync.WaitGroup
	sem := make(chan struct{}, tagFetchWorkers)
	for i := 0; i < limit; i++ {
		bvid := bvidOf(links[i].Href)
		if bvid == "" {
			continue
		}
		wg.Add(1)
		go func(idx int, bvid string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			tags, err := fetchVideoTags(bvid)
			if err != nil || len(tags) == 0 {
				return
			}
			results <- scored{idx: idx, score: titleScore(strings.Join(tags, " "), cfg)}
		}(i, bvid)
	}
	go func() {
		wg.Wait()
		close(results)
	}()
	for item := range results {
		links[item.idx].Score += item.score
	}
	return links
}

// dropSeen 剔除保留期内看过的视频
func dropSeen(links []videoLink, seen map[string]bool) []videoLink {
	if len(seen) == 0 {
		return links
	}
	out := make([]videoLink, 0, len(links))
	for _, link := range links {
		if bvid := bvidOf(link.Href); bvid != "" && seen[bvid] {
			continue
		}
		out = append(out, link)
	}
	return out
}

// pickSearchKeywords 随机挑 n 个参考词（去空白、去重），避免每轮都只搜同一批词
func pickSearchKeywords(keywords []string, n int) []string {
	pool := make([]string, 0, len(keywords))
	seen := make(map[string]bool, len(keywords))
	for _, raw := range keywords {
		word := strings.TrimSpace(raw)
		if word == "" || seen[word] {
			continue
		}
		seen[word] = true
		pool = append(pool, word)
	}
	rand.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
	if n <= 0 || n > len(pool) {
		n = len(pool)
	}
	return pool[:n]
}

func bestScore(links []videoLink) int {
	best := 0
	for _, link := range links {
		if link.Score > best {
			best = link.Score
		}
	}
	return best
}

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
