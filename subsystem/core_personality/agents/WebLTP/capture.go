package WebLTP

// Web-LTP 截图与页面滚动（基于 BrowserClient.WebViewSession）
//
// 滚动流程统一为「从页面顶部走到底部」：每步按视口高度的 85% 下移，
// 留出重叠区避免切断段落；滚动后等待页面稳定与懒加载长高，触底或停滞即止。
// 截图不是必经步骤：只有页面正文很少（VisionMinChars 以下）时才按策略采集，
// 其余场合只做文本提取，避免无意义的截图与视觉模型开销。

import (
	"CorePersonality/engine/multimodal"
	"LunarSubsystem/BrowserClient"
	"bytes"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"image/jpeg"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// shotCapture 会话窗口截图：统一走 engine 主线的窗口截图机制
// （PrintWindow 内容优先、纯色/失败时自动回退屏幕客户区），这里只做 JPEG 编码。
func shotCapture(sess *BrowserClient.WebViewSession, quality int) ([]byte, error) {
	if quality <= 0 || quality > 100 {
		quality = 85
	}
	hwnd, err := sess.WindowHandle()
	if err != nil {
		return nil, fmt.Errorf("会话窗口截图失败: %w", err)
	}
	img, err := multimodal.CaptureWindowRGBA(hwnd, true)
	if err != nil {
		return nil, fmt.Errorf("会话窗口截图失败: %w", err)
	}
	buf := &bytes.Buffer{}
	if err := jpeg.Encode(buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return nil, fmt.Errorf("截图编码失败: %w", err)
	}
	return buf.Bytes(), nil
}

// save 保存一张截图（按顺序编号命名），返回序号
func (s *shotSaver) save(jpg []byte, label string) int {
	s.count++
	if !s.enabled {
		return s.count
	}
	name := fmt.Sprintf("%02d-%s.jpg", s.count, sanitizeFileName(label))
	path := filepath.Join(s.dir, name)
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		LoggerGeneralWarn("截图目录创建失败 %s: %v", s.dir, err)
		return s.count
	}
	if err := os.WriteFile(path, jpg, 0o644); err != nil {
		LoggerGeneralWarn("截图保存失败 %s: %v", path, err)
		return s.count
	}
	s.files = append(s.files, path)
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

// probePage 滚动前探测页面体量与视觉元素，作为「是否需要视觉截图」的依据。
// 正文优先取 article/main/[role=main]，缺失时回退 body；
// 同时识别真正承载滚动的元素（普通文档或 SPA 内部滚动容器），缓存到页面供后续滚动复用。
func probePage(sess *BrowserClient.WebViewSession) (pageProbe, error) {
	var probe pageProbe
	raw, err := sess.EvalJSON(`return (function(){
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
        var el = pickScroller();
        window.__lunarScroller = el;
        var root = document.querySelector('article') || document.querySelector('main') || document.querySelector('[role="main"]') || document.body;
        var text = root ? (root.innerText || root.textContent || '') : '';
        text = text.replace(/\u00a0/g, ' ').replace(/[ \t]+/g, ' ').replace(/\n{3,}/g, '\n\n').trim();
        return {
            textChars: text.length,
            images: document.images ? document.images.length : 0,
            visuals: document.querySelectorAll('canvas,svg,video,figure').length,
            height: el ? el.scrollHeight : 0,
            viewport: el ? el.clientHeight : (window.innerHeight || 0)
        };
    })();`, 8*time.Second)
	if err != nil {
		return probe, err
	}
	if err := json.Unmarshal([]byte(raw), &probe); err != nil {
		return probe, fmt.Errorf("解析页面探测结果失败: %w", err)
	}
	return probe, nil
}

// ensurePageTop 把页面滚动位置复位到顶部（关闭平滑滚动，保证后续偏移可精确计算）
func ensurePageTop(sess *BrowserClient.WebViewSession) error {
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

// scrollByViewport 向下滚动约一屏（85% 视口），返回滚动前后位置与是否触底。
// 优先使用 probePage 识别出的滚动容器，兼容 SPA 内部滚动区域。
func scrollByViewport(sess *BrowserClient.WebViewSession) (scrollState, error) {
	var st scrollState
	raw, err := sess.EvalJSON(`return (function(){
        var el = (window.__lunarScroller && window.__lunarScroller.isConnected)
            ? window.__lunarScroller
            : (document.scrollingElement || document.documentElement);
        if (!el) return {before: 0, after: 0, atBottom: true, height: 0};
        try { document.documentElement.style.scrollBehavior = 'auto'; } catch(e) {}
        try { document.body.style.scrollBehavior = 'auto'; } catch(e) {}
        var before = el.scrollTop;
        var viewport = el.clientHeight || window.innerHeight;
        var step = Math.max(200, Math.round(viewport * 0.85));
        el.scrollTop = Math.min(before + step, el.scrollHeight);
        var after = el.scrollTop;
        return {
            before: before,
            after: after,
            atBottom: (after + viewport) >= (el.scrollHeight - 4),
            height: el.scrollHeight
        };
    })();`, 4*time.Second)
	if err != nil {
		return st, err
	}
	if err := json.Unmarshal([]byte(raw), &st); err != nil {
		return st, fmt.Errorf("解析滚动状态失败: %w", err)
	}
	return st, nil
}

// walkPage 从页面顶部滚动到底部；按策略顺带采集截图。
//
// 截图策略由 capturePolicy 决定：页面正文足够多时 Max=0，只滚动不截图；
// 正文很少时按 Max 采集，并用 JPEG 指纹跳过完全相同的重复帧（固定悬浮层等）。
// 每步滚动后等待 settle，让懒加载内容补进文档；页面高度增长会继续走查。
func walkPage(sess *BrowserClient.WebViewSession, probe pageProbe, maxSteps int, settle time.Duration,
	policy capturePolicy, label string, saver *shotSaver, deadline time.Time) walkResult {
	res := walkResult{Height: probe.Height, Viewport: probe.Viewport}
	if maxSteps <= 0 {
		maxSteps = 1
	}
	if err := ensurePageTop(sess); err != nil {
		return res
	}

	lastHash := uint64(0)
	capture := func() {
		if !policy.Enabled || len(res.Frames) >= policy.Max {
			return
		}
		jpg, err := shotCapture(sess, 85)
		if err != nil {
			LoggerGeneralWarn("页面截图失败 [%s]: %v", label, err)
			return
		}
		h := fnv.New64a()
		_, _ = h.Write(jpg)
		sum := h.Sum64()
		if sum == lastHash {
			return
		}
		lastHash = sum
		frameLabel := fmt.Sprintf("%s-滚动%02d", label, len(res.Frames)+1)
		saver.save(jpg, frameLabel)
		res.Frames = append(res.Frames, shotFrame{JPEG: jpg, Label: frameLabel})
	}

	for i := 0; i < maxSteps; i++ {
		if !sess.IsAlive() || time.Now().After(deadline) {
			break
		}
		capture()
		st, err := scrollByViewport(sess)
		if err != nil {
			break
		}
		res.Steps = i + 1
		res.Height = st.Height
		time.Sleep(settle)
		if st.AtBottom {
			res.AtBottom = true
			capture() // 底部最后一屏也纳入视觉理解
			break
		}
		if st.After <= st.Before {
			// 无法继续下移：多为内部滚动容器或弹窗，按当前已到达位置收尾
			break
		}
	}
	return res
}

// decideCapturePolicy 根据页面正文体量决定是否启用滚动截图：
// 正文达到 VisionMinChars 就完全依赖文本，视觉只在「字很少」时兜底。
func decideCapturePolicy(probe pageProbe, visionMinChars, maxShots int) capturePolicy {
	if visionMinChars <= 0 {
		visionMinChars = 240
	}
	if probe.TextChars >= visionMinChars {
		return capturePolicy{Enabled: false}
	}
	if maxShots <= 0 {
		maxShots = 3
	}
	return capturePolicy{Enabled: true, Max: maxShots}
}
