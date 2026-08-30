/* =====================================================================
 * MiniLTP Agent · 通用页面操作智能体（LLM 驱动，LTPX AtoA）
 * ---------------------------------------------------------------------
 * 遵循 LTPX AtoA 设计理念：月华将自然语言指令交给本智能体，
 * 由大语言模型（LLM）作为最终执行者完成意图识别与页面操作调度。
 * 与文件管理器 ltpx-agent.js 同构，仅能力域不同（通用页面操作，非业务定制）。
 *
 * - 独立上下文历史：保留最近 40 轮对话，超出丢弃最早的轮次
 * - 专用系统提示词：定义「通用页面操作执行者」角色与操作边界
 * - 多轮工具调用循环：LLM 通过 OpenAI function calling 逐次调用页面操作工具，
 *   观察工具结果并持续决策，直到给出最终答复
 * - 模型调用走琉璃后端 /v1 代理（OpenAI v1 协议，由后端按 lunar_config.json
 *   解析模型 name/key/url，前端不接触模型配置）
 * - 内嵌 AtoA 集成层：位于 iframe 中时自动监听 ltpx_run 并回执 ltpx_result
 * - LLM 不可用时降级到最小离线规则，保证基础可用（非主路径）
 * ===================================================================== */
(function (global, factory) {
    var api = factory();
    if (typeof module === 'object' && module.exports) {
        module.exports = api;
    } else if (typeof define === 'function' && define.amd) {
        define([], function () { return api; });
    } else {
        global.MiniLTPAgent = api;
    }
})(typeof window !== 'undefined' ? window : (typeof globalThis !== 'undefined' ? globalThis : this), function () {
    'use strict';

    var VERSION = '2.1.0';

    // ================= 智能体常量与状态 =================

    /** 独立上下文历史：最多保留的对话轮数（1 轮 = 用户指令 + 智能体答复） */
    var AGENT_MAX_ROUNDS = 40;
    /** 单条指令允许的最大工具调用循环次数（防模型无限调用工具） */
    var AGENT_MAX_TOOL_LOOPS = 20;
    /** 页面元素列表最多展示条目数（控制 token 消耗） */
    var AGENT_MAX_LIST_ITEMS = 60;

    /** 独立上下文历史：数组元素为 { user, assistant }（各为纯文本） */
    var agentHistory = [];

    // ================= 模型配置加载（从 lunar_config.json 的 agent 字段读取，不硬编码） =================

    /** 配置缓存与加载 Promise（一次加载，后续复用） */
    let modelConfigPromise = null;
    let modelConfig = null;

    /**
     * 读取 lunar_config.json 的 agent 配置（通过 /file/read/ 文件接口）。
     * 模型名取 agent.multimodal_model；base_url / api_key 一并返回供自定义代理使用。
     * @returns {Promise<{model:string, base_url:string, api_key:string|null}>}
     */
    async function loadModelConfig() {
        if (modelConfigPromise) return modelConfigPromise;
        modelConfigPromise = (async () => {
            try {
                const resp = await fetch('/file/read/lunar_config.json', { cache: 'no-store' });
                if (!resp.ok) throw new Error('读取配置失败 HTTP ' + resp.status);
                const cfg = await resp.json();
                const agent = (cfg && cfg.agent) || {};
                modelConfig = {
                    model: (agent.multimodal_model && String(agent.multimodal_model)) || 'system-multimodal',
                    base_url: (agent.multimodal_url && String(agent.multimodal_url)) || '/v1',
                    api_key: agent.multimodal_key || null
                };
                return modelConfig;
            } catch (e) {
                // 读取失败回退默认（与配置中的占位值一致，仍走同源 /v1 代理解析）
                modelConfig = { model: 'system-multimodal', base_url: '/v1', api_key: null };
                return modelConfig;
            }
        })();
        return modelConfigPromise;
    }

    /** 强制重新加载模型配置（配置变更后调用） */
    async function refreshModelConfig() {
        modelConfigPromise = null;
        modelConfig = null;
        return loadModelConfig();
    }

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
        var sel = opts.selector || 'a,button,input,select,textarea,[role="button"],[role="link"],[role="tab"],[role="searchbox"],[onclick],label,img,.btn,[data-testid]';
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

    // 生成紧凑元素列表文本（供模型识别选择器，与文件管理器目录列表同思路）
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

    function dispatchMouse(el, type) {
        var evt;
        try { evt = new MouseEvent(type, { bubbles: true, cancelable: true, view: window }); }
        catch (e) { evt = document.createEvent('MouseEvents'); evt.initEvent(type, true, true); }
        el.dispatchEvent(evt);
    }

    function clickEl(target) {
        var el = resolveTarget(target);
        if (!el) return { ok: false, reason: '未找到目标元素: ' + target };
        try { el.scrollIntoView({ block: 'center' }); } catch (e) { /* 忽略 */ }
        try { el.focus({ preventScroll: true }); } catch (e) { /* 忽略 */ }
        ['pointerdown', 'mousedown', 'pointerup', 'mouseup', 'click'].forEach(function (t) {
            if (t.indexOf('pointer') === 0) {
                try {
                    var pe = new PointerEvent(t, { bubbles: true, cancelable: true, view: window, pointerId: 1, pointerType: 'mouse' });
                    el.dispatchEvent(pe);
                    return;
                } catch (e) { /* 降级为鼠标事件 */ }
            }
            dispatchMouse(el, t);
        });
        return { ok: true, info: '已点击 ' + describe(el) };
    }

    function hoverEl(target) {
        var el = resolveTarget(target);
        if (!el) return { ok: false, reason: '未找到目标元素: ' + target };
        ['pointerover', 'mouseover', 'mouseenter', 'pointerenter'].forEach(function (t) {
            if (t.indexOf('pointer') === 0) {
                try {
                    var pe = new PointerEvent(t, { bubbles: true, cancelable: true, view: window, pointerId: 1, pointerType: 'mouse' });
                    el.dispatchEvent(pe);
                    return;
                } catch (e) { /* 降级 */ }
            }
            dispatchMouse(el, t);
        });
        return { ok: true, info: '已悬停 ' + describe(el) };
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
            'Home': 'Home', 'End': 'End', 'PageUp': 'PageUp', 'PageDown': 'PageDown'
        };
        return map[k] || '';
    }

    // 按键语义差分：
    //   - 默认 / tap:  键入 —— 瞬间按下并立即松开（等价一次点击操作）
    //   - short:       短按 —— 按住 1 秒后松开
    //   - long:        长按 —— 按住 10 秒后松开
    async function pressKeyEl(combo) {
        var comboStr = String(combo || '').trim();
        if (!comboStr) return { ok: false, reason: '未指定按键' };
        // 解析语义前缀（支持 short/short:/1s/tap:long/long:/10s 等变体）
        var holdMs = 0; // 0 = 瞬间（键入）
        var m;
        if ((m = comboStr.match(/^(?:long|hold|10s)(?::|\s)*/i))) {
            holdMs = 10000;
            comboStr = comboStr.slice(m[0].length).trim();
        } else if ((m = comboStr.match(/^(?:short|tap|1s)(?::|\s)*/i))) {
            holdMs = 1000;
            comboStr = comboStr.slice(m[0].length).trim();
        }
        var parts = comboStr.split(/\s*\+\s*/);
        var key = parts.pop() || '';
        if (!key) return { ok: false, reason: '未指定按键' };
        var mods = {
            ctrlKey: parts.some(function (p) { return /^ctrl$/i.test(p); }),
            altKey: parts.some(function (p) { return /^alt$/i.test(p); }),
            shiftKey: parts.some(function (p) { return /^shift$/i.test(p); }),
            metaKey: parts.some(function (p) { return /^(meta|win|cmd)$/i.test(p); })
        };
        var keyMap = { enter: 'Enter', esc: 'Escape', escape: 'Escape', space: ' ', tab: 'Tab', up: 'ArrowUp', down: 'ArrowDown', left: 'ArrowLeft', right: 'ArrowRight', backspace: 'Backspace', del: 'Delete', delete: 'Delete', home: 'Home', end: 'End', pageup: 'PageUp', pagedown: 'PageDown' };
        key = keyMap[key.toLowerCase()] || key;
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
        // keydown → keypress；按住 holdMs 后再 keyup。
        // 键入（holdMs=0）：按下立即松开（一次点击）；短按/长按：保持按住指定时长再松开。
        target.dispatchEvent(makeEvt('keydown'));
        target.dispatchEvent(makeEvt('keypress'));
        if (holdMs > 0) {
            await sleep(holdMs);
        }
        target.dispatchEvent(makeEvt('keyup'));
        if (key === 'Enter') {
            var form = target.form;
            if (form) { try { form.requestSubmit(); } catch (e2) { } }
        }
        var mode = holdMs === 10000 ? '长按' : (holdMs === 1000 ? '短按' : '键入');
        return { ok: true, info: mode + ' ' + comboStr + '（按住 ' + (holdMs || '0ms/瞬间') + '，作用于 ' + describe(target) + '）' };
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

    /* ---------------- 工具定义（OpenAI function calling 格式） ---------------- */

    var agentTools = [
        {
            type: 'function',
            function: {
                name: 'capture_page',
                description: '捕获当前页面可见可交互的元素快照（含标签/文本/选择器/位置），用于确认可操作目标。当不确定要操作什么、或找不到目标时调用。',
                parameters: {
                    type: 'object',
                    properties: {
                        max: { type: 'integer', description: '最多返回元素数，默认 60' }
                    },
                    required: []
                }
            }
        },
        {
            type: 'function',
            function: {
                name: 'click',
                description: '点击页面上的元素。target 可为选择器（如 #submit、.btn）或元素文本/aria-label/占位符描述（如「搜索按钮」）。',
                parameters: {
                    type: 'object',
                    properties: {
                        target: { type: 'string', description: '目标元素的选择器或文本描述' }
                    },
                    required: ['target']
                }
            }
        },
        {
            type: 'function',
            function: {
                name: 'type',
                description: '向输入框/文本域输入文本。target 可为选择器或文本描述。',
                parameters: {
                    type: 'object',
                    properties: {
                        target: { type: 'string', description: '输入框的选择器或文本描述' },
                        text: { type: 'string', description: '要输入的文本内容' }
                    },
                    required: ['target', 'text']
                }
            }
        },
        {
            type: 'function',
            function: {
                name: 'press_key',
                description: '模拟按键，支持三种按住语义（在 key 内用前缀表达）：默认/无前缀=键入（瞬间按下立即松开，等价一次点击）；short:=短按（按住 1 秒）；long: 或 hold:=长按（按住 10 秒）。如 press_key(W)=键入W，press_key(short:W)=短按W，press_key(long:W)=长按W。支持 Enter、Escape、Space、Tab、R、ArrowDown 等，组合键用 + 连接（如 Ctrl+A）。',
                parameters: {
                    type: 'object',
                    properties: {
                        key: { type: 'string', description: '按键名，可用前缀 short:/long:/hold: 表达按住时长，如 W、short:W、long:W、Ctrl+A' }
                    },
                    required: ['key']
                }
            }
        },
        {
            type: 'function',
            function: {
                name: 'scroll',
                description: '滚动页面：direction 为 up/down/left/right/top/bottom（可选 amount 像素），或传 target 滚动到指定元素。',
                parameters: {
                    type: 'object',
                    properties: {
                        direction: { type: 'string', description: '滚动方向：up/down/left/right/top/bottom，或目标元素描述（滚动到该元素）' },
                        amount: { type: 'integer', description: '滚动像素数（可选）' },
                        target: { type: 'string', description: '滚动到该元素（可选，指定时 direction 可省略）' }
                    },
                    required: []
                }
            }
        },
        {
            type: 'function',
            function: {
                name: 'hover',
                description: '悬停到元素上（触发 mouseover 等悬停事件）。',
                parameters: {
                    type: 'object',
                    properties: {
                        target: { type: 'string', description: '目标元素的选择器或文本描述' }
                    },
                    required: ['target']
                }
            }
        },
        {
            type: 'function',
            function: {
                name: 'select_option',
                description: '在下拉框中选中指定选项（按选项值或显示文本）。',
                parameters: {
                    type: 'object',
                    properties: {
                        target: { type: 'string', description: '下拉框的选择器或文本描述' },
                        value: { type: 'string', description: '要选中的选项值或显示文本' }
                    },
                    required: ['target', 'value']
                }
            }
        },
        {
            type: 'function',
            function: {
                name: 'get_state',
                description: '查看当前页面状态：URL、标题、就绪状态、视口尺寸、滚动位置。',
                parameters: { type: 'object', properties: {}, required: [] }
            }
        },
        {
            type: 'function',
            function: {
                name: 'wait',
                description: '等待指定毫秒数（用于页面加载、动画或异步刷新后）。',
                parameters: {
                    type: 'object',
                    properties: {
                        ms: { type: 'integer', description: '等待毫秒数' }
                    },
                    required: ['ms']
                }
            }
        }
    ];

    // ================= 系统提示词（专用角色） =================

    function buildSystemPrompt() {
        return [
            '你是星月智能「通用页面操作」的专用智能体（AtoA 执行者），负责解析月华发来的自然语言指令，',
            '并作为当前页面的最终执行者，通过页面操作工具完成用户要求的点击、输入、按键、滑动、悬停、',
            '下拉选择、元素捕获、页面状态查看等操作。',
            '',
            '【能力范围】',
            '在当前页面内：点击/打开元素、向输入框输入文本、模拟按键（含组合键）、滚动/滑动（方向/距离/到元素）、',
            '悬停、下拉选择、捕获可见可交互元素、查看页面状态、等待。',
            '',
            '【执行规则】',
            '1. 每轮用户消息会附上【页面信息】【页面元素】【月华指令】；【页面元素】是当前可见可交互元素的紧凑列表（含标签/文本/选择器）。',
            '2. 操作目标以【页面元素】或 capture_page 返回的列表为准，优先使用其中的 selector（如 #submit、button:nth-of-type(1)）；',
            '   没有合适选择器时可用元素文本或 aria-label 描述作为 target（会做文本匹配）。',
            '3. 找不到目标时，先调用 capture_page 确认实际元素，再重试；确实不存在则如实说明，不要臆造。',
            '4. 需要连续执行多个按键/点击/输入时，**在尽可能少的同一轮内一次性返回多个 tool_calls**（例如"按 A、B、C 再回车"应一次返回 4 个 press_key 调用），而不是挤成单个参数。',
            '5. 每条指令允许至多 20 轮工具调用循环；多步任务依序完成，未完成不要提前总结，工具执行结果会回传给你。',
            '6. press_key 有三种语义：默认/无前缀=键入（瞬间按下即松开，等价一次点击），short:=短按（按住 1 秒），long: 或 hold:=长按（按住 10 秒）。需要"只点一下"用无前缀；需要短暂按住用 short:；需要持续操控/长按用 long:。',
            '7. 全部操作完成后，用一两句简洁的中文总结做了什么与结果，不要输出额外内容或代码块。'
        ].join('\n');
    }

    /* ---------------- 模型调用（OpenAI v1 协议，琉璃后端 /v1 代理） ---------------- */

    async function callAgentModel(messages) {
        const cfg = await loadModelConfig();
        const response = await fetch('/v1/chat/completions', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                model: cfg.model,
                messages: messages,
                tools: agentTools,
                stream: false
            })
        });
        if (!response.ok) throw new Error('AI 调用失败: HTTP ' + response.status);
        const data = await response.json();
        if (data.choices && data.choices[0] && data.choices[0].message) {
            return data.choices[0].message;
        }
        if (data.success && data.data && data.data.choices && data.data.choices[0]) {
            return data.data.choices[0].message;
        }
        throw new Error('AI 响应格式异常');
    }

    function safeParseArgs(jsonStr) {
        if (jsonStr && typeof jsonStr === 'object') return jsonStr;
        if (!jsonStr) return {};
        try {
            const parsed = JSON.parse(jsonStr);
            return parsed && typeof parsed === 'object' ? parsed : {};
        } catch (e) {
            return {};
        }
    }

    /* ---------------- 工具执行器（结构化参数 → 页面操作原语） ---------------- */

    function execCapturePage(args) {
        const items = captureElements({ max: (args && args.max) || AGENT_MAX_LIST_ITEMS });
        if (items.length === 0) return { success: true, text: '页面上没有可见可交互元素' };
        return { success: true, text: buildCompactElements(items) };
    }

    function execClick(args) {
        const res = clickEl(args && args.target);
        return res.ok ? { success: true, text: res.info } : { success: false, error: res.reason };
    }

    function execType(args) {
        const res = typeEl(args && args.target, args && args.text);
        return res.ok ? { success: true, text: res.info } : { success: false, error: res.reason };
    }

    async function execPressKey(args) {
        const res = await pressKeyEl(args && args.key);
        return res.ok ? { success: true, text: res.info } : { success: false, error: res.reason };
    }

    function execScroll(args) {
        const target = (args && args.target) || undefined;
        const direction = (args && args.direction) || (target ? undefined : 'down');
        const res = target ? scrollEl(target, args && args.amount) : scrollEl(direction, args && args.amount);
        return res.ok ? { success: true, text: res.info } : { success: false, error: res.reason };
    }

    function execHover(args) {
        const res = hoverEl(args && args.target);
        return res.ok ? { success: true, text: res.info } : { success: false, error: res.reason };
    }

    function execSelectOption(args) {
        const res = selectEl(args && args.target, args && args.value);
        return res.ok ? { success: true, text: res.info } : { success: false, error: res.reason };
    }

    function execGetState() {
        const s = getState();
        return { success: true, text: buildPageInfoText() };
    }

    function execWait(args) {
        const ms = Math.max(0, parseInt(args && args.ms, 10) || 0);
        return new Promise((resolve) => {
            setTimeout(() => resolve({ success: true, text: '已等待 ' + ms + 'ms' }), ms);
        });
    }

    const toolExecutors = {
        capture_page: execCapturePage,
        click: execClick,
        type: execType,
        press_key: execPressKey,
        scroll: execScroll,
        hover: execHover,
        select_option: execSelectOption,
        get_state: execGetState,
        wait: execWait
    };

    async function executeTool(name, args) {
        const executor = toolExecutors[name];
        if (!executor) return { success: false, error: '未知工具: ' + name };
        try {
            return await executor(args || {});
        } catch (e) {
            return { success: false, error: (e && e.message) || String(e) };
        }
    }

    /* ---------------- 主流程：多轮工具调用循环 ---------------- */

    async function runLTPXAgent(instruction) {
        const text = String(instruction || '').trim();
        if (!text) throw new Error('空指令');

        // 组装本轮用户消息：页面信息 + 页面元素 + 月华指令（供模型识别目标与选择器）
        const userParts = [
            { type: 'text', text: '【页面信息】' + buildPageInfoText() },
            { type: 'text', text: '【页面元素】\n' + buildCompactElements(captureElements({ max: AGENT_MAX_LIST_ITEMS })) },
            { type: 'text', text: '【月华指令】' + text }
        ];

        // 消息骨架：系统提示 + 独立上下文历史（最近 N 轮）+ 本轮
        const messages = [{ role: 'system', content: buildSystemPrompt() }];
        for (const round of agentHistory) {
            messages.push({ role: 'user', content: round.user });
            messages.push({ role: 'assistant', content: round.assistant });
        }
        messages.push({ role: 'user', content: userParts });

        let lastReply = '';
        for (let loop = 0; loop < AGENT_MAX_TOOL_LOOPS; loop++) {
            const message = await callAgentModel(messages);
            const toolCalls = Array.isArray(message.tool_calls) ? message.tool_calls : [];

            // 无工具调用：智能体给出最终答复
            if (toolCalls.length === 0) {
                lastReply = String(message.content || '').trim() || '已完成';
                break;
            }

            // 记录助手工具调用（供工具结果回填时关联）
            messages.push({
                role: 'assistant',
                content: message.content || '',
                tool_calls: toolCalls.map(tc => ({
                    id: tc.id,
                    type: 'function',
                    function: { name: tc.function.name, arguments: tc.function.arguments }
                }))
            });

            // 依次执行工具，将结果作为 tool 消息回填
            for (const tc of toolCalls) {
                const args = safeParseArgs(tc.function.arguments);
                const result = await executeTool(tc.function.name, args);
                messages.push({
                    role: 'tool',
                    tool_call_id: tc.id,
                    content: JSON.stringify(result)
                });
            }
        }

        if (!lastReply) lastReply = '已完成相关页面操作';

        // 记录本轮对话并裁剪历史
        agentHistory.push({ user: text, assistant: lastReply });
        if (agentHistory.length > AGENT_MAX_ROUNDS) {
            agentHistory.splice(0, agentHistory.length - AGENT_MAX_ROUNDS);
        }
        return lastReply;
    }

    /* ---------------- 对外主接口：自然语言输入 → 自然语言输出 ---------------- */

    async function run(instruction, opts) {
        opts = opts || {};
        if (!instruction || !instruction.trim()) return '未提供指令，请输入要执行的操作。';
        const text = String(instruction).trim();
        // LTPX AtoA 主路径：LLM 智能体（独立上下文 + 系统提示词 + function calling 多轮执行）。
        // 月华在线时模型服务必然在线（调用由月华经 /v1 转发），因此不保留离线规则回退。
        return await runLTPXAgent(text);
    }

    /* ---------------- AtoA 集成层 ---------------- */

    function enableAtoA(opts) {
        if (window.__miniLTPAtoAReady) return publicApi;
        window.__miniLTPAtoAReady = true;
        window.addEventListener('message', async function (event) {
            const data = event.data;
            if (!data || typeof data !== 'object' || data.type !== 'ltpx_run') return;
            const requestId = data.request_id;
            const args = data.arguments || {};
            const instruction = args.instruction || '';
            try {
                const text = await run(instruction, opts || {});
                postResult(requestId, true, text, '');
            } catch (e) {
                postResult(requestId, false, '', String(e && e.message || e));
            }
        });
        return publicApi;
    }

    function postResult(requestId, success, text, error) {
        try {
            if (!window.parent) return;
            window.parent.postMessage({
                type: 'ltpx_result',
                request_id: requestId,
                success: !!success,
                text: text || '',
                error: error || '',
                // 页面操作型智能体执行后保持页面打开：让用户看到操作结果、继续观察/交互
                keep_open: true
            }, '*');
        } catch (e) { /* 父窗口不可达时静默 */ }
    }

    var publicApi = {
        version: VERSION,
        run: run,               // 自然语言输入接口 → 自然语言输出（Promise<string>）
        enableAtoA: enableAtoA, // 启用/再次启用 AtoA 集成
        // ---- 底层页面操作原语（供工具执行器与宿主扩展/测试） ----
        capture: captureElements,
        click: clickEl,
        type: typeEl,
        pressKey: pressKeyEl,
        scroll: scrollEl,
        hover: hoverEl,
        select: selectEl,
        getState: getState,
        // ---- 内部工具 ----
        query: query,
        queryAll: queryAll,
        isVisible: isVisible,
        resolveTarget: resolveTarget,
        // ---- 智能体状态 ----
        resetHistory: function () { agentHistory = []; return agentHistory.length; },
        // ---- 模型配置（从 lunar_config.json 读取） ----
        getModelConfig: loadModelConfig,
        refreshModelConfig: refreshModelConfig
    };

    // 位于 iframe 中（即被琉璃等宿主嵌入）时自动启用 AtoA 集成
    if (typeof window !== 'undefined' && window !== window.top && window.parent) {
        enableAtoA();
    }

    return publicApi;
});
