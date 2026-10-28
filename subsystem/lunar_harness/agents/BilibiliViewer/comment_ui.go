package BilibiliViewer

// comment_ui.go 用真实页面操作发布评论与回复，完全走 B 站前端的交互逻辑。
//
// 为什么不直接调接口：接口要求自己拼 root / parent，一旦对参数含义理解错，
// 评论就会挂到错误位置（对方看不到）。页面操作让 B 站自己算这些参数，
// 和真人手点没有区别。
//
// 兼容两种评论区实现：
//   - 新版 web component（bili-comments 多层 shadow DOM）；
//   - 旧版 .reply-box-textarea + .reply-box-send。
//
// 任一环节失败都返回错误，由调用方回退到接口发布，保证内容不会丢。

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// commentUIHelpers 页面内工具：
//   - __lunarRoots 收集 document 与所有 shadowRoot；
//   - __lunarFindAll 穿透 shadow DOM 查找节点；
//   - __lunarText 递归取文本（自定义元素的内容在 shadow 里，textContent 取不到）；
//   - __lunarEditable / __lunarSendButton 定位当前可见的输入框与发送按钮。
const commentUIHelpers = `
function __lunarRoots() {
	var roots = [document];
	for (var i = 0; i < roots.length; i++) {
		var els;
		try { els = roots[i].querySelectorAll('*'); } catch (e) { continue; }
		for (var j = 0; j < els.length; j++) {
			var sr = els[j].shadowRoot;
			if (sr) roots.push(sr);
		}
	}
	return roots;
}
function __lunarFindAll(selector) {
	var roots = __lunarRoots();
	var out = [];
	for (var i = 0; i < roots.length; i++) {
		var list;
		try { list = roots[i].querySelectorAll(selector); } catch (e) { continue; }
		for (var j = 0; j < list.length; j++) out.push(list[j]);
	}
	return out;
}
function __lunarVisible(el) {
	if (!el) return false;
	var r = el.getBoundingClientRect();
	return r.width > 0 && r.height > 0;
}
function __lunarText(node) {
	if (!node) return '';
	var t = node.textContent || '';
	if (node.shadowRoot) t += ' ' + __lunarText(node.shadowRoot);
	var kids = node.children || [];
	for (var i = 0; i < kids.length; i++) {
		if (kids[i].shadowRoot) t += ' ' + __lunarText(kids[i].shadowRoot);
	}
	return t;
}
function __lunarEditable() {
	var sels = [
		'bili-comment-rich-textarea #textarea',
		'#comment-input .brt-editor',
		'.reply-box-textarea',
		'[contenteditable="true"]'
	];
	for (var s = 0; s < sels.length; s++) {
		var list = __lunarFindAll(sels[s]);
		var best = null;
		for (var i = 0; i < list.length; i++) {
			if (!__lunarVisible(list[i])) continue;
			best = list[i];
		}
		if (best) return best;
	}
	return null;
}
function __lunarSendButton() {
	var list = __lunarFindAll('button, .reply-box-send');
	var best = null;
	for (var i = 0; i < list.length; i++) {
		var b = list[i];
		if (!b || !__lunarVisible(b) || b.disabled) continue;
		if (b.getAttribute && b.getAttribute('aria-disabled') === 'true') continue;
		var t = (b.textContent || '').trim();
		if (t === '发布' || t === '发送' || t === '评论' || t === '回复') best = b;
	}
	return best;
}
function __lunarType(el, text) {
	el.focus();
	try {
		var sel = window.getSelection();
		var range = document.createRange();
		range.selectNodeContents(el);
		sel.removeAllRanges();
		sel.addRange(range);
	} catch (e) {}
	var ok = false;
	try { ok = document.execCommand('insertText', false, text); } catch (e) {}
	if (!ok || !(el.textContent || '').trim()) {
		el.textContent = text;
		try {
			el.dispatchEvent(new InputEvent('input', {bubbles: true, data: text, inputType: 'insertText'}));
		} catch (e) {}
	}
	return (el.textContent || '').length;
}
function __lunarReplyButton(node) {
	var hosts = [node];
	if (node.shadowRoot) hosts.push(node.shadowRoot);
	for (var h = 0; h < hosts.length; h++) {
		var root = hosts[h];
		if (!root.querySelectorAll) continue;
		var cells = root.querySelectorAll('button, a, span, div');
		for (var i = 0; i < cells.length; i++) {
			var t = (cells[i].textContent || '').trim();
			if (t === '回复' || t === '回复 ') return cells[i];
		}
	}
	return null;
}
`

// commentUIRun 在页面上下文执行一段脚本（自动带上 helpers）。
func (w *webSession) commentUIRun(script string, timeout time.Duration) (string, error) {
	return w.eval(commentUIHelpers+"\nreturn (function(){"+script+"})();", timeout)
}

// evalPageString 把 eval 返回的 JSON 值还原成普通字符串。
func evalPageString(raw string) string {
	var text string
	if err := json.Unmarshal([]byte(raw), &text); err == nil {
		return strings.TrimSpace(text)
	}
	return strings.TrimSpace(raw)
}

// waitCommentNodes 等评论区真正渲染出可定位的评论节点（新评论区是懒加载）。
func (w *webSession) waitCommentNodes(timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		raw, err := w.commentUIRun(`
			var nodes = __lunarFindAll('bili-comment-renderer, bili-comment-thread-renderer, .reply-item, .sub-reply-item');
			return nodes.length;
		`, 8*time.Second)
		if err == nil {
			var count int
			if json.Unmarshal([]byte(raw), &count) == nil && count > 0 {
				return
			}
		}
		w.wait(800 * time.Millisecond)
	}
}

// uiReplyToComment 在当前视频页找到对方的评论，点「回复」后输入并发送。
//
// targetText 是对方评论正文（用于在页面里定位），user 是对方昵称。
// 找不到时先尝试展开折叠的回复再重试一次。
func (w *webSession) uiReplyToComment(targetText, user, message string) error {
	message = strings.TrimSpace(message)
	if message == "" {
		return fmt.Errorf("回应内容为空")
	}
	w.scrollToComments()
	w.waitCommentNodes(10 * time.Second)
	textJSON, _ := json.Marshal(strings.TrimSpace(targetText))
	userJSON, _ := json.Marshal(strings.TrimSpace(user))

	locate := func() string {
		raw, err := w.commentUIRun(`
			var needle = `+string(textJSON)+`;
			var who = `+string(userJSON)+`;
			var probe = needle.length > 20 ? needle.slice(0, 20) : needle;
			var nodes = __lunarFindAll('bili-comment-renderer, bili-comment-thread-renderer, .reply-item, .sub-reply-item');
			for (var i = 0; i < nodes.length; i++) {
				var t = __lunarText(nodes[i]);
				if (probe && t.indexOf(probe) < 0) continue;
				if (who && t.indexOf(who) < 0) continue;
				var btn = __lunarReplyButton(nodes[i]);
				if (btn) { btn.click(); return 'clicked'; }
			}
			return 'not-found';
		`, 12*time.Second)
		if err != nil {
			return "error"
		}
		return evalPageString(raw)
	}

	state := locate()
	if state == "not-found" {
		// 目标评论可能折叠在「共 N 条回复」里，先展开再重试
		_, _ = w.commentUIRun(`
			var list = __lunarFindAll('button, a, span, div');
			var clicked = 0;
			for (var i = 0; i < list.length; i++) {
				var t = (list[i].textContent || '').trim();
				if (!__lunarVisible(list[i])) continue;
				if (t.indexOf('条回复') >= 0 || t === '展开' || t.indexOf('点击查看') >= 0) {
					list[i].click();
					clicked++;
					if (clicked >= 8) break;
				}
			}
			return clicked;
		`, 10*time.Second)
		w.wait(2500 * time.Millisecond)
		state = locate()
	}
	if state != "clicked" {
		return fmt.Errorf("页面上找不到对方评论（%s）", state)
	}
	w.wait(900 * time.Millisecond)

	payload, _ := json.Marshal(message)
	raw, err := w.commentUIRun(`
		var el = __lunarEditable();
		if (!el) return 'no-input';
		var typed = __lunarType(el, `+string(payload)+`);
		if (!typed) return 'type-failed';
		return 'typed';
	`, 12*time.Second)
	if err != nil {
		return fmt.Errorf("定位回复输入框失败: %w", err)
	}
	if got := evalPageString(raw); got != "typed" {
		return fmt.Errorf("写入回应失败: %s", got)
	}
	w.wait(600 * time.Millisecond)

	raw, err = w.commentUIRun(`
		var btn = __lunarSendButton();
		if (!btn) return 'no-button';
		var label = (btn.textContent || '').trim();
		btn.click();
		return 'clicked:' + label;
	`, 10*time.Second)
	if err != nil {
		return fmt.Errorf("点击回应发送失败: %w", err)
	}
	if got := evalPageString(raw); !strings.HasPrefix(got, "clicked:") {
		return fmt.Errorf("找不到可用的回应发送按钮: %s", got)
	}
	w.wait(1800 * time.Millisecond)
	logger.Info("BilibiliViewer", "页面发布回应成功 → %s", user)
	return nil
}

// videoURLByAid 用 aid 拼视频页地址（消息中心只给对象 ID）。
func videoURLByAid(aid string) string {
	return "https://www.bilibili.com/video/av" + strings.TrimSpace(aid)
}
