package viewer

// 生活链 · B 站真实浏览器版：webview 会话、登录、选片与页面读取。
//
// 复用 WebSearch 的 browser.WebViewSession：真窗口 + 真实 B 站页面，
// Go 通过 EvalJSON 执行页面脚本。登录二维码由 B 站页面自己渲染，用户直接扫。

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
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
)

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
