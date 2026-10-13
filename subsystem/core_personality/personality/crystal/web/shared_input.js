/* =====================================================================
 * SharedInput · 统一键鼠操作共享模块
 * ---------------------------------------------------------------------
 * 供 Self-LTP（self_ltp_agent.js）与 Mini-LTP（mini_ltp_agent.js）两个网页
 * 操作智能体共享调用。承载全部「页面操作原语」：DOM 工具、元素捕获、模拟
 * 交互（鼠标/键盘/滚轮/滚动/悬停/下拉）、页面状态、视觉截图。
 *
 * 设计原则：
 *   - 单点维护：两智能体不再各自内嵌一份重复实现，键鼠逻辑唯一存在于此。
 *   - 挂载到 window.SharedInput（浏览器注入场景）；同时导出 module.exports 便于测试。
 *   - 宿主先注入本模块，再注入智能体脚本（script.js 保证顺序）。
 * ===================================================================== */
(function (global, factory) {
    var api = factory();
    if (typeof module === 'object' && module.exports) {
        module.exports = api;
    } else if (typeof define === 'function' && define.amd) {
        define([], function () { return api; });
    } else {
        global.SharedInput = api;
    }
})(typeof window !== 'undefined' ? window : (typeof globalThis !== 'undefined' ? globalThis : this), function () {
    'use strict';

    /** 页面元素列表最多展示条目数（控制 token 消耗） */
    var AGENT_MAX_LIST_ITEMS = 60;

    /* ---------------- DOM 基础工具 ---------------- */

    function isVisible(el) {
        if (!el || el.nodeType !== 1) return false;
        try {
            var style = window.getComputedStyle(el);
            if (style.display === 'none' || style.visibility === 'hidden' || Number(style.opacity) === 0) return false;
        } catch (e) { return false; }
        var r = el.getBoundingClientRect();
        return r.width > 0 && r.height > 0;
    }

    function escapeCss(sel) {
        return String(sel).replace(/[\\"']/g, '\\$&');
    }

    function query(sel, root) {
        if (!sel) return null;
        if (typeof sel !== 'string') return (sel && sel.nodeType === 1) ? sel : null;
        try { return (root || document).querySelector(sel); } catch (e) { return null; }
    }

    function queryAll(sel, root) {
        if (!sel) return [];
        if (typeof sel !== 'string') return (sel && sel.nodeType === 1) ? [sel] : [];
        try { return Array.prototype.slice.call((root || document).querySelectorAll(sel)); } catch (e) { return []; }
    }

    function textOf(el, max) {
        if (!el) return '';
        var t = (el.innerText || el.textContent || '').replace(/\s+/g, ' ').trim();
        return max ? t.slice(0, max) : t;
    }

    function describe(el) {
        if (!el) return '未知元素';
        var tag = (el.tagName || '').toLowerCase();
        var label = el.getAttribute('aria-label') || el.getAttribute('title') || el.getAttribute('placeholder') || '';
        var text = textOf(el, 40);
        var name = el.getAttribute('name') || el.id || '';
        var parts = [tag, label, text, name].filter(Boolean);
        return parts[0] === tag ? (tag + (name ? '#' + name : '')) : parts.join('「');
    }

    function findByText(text, root) {
        if (!text || !text.trim()) return null;
        var kw = text.trim().toLowerCase();
        var candidates = [];
        var sel = 'a,button,input,select,textarea,[role="button"],[role="link"],[role="tab"],[role="searchbox"],[onclick],label';
        queryAll(sel, root).forEach(function (el) {
            var hay = [
                el.getAttribute('aria-label'), el.getAttribute('title'),
                el.getAttribute('placeholder'), el.getAttribute('name'), el.id, textOf(el)
            ].filter(Boolean).join(' ').toLowerCase();
            if (hay.indexOf(kw) !== -1) candidates.push(el);
        });
        if (candidates.length) {
            var exact = candidates.filter(function (el) {
                return [el.getAttribute('aria-label'), el.getAttribute('title'), el.getAttribute('placeholder'), textOf(el)]
                    .filter(Boolean).some(function (t) { return t.trim().toLowerCase() === kw; });
            });
            var pool = exact.length ? exact : candidates;
            pool.sort(function (a, b) { return textOf(a).length - textOf(b).length; });
            return pool[0];
        }
        var walker = document.createTreeWalker(root || document.body || document, NodeFilter.SHOW_ELEMENT);
        var node;
        while ((node = walker.nextNode())) {
            if (node.nodeType !== 1) continue;
            var own = node.childNodes.length && Array.prototype.every.call(node.childNodes, function (c) { return c.nodeType === 3; });
            if (own && (node.textContent || '').trim().toLowerCase() === kw && isVisible(node)) return node;
        }
        return null;
    }

    // 解析目标：先按 CSS 选择器，失败后按文本/语义匹配
    function resolveTarget(target, root) {
        if (!target) return null;
        if (typeof target !== 'string') return (target && target.nodeType === 1) ? target : null;
        var t = target.trim();
        if (!t) return null;
        var el = query(t, root);
        if (el) return el;
        return findByText(t, root);
    }

    function uniqueSelector(el) {
        if (!el || el.nodeType !== 1) return null;
        if (el.id) return '#' + escapeCss(el.id.replace(/[^a-zA-Z0-9_-]/g, '\\$&'));
        var parent = el.parentElement;
        if (!parent) return (el.tagName || '').toLowerCase();
        var idx = Array.prototype.indexOf.call(parent.children, el) + 1;
        return (el.tagName || '').toLowerCase() + ':nth-of-type(' + idx + ')';
    }

    /* ---------------- 元素捕获 ---------------- */

    function captureElements(opts) {
        opts = opts || {};
        var max = opts.max || 80;
        var viewportOnly = opts.viewportOnly !== false;
        var sel = opts.selector || 'a,button,input,select,textarea,canvas,[role="button"],[role="link"],[role="tab"],[role="searchbox"],[onclick],label,img,.btn,[data-testid]';
        var roots = opts.root ? [resolveTarget(opts.root)] : [document];
        var seen = new Set();
        var items = [];
        var vw = window.innerWidth || document.documentElement.clientWidth;
        var vh = window.innerHeight || document.documentElement.clientHeight;

        roots.forEach(function (root) {
            if (!root) return;
            queryAll(sel, root).forEach(function (el) {
                if (items.length >= max) return;
                if (seen.has(el)) return;
                seen.add(el);
                if (!isVisible(el)) return;
                if (viewportOnly) {
                    var r0 = el.getBoundingClientRect();
                    if (r0.bottom < 0 || r0.top > vh || r0.right < 0 || r0.left > vw) return;
                }
                var r = el.getBoundingClientRect();
                var tag = (el.tagName || '').toLowerCase();
                var value = '';
                if (el.value !== undefined && String(el.value).length) value = String(el.value).slice(0, 60);
                var label = el.getAttribute('aria-label') || el.getAttribute('title') || el.getAttribute('placeholder') || '';
                var inner = textOf(el, 60);
                var text = [label, inner, value ? '值:' + value : ''].filter(Boolean).join(' | ') || tag;
                items.push({
                    tag: tag, role: el.getAttribute('role') || '', id: el.id || '',
                    type: el.getAttribute('type') || '', name: el.getAttribute('name') || '',
                    href: el.getAttribute('href') || '', text: text, selector: uniqueSelector(el),
                    x: Math.round(r.x), y: Math.round(r.y), w: Math.round(r.width), h: Math.round(r.height)
                });
            });
        });
        return items;
    }

    // 生成紧凑元素列表文本（供模型识别选择器）
    function buildCompactElements(items) {
        if (!items || items.length === 0) return '（页面上没有可见可交互元素）';
        var lines = items.map(function (it, i) {
            return (i + 1) + '. <' + it.tag
                + (it.id ? '#' + it.id : '')
                + (it.role ? ' role=' + it.role : '')
                + (it.type ? ' type=' + it.type : '')
                + (it.name ? ' name=' + it.name : '')
                + '> ' + it.text + '  [' + it.selector + ']';
        });
        if (items.length > AGENT_MAX_LIST_ITEMS) {
            lines = lines.slice(0, AGENT_MAX_LIST_ITEMS);
            lines.push('... 共 ' + items.length + ' 个元素，可用 capture_page 查看更多');
        }
        return lines.join('\n');
    }

    function buildPageInfoText() {
        var s = getState();
        return 'URL: ' + s.url + ' | 标题: ' + s.title + ' | 状态: ' + s.readyState
            + ' | 视口: ' + s.viewport.w + '×' + s.viewport.h + ' | 滚动: (' + s.scroll.x + ', ' + s.scroll.y + ')';
    }

    /* ---------------- 模拟交互（页面操作原语） ---------------- */

    // 在元素上派发一次带坐标/按键的鼠标事件（优先 PointerEvent，降级 MouseEvent）。
    // 关键修复：必须注入 clientX/clientY/offsetX/offsetY/button/buttons，否则依赖坐标做射线拾取的
    // 页面（如监听 pointer 事件并用 clientX/clientY 做坐标拾取的 canvas 交互）会因坐标缺省(0,0)而无法命中。
    function dispatchAt(el, type, cx, cy, button) {
        button = (button === undefined || button === null) ? 0 : button;
        var buttons = button === 0 ? 1 : (button === 2 ? 2 : 4);
        var rect = el.getBoundingClientRect();
        var ox = cx - rect.left, oy = cy - rect.top;
        var evt;
        if (type.indexOf('pointer') === 0) {
            try {
                evt = new PointerEvent(type, {
                    bubbles: true, cancelable: true, view: window,
                    pointerId: 7, pointerType: 'mouse', isPrimary: true,
                    button: button, buttons: buttons,
                    clientX: cx, clientY: cy, offsetX: ox, offsetY: oy,
                    screenX: cx, screenY: cy
                });
            } catch (e) {
                evt = document.createEvent('MouseEvent');
                evt.initMouseEvent(type, true, true, window, 0, cx, cy, cx, cy,
                    false, false, false, false, button, null);
            }
        } else {
            try {
                evt = new MouseEvent(type, {
                    bubbles: true, cancelable: true, view: window,
                    button: button, buttons: buttons,
                    clientX: cx, clientY: cy, offsetX: ox, offsetY: oy,
                    screenX: cx, screenY: cy
                });
            } catch (e) {
                evt = document.createEvent('MouseEvent');
                evt.initMouseEvent(type, true, true, window, 0, cx, cy, cx, cy,
                    false, false, false, false, button, null);
            }
        }
        // 兜底：个别环境 clientX/clientY 只读，强制固化坐标
        try {
            if (evt.clientX !== cx) {
                Object.defineProperty(evt, 'clientX', { value: cx });
                Object.defineProperty(evt, 'clientY', { value: cy });
            }
        } catch (e) { /* 忽略 */ }

        // 合成 pointer 事件触发 setPointerCapture(pointerId) 时会因「无活动指针」抛 NotFoundError，
        // 进而中断页面 onDown 的后续记录（pointers.set）逻辑；派发 pointerdown 期间临时置为 no-op 规避。
        var proto = null, origCap = null;
        if (type === 'pointerdown' && typeof Element !== 'undefined' && Element.prototype && 'setPointerCapture' in Element.prototype) {
            try { proto = Element.prototype; origCap = proto.setPointerCapture; proto.setPointerCapture = function () { }; } catch (e) { }
        }
        try {
            el.dispatchEvent(evt);
        } finally {
            if (proto) { try { proto.setPointerCapture = origCap; } catch (e) { } }
        }
    }

    // —— 鼠标统一交互底层 ——
    // 归一化点击目标：给定 target 优先用元素；给出 x/y 则按视口坐标定位（elementFromPoint
    // 命中该点的实际元素，通常用于画布/地图等按坐标交互的目标）。返回 {el, x, y}。
    function resolvePoint(target, x, y) {
        var el = target ? resolveTarget(target) : null;
        if (typeof x !== 'number' || typeof y !== 'number') {
            if (!el) el = document.activeElement instanceof Element ? document.activeElement : document.body;
            var r = el.getBoundingClientRect();
            x = r.left + r.width / 2;
            y = r.top + r.height / 2;
        } else if (!el) {
            el = (document.elementFromPoint && document.elementFromPoint(x, y)) || document.body;
        }
        return { el: el, x: x, y: y };
    }

    // 同步派发一次完整的「按下→抬起→左键click」鼠标序列（统一底层）。
    // 带坐标与按键，兼顾只监听 pointer（画布坐标拾取）或只监听 mouse 的页面。
    function fireMouse(el, x, y, button) {
        dispatchAt(el, 'pointerdown', x, y, button);
        dispatchAt(el, 'mousedown', x, y, button);
        dispatchAt(el, 'pointerup', x, y, button);
        dispatchAt(el, 'mouseup', x, y, button);
        if (button === 0) dispatchAt(el, 'click', x, y, button); // 左键才触发完整 click
    }

    function focusEl(el) {
        try { el.scrollIntoView({ block: 'center' }); } catch (e) { /* 忽略 */ }
        try { el.focus({ preventScroll: true }); } catch (e) { /* 忽略 */ }
    }

    // clickEl：左键单击（tap）。覆盖绝大多数「点击按钮/链接/画布目标点」需求；
    // 需要自定义按键（右键/中键）或按住时长时，改用 mouse。
    function clickEl(target, coords) {
        var pt = resolvePoint(target, coords && coords.x, coords && coords.y);
        if (!pt.el) return { ok: false, reason: '未找到目标元素: ' + target };
        focusEl(pt.el);
        fireMouse(pt.el, pt.x, pt.y, 0);
        return { ok: true, info: '左键点击 ' + describe(pt.el) + ' @(' + Math.round(pt.x) + ',' + Math.round(pt.y) + ')' };
    }

    function hoverEl(target) {
        var el = resolveTarget(target);
        if (!el) return { ok: false, reason: '未找到目标元素: ' + target };
        var pt = resolvePoint(target, undefined, undefined);
        var cx = pt.x, cy = pt.y;
        ['pointerover', 'mouseover', 'mouseenter', 'pointerenter'].forEach(function (t) {
            dispatchAt(el, t, cx, cy, 0);
        });
        return { ok: true, info: '已悬停 ' + describe(el) + ' @(' + Math.round(cx) + ',' + Math.round(cy) + ')' };
    }

    function setInputValue(el, text) {
        if (el.isContentEditable || el.getAttribute('contenteditable') === 'true') {
            el.focus();
            if (document.createRange && window.getSelection) {
                var range = document.createRange();
                range.selectNodeContents(el);
                var sel = window.getSelection();
                sel.removeAllRanges();
                sel.addRange(range);
            }
            el.textContent = text;
        } else {
            var proto = el.tagName === 'TEXTAREA' ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
            var setter = Object.getOwnPropertyDescriptor(proto, 'value').set;
            setter.call(el, text);
        }
        el.dispatchEvent(new Event('input', { bubbles: true }));
        el.dispatchEvent(new Event('change', { bubbles: true }));
    }

    function typeEl(target, text) {
        var el = resolveTarget(target);
        if (!el) return { ok: false, reason: '未找到目标元素: ' + target };
        try { el.scrollIntoView({ block: 'center' }); } catch (e) { /* 忽略 */ }
        try { el.focus({ preventScroll: true }); } catch (e) { /* 忽略 */ }
        setInputValue(el, String(text));
        return { ok: true, info: '已向 ' + describe(el) + ' 输入 ' + String(text).length + ' 个字符' };
    }

    // 将 key 值映射为 KeyboardEvent.code（许多页面用 e.code 判断按键，如游戏监听 KeyR）
    function keyToCode(key) {
        var k = String(key);
        if (/^[a-zA-Z]$/.test(k)) return 'Key' + k.toUpperCase();
        if (/^[0-9]$/.test(k)) return 'Digit' + k;
        var map = {
            'Enter': 'Enter', ' ': 'Space', 'Tab': 'Tab', 'Backspace': 'Backspace',
            'Delete': 'Delete', 'ArrowUp': 'ArrowUp', 'ArrowDown': 'ArrowDown',
            'ArrowLeft': 'ArrowLeft', 'ArrowRight': 'ArrowRight', 'Escape': 'Escape',
            'Home': 'Home', 'End': 'End', 'PageUp': 'PageUp', 'PageDown': 'PageDown',
            'CapsLock': 'CapsLock', 'NumLock': 'NumLock', 'ScrollLock': 'ScrollLock',
            'PrintScreen': 'PrintScreen', 'Pause': 'Pause', 'ContextMenu': 'ContextMenu',
            '+': 'NumpadAdd', '-': 'NumpadSubtract', '=': 'Equal', ',': 'Comma',
            '.': 'Period', '/': 'Slash', ';': 'Semicolon', "'": 'Quote',
            '[': 'BracketLeft', ']': 'BracketRight', '\\': 'Backslash', '`': 'Backquote'
        };
        if (map[k]) return map[k];
        if (/^F([1-9]|1[0-2])$/i.test(k)) return k.toUpperCase();
        return '';
    }

    // 将按键别名/符号归一为可直接用于 KeyboardEvent 的 key 值
    function normalizeKeyToken(tok) {
        var t = String(tok == null ? '' : tok).trim();
        if (!t) return '';
        var lower = t.toLowerCase();
        var aliases = {
            'enter': 'Enter', 'return': 'Enter', 'esc': 'Escape', 'escape': 'Escape',
            'space': ' ', 'spacebar': ' ', 'tab': 'Tab',
            'up': 'ArrowUp', 'arrowup': 'ArrowUp', 'down': 'ArrowDown', 'arrowdown': 'ArrowDown',
            'left': 'ArrowLeft', 'arrowleft': 'ArrowLeft', 'right': 'ArrowRight', 'arrowright': 'ArrowRight',
            'backspace': 'Backspace', 'del': 'Delete', 'delete': 'Delete', 'home': 'Home', 'end': 'End',
            'pageup': 'PageUp', 'pagedown': 'PageDown', 'capslock': 'CapsLock',
            'plus': '+', 'add': '+', 'minus': '-', 'subtract': '-', 'equal': '=', 'equals': '=',
            'comma': ',', 'period': '.', 'dot': '.', 'slash': '/', 'semicolon': ';', 'quote': "'"
        };
        if (aliases[lower]) return aliases[lower];
        if (/^F1[0-2]$|^F[1-9]$/i.test(t)) return t.toUpperCase();
        return t;
    }

    // 模拟键盘按键，支持三态按住语义与组合键（同 Mini-LTP）
    async function pressKeyEl(combo) {
        var comboStr = String(combo || '').trim();
        if (!comboStr) return { ok: false, reason: '未指定按键' };
        var holdMs = 0;
        var m;
        if ((m = comboStr.match(/^(?:long|hold|10s)(?::|\s)*/i))) {
            holdMs = 10000;
            comboStr = comboStr.slice(m[0].length).trim();
        } else if ((m = comboStr.match(/^(?:short|tap|1s)(?::|\s)*/i))) {
            holdMs = 1000;
            comboStr = comboStr.slice(m[0].length).trim();
        }
        var parts = comboStr.split(/\s*\+\s*/).map(normalizeKeyToken).filter(Boolean);
        var key = parts.pop() || '';
        var modNames = { ctrl: 'ctrl', control: 'ctrl', alt: 'alt', option: 'alt', shift: 'shift', meta: 'meta', win: 'meta', cmd: 'meta', command: 'meta' };
        var mods = { ctrlKey: false, altKey: false, shiftKey: false, metaKey: false };
        var ghosts = [];
        parts.forEach(function (p) {
            var lower = p.toLowerCase();
            if (modNames[lower]) mods[modNames[lower] + 'Key'] = true;
            else ghosts.push(p);
        });
        if (!key && ghosts.length) key = ghosts[ghosts.length - 1];
        if (!key) return { ok: false, reason: '未指定按键' };
        if (/^[a-zA-Z]$/.test(key)) key = mods.shiftKey ? key.toUpperCase() : key.toLowerCase();
        var code = keyToCode(key);
        var target = document.activeElement || document.body;
        var makeEvt = function (type) {
            var evt;
            try {
                evt = new KeyboardEvent(type, { bubbles: true, cancelable: true, key: key, code: code, view: window, ...mods });
            } catch (e) {
                evt = document.createEvent('KeyboardEvent');
                evt.initKeyboardEvent(type, true, true, window, key, 0, mods.ctrlKey, mods.altKey, mods.shiftKey, mods.metaKey);
                try { Object.defineProperty(evt, 'code', { get: function () { return code; } }); } catch (e2) { /* 旧环境忽略 */ }
            }
            return evt;
        };
        target.dispatchEvent(makeEvt('keydown'));
        target.dispatchEvent(makeEvt('keypress'));
        if (holdMs > 0) await sleep(holdMs);
        target.dispatchEvent(makeEvt('keyup'));
        if (key === 'Enter') {
            var form = target.form;
            if (form) { try { form.requestSubmit(); } catch (e2) { } }
        }
        var mode = holdMs === 10000 ? '长按' : (holdMs === 1000 ? '短按' : '键入');
        var modDesc = Object.keys(mods).filter(function (k) { return mods[k]; })
            .map(function (k) { return k.replace('Key', ''); }).join('+');
        return { ok: true, info: mode + ' ' + (modDesc ? modDesc + '+' : '') + key + '（作用于 ' + describe(target) + '）' };
    }

    function parseHoldSpec(spec) {
        if (!spec || typeof spec !== 'string') return { holdMs: 0, rest: String(spec == null ? '' : spec) };
        var s = String(spec).trim();
        var m;
        if ((m = s.match(/^(?:long|hold|10s)(?::|\s)*/i))) return { holdMs: 10000, rest: s.slice(m[0].length).trim() };
        if ((m = s.match(/^(?:short|1s)(?::|\s)*/i))) return { holdMs: 1000, rest: s.slice(m[0].length).trim() };
        return { holdMs: 0, rest: s };
    }

    // mouse 通用鼠标按键：用于需要指定按键（右键/中键）或按住时长（short/long）的场景；
    // 普通「左键单击」请直接用 clickEl（更简洁、更不易出错）。
    async function mouseButtonEl(spec, target, coords) {
        var p = parseHoldSpec(spec);
        var btnStr = (p.rest || 'left').toLowerCase();
        var btnMap = { left: 0, right: 2, middle: 1 };
        var btn = btnMap[btnStr] !== undefined ? btnMap[btnStr] : 0;
        var btnName = { 0: '左键', 2: '右键', 1: '中键' }[btn];
        var pt = resolvePoint(target, coords && coords.x, coords && coords.y);
        if (!pt.el) return { ok: false, reason: '未找到目标元素: ' + target };
        focusEl(pt.el);
        if (p.holdMs > 0) {
            // 按住：按下→hold→抬起（与 fireMouse 同序，仅中间插入延时）
            dispatchAt(pt.el, 'pointerdown', pt.x, pt.y, btn);
            dispatchAt(pt.el, 'mousedown', pt.x, pt.y, btn);
            await sleep(p.holdMs);
            dispatchAt(pt.el, 'pointerup', pt.x, pt.y, btn);
            dispatchAt(pt.el, 'mouseup', pt.x, pt.y, btn);
            if (btn === 0) dispatchAt(pt.el, 'click', pt.x, pt.y, btn);
        } else {
            fireMouse(pt.el, pt.x, pt.y, btn);
        }
        var mode = p.holdMs === 10000 ? '长按' : (p.holdMs === 1000 ? '短按' : '单击');
        return { ok: true, info: mode + ' ' + btnName + ' @(' + Math.round(pt.x) + ',' + Math.round(pt.y) + ')（作用于 ' + describe(pt.el) + '）' };
    }

    function wheelEl(direction, ticks) {
        var dir = String(direction || 'down').toLowerCase();
        var isUp = dir === 'up' || dir === 'scrollup' || dir === 'top';
        var n = Math.round(Number(ticks) || 3);
        if (n <= 0) n = 3;
        var deltaY = isUp ? -n * 100 : n * 100;
        var el = document.activeElement instanceof Element ? document.activeElement : document.body;
        var evt;
        try {
            evt = new WheelEvent('wheel', { bubbles: true, cancelable: true, view: window, deltaY: deltaY, deltaMode: 0 });
        } catch (e) {
            evt = document.createEvent('Event');
            evt.initEvent('wheel', true, true);
            try { Object.defineProperty(evt, 'deltaY', { get: function () { return deltaY; } }); } catch (e2) { /* 忽略 */ }
        }
        el.dispatchEvent(evt);
        return { ok: true, info: '滚轮向' + (isUp ? '上' : '下') + '滚动 ' + n + ' 格' };
    }

    function sleep(ms) {
        return new Promise(function (resolve) { setTimeout(resolve, ms); });
    }

    function scrollEl(direction, amount) {
        var dir = String(direction || 'down').toLowerCase();
        var el = document.scrollingElement || document.documentElement;
        var vh = window.innerHeight || el.clientHeight;
        if (dir === 'top' || dir === 'bottom' || dir === 'up' || dir === 'down' || dir === 'left' || dir === 'right') {
            var delta = typeof amount === 'number' ? amount : vh * 0.8;
            var x = 0, y = 0;
            if (dir === 'top') y = -el.scrollTop;
            else if (dir === 'bottom') y = el.scrollHeight - el.scrollTop;
            else if (dir === 'down') y = delta;
            else if (dir === 'up') y = -delta;
            else if (dir === 'right') x = delta;
            else if (dir === 'left') x = -delta;
            try { window.scrollBy({ top: y, left: x, behavior: 'auto' }); } catch (e) { window.scrollBy(x, y); }
            return { ok: true, info: '已向' + dir + '滚动 ' + Math.round(Math.abs(y || x)) + 'px' };
        }
        var targetEl = resolveTarget(String(dir));
        if (targetEl) {
            try { targetEl.scrollIntoView({ block: 'center' }); } catch (e) { /* 忽略 */ }
            return { ok: true, info: '已滚动到 ' + describe(targetEl) };
        }
        return { ok: false, reason: '无法识别的滚动目标: ' + direction };
    }

    function selectEl(target, value) {
        var el = resolveTarget(target);
        if (!el || el.tagName !== 'SELECT') return { ok: false, reason: '未找到下拉框: ' + target };
        var matched = Array.prototype.find.call(el.options, function (o) {
            return o.value === String(value) || o.text.trim() === String(value).trim();
        });
        if (!matched) return { ok: false, reason: '下拉框不存在选项: ' + value };
        el.value = matched.value;
        el.dispatchEvent(new Event('input', { bubbles: true }));
        el.dispatchEvent(new Event('change', { bubbles: true }));
        return { ok: true, info: '已选择「' + matched.text + '」' };
    }

    function getState() {
        var el = document.scrollingElement || document.documentElement;
        return {
            url: location.href,
            title: document.title,
            readyState: document.readyState,
            viewport: { w: window.innerWidth, h: window.innerHeight },
            scroll: { x: Math.round(el.scrollLeft), y: Math.round(el.scrollTop) }
        };
    }

    /* ---------------- 视觉截图（网格坐标 + 元素编号，供多模态模型视觉定位） ---------------- */

    var SCREEN_MAX_WIDTH = 1024;
    var SCREEN_MAX_HEIGHT = 1536;
    var SCREEN_GRID_STEP = 50;

    var latestScreenshot = null;

    function tryDrawPagePixels(ctx, vw, vh, cw, ch) {
        return new Promise(function (resolve) {
            try {
                var root = document.documentElement;
                var dw = Math.max(root.scrollWidth || vw, vw);
                var dh = Math.max(root.scrollHeight || vh, vh);
                var svg = '<svg xmlns="http://www.w3.org/2000/svg" xmlns:xhtml="http://www.w3.org/1999/xhtml" width="' + dw + '" height="' + dh + '">'
                    + '<foreignObject width="100%" height="100%">'
                    + '<xhtml:body xmlns:xhtml="http://www.w3.org/1999/xhtml" style="margin:0;padding:0">'
                    + (root.body ? root.body.innerHTML : '') + '</xhtml:body></foreignObject></svg>';
                var img = new Image();
                img.onload = function () {
                    try {
                        var sx = window.scrollX || window.pageXOffset || 0;
                        var sy = window.scrollY || window.pageYOffset || 0;
                        ctx.drawImage(img, sx, sy, vw, vh, 0, 0, cw, ch);
                        resolve(true);
                    } catch (e) { resolve(false); }
                };
                img.onerror = function () { resolve(false); };
                img.src = 'data:image/svg+xml;charset=utf-8,' + encodeURIComponent(svg);
            } catch (e) { resolve(false); }
        });
    }

    function drawGrid(ctx, vw, vh, cw, ch, scale) {
        ctx.save();
        var i;
        ctx.font = '10px sans-serif';
        ctx.textBaseline = 'alphabetic';
        for (i = 0; i <= vw; i += SCREEN_GRID_STEP) {
            var gx = Math.round(i * scale) + 0.5;
            ctx.strokeStyle = 'rgba(24,80,160,0.18)';
            ctx.beginPath(); ctx.moveTo(gx, 0); ctx.lineTo(gx, ch); ctx.stroke();
            ctx.fillStyle = 'rgba(24,80,160,0.6)';
            ctx.fillText(String(i), gx + 3, 12);
        }
        for (i = 0; i <= vh; i += SCREEN_GRID_STEP) {
            var gy = Math.round(i * scale) + 0.5;
            ctx.strokeStyle = 'rgba(24,80,160,0.18)';
            ctx.beginPath(); ctx.moveTo(0, gy); ctx.lineTo(cw, gy); ctx.stroke();
            ctx.fillStyle = 'rgba(24,80,160,0.6)';
            ctx.fillText(String(i), 3, gy - 3);
        }
        ctx.strokeStyle = 'rgba(24,80,160,0.7)';
        ctx.lineWidth = 2;
        ctx.strokeRect(1, 1, cw - 2, ch - 2);
        ctx.restore();
    }

    function drawElementBoxes(ctx, items, cw, ch, scale) {
        if (!items || items.length === 0) return;
        ctx.save();
        items.forEach(function (it, i) {
            var bx = Math.round(it.x * scale), by = Math.round(it.y * scale);
            var bw = Math.max(3, Math.round(it.w * scale)), bh = Math.max(3, Math.round(it.h * scale));
            if (bx + bw < 0 || by + bh < 0 || bx > cw || by > ch) return;
            var num = i + 1;
            ctx.strokeStyle = 'rgba(224,86,32,0.95)';
            ctx.lineWidth = 1.5;
            ctx.strokeRect(bx, by, bw, bh);
            ctx.fillStyle = 'rgba(224,86,32,0.9)';
            ctx.font = 'bold 11px sans-serif';
            var tw = ctx.measureText(String(num)).width;
            var badge = Math.max(15, tw + 6);
            ctx.beginPath(); ctx.arc(bx, by, badge / 2, 0, Math.PI * 2); ctx.fill();
            ctx.fillStyle = '#ffffff';
            ctx.textAlign = 'center';
            ctx.textBaseline = 'middle';
            ctx.fillText(String(num), bx, by + 0.5);
            ctx.textAlign = 'left';
            ctx.textBaseline = 'alphabetic';
        });
        ctx.restore();
    }

    async function captureScreenshot(itemsForBoxes) {
        var vw = window.innerWidth || document.documentElement.clientWidth || 800;
        var vh = window.innerHeight || document.documentElement.clientHeight || 600;
        var scale = Math.min(1, SCREEN_MAX_WIDTH / vw, SCREEN_MAX_HEIGHT / vh);
        if (!(scale > 0) || !isFinite(scale)) scale = 1;
        var cw = Math.max(1, Math.round(vw * scale));
        var ch = Math.max(1, Math.round(vh * scale));
        var canvas = document.createElement('canvas');
        canvas.width = cw; canvas.height = ch;
        var ctx = canvas.getContext('2d');
        ctx.fillStyle = '#ffffff'; ctx.fillRect(0, 0, cw, ch);
        if (!itemsForBoxes) itemsForBoxes = captureElements({ max: AGENT_MAX_LIST_ITEMS });
        var ok = await tryDrawPagePixels(ctx, vw, vh, cw, ch);
        if (!ok) {
            ctx.fillStyle = '#f4f6fa'; ctx.fillRect(0, 0, cw, ch);
            ctx.fillStyle = '#8a93a1'; ctx.font = '12px sans-serif';
            ctx.fillText('（无法还原页面像素，仅显示网格坐标与元素编号示意）', 12, 26);
        }
        drawGrid(ctx, vw, vh, cw, ch, scale);
        drawElementBoxes(ctx, itemsForBoxes, cw, ch, scale);
        var dataUrl;
        try { dataUrl = canvas.toDataURL('image/png'); } catch (e) { dataUrl = ''; }
        latestScreenshot = { dataUrl: dataUrl, width: cw, height: ch, count: itemsForBoxes.length, viewport: [vw, vh], scale: scale };
        return latestScreenshot;
    }

    /* ---------------- 对外命名空间 ---------------- */

    return {
        AGENT_MAX_LIST_ITEMS: AGENT_MAX_LIST_ITEMS,
        SCREEN_GRID_STEP: SCREEN_GRID_STEP,
        // DOM 基础工具
        isVisible: isVisible, escapeCss: escapeCss, query: query, queryAll: queryAll,
        textOf: textOf, describe: describe, findByText: findByText,
        resolveTarget: resolveTarget, uniqueSelector: uniqueSelector,
        // 元素捕获
        captureElements: captureElements, buildCompactElements: buildCompactElements,
        buildPageInfoText: buildPageInfoText,
        // 模拟交互
        dispatchAt: dispatchAt, resolvePoint: resolvePoint, fireMouse: fireMouse, focusEl: focusEl,
        clickEl: clickEl, hoverEl: hoverEl, setInputValue: setInputValue, typeEl: typeEl,
        keyToCode: keyToCode, normalizeKeyToken: normalizeKeyToken, pressKeyEl: pressKeyEl,
        parseHoldSpec: parseHoldSpec, mouseButtonEl: mouseButtonEl, wheelEl: wheelEl,
        sleep: sleep, scrollEl: scrollEl, selectEl: selectEl, getState: getState,
        // 视觉截图
        tryDrawPagePixels: tryDrawPagePixels, drawGrid: drawGrid,
        drawElementBoxes: drawElementBoxes, captureScreenshot: captureScreenshot
    };
});