package WebSearch

// scroll.go 页面滚动与正文提取（基于 browser.WebViewSession）
//
// 统一流程：页面复位到顶部后，每步按视口高度的 150% 快速下移（中途只滚动不提取，
// 文本反正以最终快照为准）；走查结束后统一等待懒加载窗口，再做一次带正文提取的
// 最终快照——触底时纳入懒加载尾部内容，非触底收尾时也能拿到当前位置的正文。

import (
	"LunarSubsystem/LunarHarness/engine/browser"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"encoding/json"
	"fmt"
	"time"
)

// bottomSettleMS 走查结束后的懒加载等待毫秒数：给「滚动到位后才触发加载」的尾部
// 内容留出进入文档的时间，随后一次性提取最终快照。
const bottomSettleMS = 600

// navTo 导航并等待就绪 + 渲染稳定。仅导航调用本身失败才返回错误；
// 就绪等待超时仅记日志（后续提取自行决定成败）。
func navTo(sess *browser.WebViewSession, target string, readyTimeout, settle time.Duration) error {
	prev := ""
	if st, err := sess.Snapshot(); err == nil {
		prev = st.URL
	}
	if err := sess.Navigate(target); err != nil {
		return err
	}
	if _, err := sess.WaitReady(prev, readyTimeout); err != nil {
		logger.SubWarn("CrystalAstral", "WebSearch", "页面就绪等待超时（继续尝试提取）: %v", err)
	}
	time.Sleep(settle)
	return nil
}

// ensurePageTop 把页面滚动位置复位到顶部（关闭平滑滚动，保证后续偏移可精确计算）
func ensurePageTop(sess *browser.WebViewSession) error {
	_, err := sess.EvalJSON(`return (function(){
        var el = (window.__lunarScroller && window.__lunarScroller.isConnected)
            ? window.__lunarScroller
            : (document.scrollingElement || document.documentElement);
        try { document.documentElement.style.scrollBehavior = 'auto'; } catch(e) {}
        try { document.body.style.scrollBehavior = 'auto'; } catch(e) {}
        if (el) el.scrollTop = 0;
        window.scrollTo(0, 0);
        return {y: el ? el.scrollTop : 0};
    })();`, 4*time.Second)
	return err
}

// scrollStep 向下滚动约一屏半（150% 视口），返回滚动状态；extract 为 true 时
// 同时提取整页正文（innerText 开销大，仅在最终快照时使用）。
// 自动识别真正承载滚动的元素（普通文档或 SPA 内部滚动容器），并缓存到页面供后续步骤复用。
func scrollStep(sess *browser.WebViewSession, textMax int, extract bool) (stepState, error) {
	var st stepState
	if textMax <= 0 {
		textMax = 24000
	}
	raw, err := sess.EvalJSON(scrollStepScript(textMax, extract), 8*time.Second)
	if err != nil {
		return st, err
	}
	if err := json.Unmarshal([]byte(raw), &st); err != nil {
		return st, fmt.Errorf("解析滚动状态失败: %w", err)
	}
	return st, nil
}

// scrollStepScript 滚动 + 可选的正文提取脚本
func scrollStepScript(textMax int, extract bool) string {
	return fmt.Sprintf(`return (function(){
    function pickScroller() {
        var doc = document.scrollingElement || document.documentElement;
        var best = doc;
        var bestDiff = doc ? (doc.scrollHeight - doc.clientHeight) : 0;
        var nodes = document.querySelectorAll('body *');
        for (var i = 0; i < nodes.length && i < 1500; i++) {
            var n = nodes[i];
            var style = window.getComputedStyle(n);
            if (!style || !/(auto|scroll|overlay)/.test(style.overflowY)) continue;
            if (n.clientHeight < 200) continue;
            var diff = n.scrollHeight - n.clientHeight;
            if (diff > bestDiff) { best = n; bestDiff = diff; }
        }
        return best || doc;
    }
    var el = (window.__lunarScroller && window.__lunarScroller.isConnected)
        ? window.__lunarScroller
        : pickScroller();
    window.__lunarScroller = el;
    try { document.documentElement.style.scrollBehavior = 'auto'; } catch(e) {}
    try { document.body.style.scrollBehavior = 'auto'; } catch(e) {}
    var before = el ? el.scrollTop : 0;
    var viewport = el ? (el.clientHeight || window.innerHeight) : (window.innerHeight || 0);
    if (el) {
        var step = Math.max(300, Math.round(viewport * 1.5));
        el.scrollTop = Math.min(before + step, el.scrollHeight);
    }
    var after = el ? el.scrollTop : 0;
    var height = el ? el.scrollHeight : 0;
    var text = '';
    if (%t) {
        var root = document.querySelector('article') || document.querySelector('main') || document.querySelector('[role="main"]') || document.body;
        text = root ? (root.innerText || root.textContent || '') : '';
        text = text.replace(/\u00a0/g, ' ').replace(/[ \t]+/g, ' ').replace(/\n{3,}/g, '\n\n').trim();
    }
    return {
        before: before,
        after: after,
        atBottom: el ? ((after + viewport) >= (height - 4)) : true,
        height: height,
        viewport: viewport,
        url: location.href,
        title: document.title,
        textChars: text.length,
        text: text.slice(0, %d)
    };
})();`, extract, textMax)
}

// walkPage 从页面顶部滚动到底部快速走查（中途只滚动不提取，单页正文按 textMax
// 配额截断——总提取上限由所有参与页面均分后的单页额度）；onText 非空时把最终
// 快照正文交给回调。走查结束后等待懒加载窗口，再做一次带提取的最终快照。
func walkPage(sess *browser.WebViewSession, textMax int, cfg Config, deadline time.Time, onText func(string)) walkResult {
	res := walkResult{}
	if err := ensurePageTop(sess); err != nil {
		logger.SubWarn("CrystalAstral", "WebSearch", "页面复位到顶部失败（从当前位置开始滚动）: %v", err)
	}
	maxSteps := cfg.ScrollMaxSteps
	if maxSteps <= 0 {
		maxSteps = 1
	}
	settle := time.Duration(cfg.ScrollSettleMS) * time.Millisecond

	for i := 0; i < maxSteps; i++ {
		if !sess.IsAlive() || time.Now().After(deadline) {
			break
		}
		st, err := scrollStep(sess, textMax, false)
		if err != nil {
			logger.SubWarn("CrystalAstral", "WebSearch", "页面滚动失败（停止走查）: %v", err)
			break
		}
		res.Steps = i + 1
		res.Height = st.Height
		res.Viewport = st.Viewport
		res.URL = st.URL
		res.Title = st.Title
		time.Sleep(settle)
		if st.AtBottom {
			res.AtBottom = true
			break
		}
		if st.After <= st.Before {
			// 无法继续下移：多为内部滚动容器或弹窗，按当前已到达位置收尾
			res.AtBottom = true
			break
		}
	}

	// 最终快照：等待懒加载窗口后带正文提取一次（触底纳入尾部内容；
	// 非触底收尾——超时/步数用尽/瞬时失败——也能拿到当前位置的正文）
	if sess.IsAlive() && !time.Now().After(deadline) {
		time.Sleep(time.Duration(bottomSettleMS) * time.Millisecond)
		if final, err := scrollStep(sess, textMax, true); err == nil {
			res = applyStep(res, final)
			if onText != nil && final.Text != "" {
				onText(final.Text)
			}
		}
	}
	return res
}

// applyStep 把单步状态合并进走查结果
func applyStep(res walkResult, st stepState) walkResult {
	res.Height = st.Height
	res.Viewport = st.Viewport
	res.URL = st.URL
	res.Title = st.Title
	res.Text = st.Text
	res.TextChars = st.TextChars
	return res
}
