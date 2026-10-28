/**
 * 编辑运行 - 页面逻辑
 * 职责：
 * - 代码编辑器：单文件 JavaScript（ES2023+，浏览器 WebAPI）与完整 HTML 文档的编写
 * - 运行器：JS 包装为带控制台桥接的沙箱页面运行，HTML 注入桥接后直接渲染（iframe srcdoc）
 * - 控制台输出：沙箱内 console/error 经 postMessage 实时回传展示
 * - 文件管理：文档库（documents/）按原文件名读写，保存后自动登记资源句柄
 * - 人机协同：月华生成的代码同步到编辑器，用户可修改后再次运行
 * - LTPX AtoA：接收琉璃主窗口投递的 ltpx_run 指令，驱动智能体完成"写码 → 运行 → 修正"循环
 */

// ===== DOM 引用 =====
const codeEditor = document.getElementById('codeEditor');
const lineNumbers = document.getElementById('lineNumbers');
const fileNameInput = document.getElementById('fileNameInput');
const codeStats = document.getElementById('codeStats');
const runBtn = document.getElementById('runBtn');
const stopBtn = document.getElementById('stopBtn');
const saveBtn = document.getElementById('saveBtn');
const newBtn = document.getElementById('newBtn');
const refreshFilesBtn = document.getElementById('refreshFilesBtn');
const fileList = document.getElementById('fileList');
const previewFrame = document.getElementById('previewFrame');
const previewStatus = document.getElementById('previewStatus');
const previewOverlay = document.getElementById('previewOverlay');
const previewCloseBtn = document.getElementById('previewCloseBtn');
const consoleList = document.getElementById('consoleList');
const clearConsoleBtn = document.getElementById('clearConsoleBtn');
const stepList = document.getElementById('stepList');
const clearStepBtn = document.getElementById('clearStepBtn');
const themeToggle = document.getElementById('themeToggle');
const themeIcon = document.getElementById('themeIcon');
const themeLabel = document.getElementById('themeLabel');

// ===== 常量 =====

/** 文档库目录（相对于 local_data，经 /file/* 接口读写） */
const DOCUMENTS_PATH = 'documents';
/** 可在编辑运行中打开/运行的文本类扩展名 */
const READABLE_EXTENSIONS = new Set([
    'txt', 'md', 'markdown', 'json', 'xml', 'yaml', 'yml', 'toml', 'ini', 'cfg', 'conf',
    'log', 'csv', 'tsv', 'go', 'ts', 'tsx', 'js', 'jsx', 'mjs', 'cjs', 'py', 'rs', 'java',
    'kt', 'c', 'h', 'cpp', 'cxx', 'hpp', 'cs', 'rb', 'php', 'swift', 'sh', 'ps1', 'bat',
    'cmd', 'sql', 'html', 'htm', 'css', 'vue', 'lua', 'r', 'code-workspace'
]);
/** run_code 运行后收集控制台输出的等待时长（毫秒） */
const OUTPUT_WAIT_MS = 2000;
/** 控制台输出最大条数（超出丢弃最早的） */
const CONSOLE_MAX = 300;

// ===== 深色/浅色模式切换 =====
const THEME_KEY = 'csTheme';

function applyTheme(dark) {
    document.body.classList.toggle('dark-mode', dark);
    if (themeIcon) themeIcon.className = 'fas ' + (dark ? 'fa-sun' : 'fa-moon');
    if (themeLabel) themeLabel.textContent = dark ? '浅色' : '深色';
}
if (themeToggle) {
    themeToggle.addEventListener('click', () => {
        const dark = !document.body.classList.contains('dark-mode');
        applyTheme(dark);
        try { localStorage.setItem(THEME_KEY, dark ? 'dark' : 'light'); } catch (e) { }
    });
}
applyTheme(localStorage.getItem(THEME_KEY) === 'dark');

// ===== Toast =====

function showToast(text, kind) {
    const toast = document.getElementById('toast');
    if (!toast) return;
    toast.textContent = text;
    toast.className = 'toast visible' + (kind ? ' ' + kind : '');
    clearTimeout(showToast._timer);
    showToast._timer = setTimeout(() => { toast.className = 'toast'; }, 2600);
}

// ===== 操作记录面板 =====
const STEP_ICONS = {
    info: 'fa-circle-info',
    thinking: 'fa-brain',
    tool: 'fa-wrench',
    'tool-result': 'fa-arrow-right',
    done: 'fa-check-circle',
    error: 'fa-circle-xmark',
    warn: 'fa-triangle-exclamation'
};

function appendStep(kind, text) {
    const empty = stepList.querySelector('.step-empty');
    if (empty) empty.remove();
    const item = document.createElement('div');
    item.className = 'step-item step-' + kind;
    const time = new Date().toLocaleTimeString('zh-CN', { hour12: false });
    item.innerHTML = '<span class="step-icon"><i class="fas ' + (STEP_ICONS[kind] || STEP_ICONS.info) + '"></i></span>'
        + '<span class="step-text"></span>'
        + '<span class="step-time">' + time + '</span>';
    item.querySelector('.step-text').textContent = text;
    stepList.appendChild(item);
    stepList.scrollTop = stepList.scrollHeight;
}

function clearStepPanel() {
    stepList.innerHTML = '<div class="list-empty">暂无操作记录。</div>';
}

clearStepBtn.addEventListener('click', clearStepPanel);

// ===== 文件管理（文档库：原文件名读写 + 自动登记句柄） =====

/** 文件名编码（与琉璃文件接口约定一致：encodeURIComponent 后按字节 btoa） */
function encodeFileName(filename) {
    const encodedParams = encodeURIComponent(filename);
    const decodedParams = encodedParams.replace(/%([0-9A-F]{2})/g, (_, p1) => String.fromCharCode(parseInt(p1, 16)));
    return btoa(decodedParams);
}

/** 规范化文件名：去路径、禁 ..、按内容补扩展名 */
function normalizeFileName(name, content) {
    let n = String(name || '').trim().replace(/\\/g, '/').replace(/^\/+/, '');
    if (n.includes('..')) throw new Error('文件名不能包含 ..');
    if (n.includes('/')) n = n.split('/').pop();
    if (!n) n = detectLanguage('', content) === 'html' ? 'index.html' : 'script.js';
    if (!/\.[A-Za-z0-9]+$/.test(n)) n += detectLanguage('', content) === 'html' ? '.html' : '.js';
    return n;
}

/** 统一资源句柄正则（与资源目录文法一致，可带 |§n / |Lx-y 锚点） */
const RESOURCE_HANDLE_RE = /^@[DIVATU]\.\d{3,}(?:\|(?:§\d+|L\d+-\d+))?$/;

/**
 * 把「文件名或资源句柄」解析为文档库中的实际文件名
 *
 * - 普通文件名：直接返回（兼容历史 # 前缀）
 * - @D.xxx：调用 /file/resource/resolve 取 blob_path 的 basename
 *
 * @param {string} input - 文件名或资源句柄
 * @returns {Promise<string>} 文档库中的实际文件名
 */
async function resolveFileReference(input) {
    const raw = String(input || '').trim();
    if (!RESOURCE_HANDLE_RE.test(raw)) return raw.replace(/^#/, '');
    const resp = await fetch('/file/resource/resolve?handle=' + encodeURIComponent(raw), { cache: 'no-store' });
    if (!resp.ok) {
        const message = await resp.text().catch(() => '');
        throw new Error('句柄解析失败：' + raw
            + (message ? '（' + message.trim() + '）' : '（HTTP ' + resp.status + '）'));
    }
    const data = await resp.json();
    const res = data && (data.resource || data);
    if (!res || res.kind !== 'D') throw new Error('该句柄不是文档资源：' + raw);
    const blob = String(res.blob_path || '');
    if (blob && !blob.startsWith('documents/')) {
        throw new Error('该句柄资源不在文档库目录，无法在编辑运行中编辑：' + raw);
    }
    const blobName = blob ? blob.split('/').pop() : '';
    const name = blobName || String(res.name || '').split('/').pop();
    if (!name) throw new Error('句柄未关联可编辑文件：' + raw);
    return name;
}

/** 解析并规范化文件名（支持资源句柄） */
async function normalizeFileTarget(name) {
    const raw = String(name || '').trim();
    const resolved = await resolveFileReference(raw);
    // 句柄解析出的实际文件名原样使用，不按内容补扩展名
    if (RESOURCE_HANDLE_RE.test(raw)) return resolved;
    return normalizeFileName(resolved, '');
}

/** 写入指定相对路径（POST /file/write，X-File-Name 相对 local_data） */
function saveFileToPath(relativePath, content) {
    return new Promise((resolve, reject) => {
        const xhr = new XMLHttpRequest();
        xhr.addEventListener('load', () => {
            if (xhr.status === 200) resolve();
            else reject(new Error(xhr.responseText || '保存失败（HTTP ' + xhr.status + '）'));
        });
        xhr.addEventListener('error', () => reject(new Error('保存失败')));
        xhr.addEventListener('timeout', () => reject(new Error('保存超时')));
        xhr.open('POST', '/file/write');
        xhr.setRequestHeader('X-File-Name', encodeFileName(relativePath));
        xhr.setRequestHeader('X-Overwrite', 'true');
        xhr.send(new Blob([content], { type: 'text/plain; charset=utf-8' }));
    });
}

/** 读取指定相对路径的文本（GET /file/read） */
async function readPathText(relativePath) {
    const urlPath = String(relativePath).split('/').map(encodeURIComponent).join('/');
    const resp = await fetch('/file/read/' + urlPath, { cache: 'no-store' });
    if (!resp.ok) throw new Error('读取文件失败：' + relativePath + '（HTTP ' + resp.status + '）');
    return await resp.text();
}

/** 列出指定目录（GET /file/list），失败返回空数组 */
async function listDirectory(relativePath) {
    try {
        const resp = await fetch('/file/list/' + relativePath, { cache: 'no-store' });
        if (!resp.ok) return [];
        const entries = await resp.json();
        return Array.isArray(entries) ? entries : [];
    } catch (e) {
        return [];
    }
}

/** 是否为可在编辑运行中打开的文本类文件 */
function isReadableFile(name) {
    const lower = String(name || '').toLowerCase();
    if (lower === 'dockerfile' || lower === '.env' || lower === '.gitignore' || lower === '.editorconfig') return true;
    const match = /\.([A-Za-z0-9]+)$/.exec(lower);
    return !!match && READABLE_EXTENSIONS.has(match[1]);
}

/** 读取文档库文件内容（原文件名直接寻址） */
async function fetchFileText(name) {
    return await readPathText(DOCUMENTS_PATH + '/' + name);
}

/**
 * 为文档库文件登记资源句柄
 *
 * 后端以「documents/原文件名」为业务身份：同一路径始终复用同一句柄，
 * 内容变化时刷新哈希与段落表，因此保存后调用即可。
 * 端点不可用时仅告警，不阻断文件保存。
 *
 * @param {string} name - 原文件名
 * @returns {Promise<string>} 句柄；失败返回空串
 */
async function requestFileHandle(name) {
    try {
        const resp = await fetch('/file/resource/admit', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ path: DOCUMENTS_PATH + '/' + name, name: name })
        });
        if (!resp.ok) throw new Error('HTTP ' + resp.status);
        const data = await resp.json();
        return String((data.resource && data.resource.handle) || data.handle || '');
    } catch (e) {
        console.warn('[编辑运行] 申请资源句柄失败:', e);
        return '';
    }
}

/** 列出文档库中全部可读文件名（供智能体工具复用） */
async function listDocumentFiles() {
    const entries = await listDirectory(DOCUMENTS_PATH);
    return entries
        .filter(e => e && !e.isDir && isReadableFile(e.name))
        .map(e => String(e.name))
        .sort((a, b) => String(a).localeCompare(String(b), 'zh-CN'));
}

/** 刷新文件列表：直接展示文档库中的全部可读文件 */
async function renderFileList() {
    try {
        const entries = await listDirectory(DOCUMENTS_PATH);
        const list = entries
            .filter(e => e && !e.isDir && isReadableFile(e.name))
            .sort((a, b) => String(a.name).localeCompare(String(b.name), 'zh-CN'));
        fileList.innerHTML = '';
        if (list.length === 0) {
            fileList.innerHTML = '<div class="list-empty">暂无文件。</div>';
            return;
        }
        list.forEach(f => {
            const name = String(f.name || '');
            const el = document.createElement('div');
            el.className = 'file-item' + (fileNameInput.value.trim() === name ? ' active' : '');
            const iconCls = /\.html?$/i.test(name) ? 'f-html' : (/\.js$/i.test(name) ? 'f-js' : 'f-other');
            const icon = /\.html?$/i.test(name) ? 'fa-file-code' : (/\.js$/i.test(name) ? 'fa-js' : 'fa-file');
            const size = typeof f.size === 'number' ? (f.size / 1024).toFixed(1) + ' KB' : '';
            el.innerHTML = '<span class="file-icon ' + iconCls + '"><i class="fas ' + icon + '"></i></span>'
                + '<span class="file-name"></span>'
                + '<span class="file-meta">' + size + '</span>';
            el.querySelector('.file-name').textContent = name;
            el.addEventListener('click', async () => {
                try {
                    const text = await fetchFileText(name);
                    fileNameInput.value = name;
                    codeEditor.value = text;
                    updateStats();
                    renderFileList();
                    appendStep('info', '载入文件：' + name);
                } catch (err) {
                    showToast(err.message, 'error');
                }
            });
            fileList.appendChild(el);
        });
    } catch (e) {
        fileList.innerHTML = '';
        const empty = document.createElement('div');
        empty.className = 'list-empty';
        empty.textContent = '文件列表加载失败：' + (e.message || e);
        fileList.appendChild(empty);
    }
}

refreshFilesBtn.addEventListener('click', async () => {
    await renderFileList();
    showToast('文件列表已刷新', 'success');
});

// ===== 语言检测与运行器 =====

/** 按扩展名与内容识别类型：'js' | 'html' */
function detectLanguage(name, content) {
    if (/\.html?$/i.test(String(name || ''))) return 'html';
    const head = String(content || '').slice(0, 200).trim().toLowerCase();
    if (head.startsWith('<!doctype') || head.startsWith('<html')) return 'html';
    return 'js';
}

/** 控制台桥接脚本：沙箱内 console/异常经 postMessage 回传父页面 */
const CONSOLE_BRIDGE = [
    '(function(){',
    '  function fmt(v){ if (typeof v === "string") return v; if (v instanceof Error) return (v.stack || v.message); try { return JSON.stringify(v); } catch (e) { return String(v); } }',
    '  function send(level, args){ try { parent.postMessage({ type: "code_console", level: level, text: Array.prototype.map.call(args, fmt).join(" ") }, "*"); } catch (e) {} }',
    '  ["log","info","warn","error","debug"].forEach(function(k){ var o = console[k] ? console[k].bind(console) : function(){}; console[k] = function(){ send(k === "debug" ? "log" : k, arguments); o.apply(null, arguments); }; });',
    '  window.addEventListener("error", function(e){ send("error", [String(e.message) + (e.lineno ? " (行 " + e.lineno + ")" : "")]); });',
    '  window.addEventListener("unhandledrejection", function(e){ var r = e.reason; send("error", ["未处理的 Promise 拒绝: " + ((r && (r.stack || r.message)) || r)]); });',
    '})();'
].join('\n');

/** JS 单文件包装为带控制台桥接的运行页面（转义 </script> 防止提前闭合） */
function buildJSRunner(js) {
    const safe = String(js).replace(/<\/script>/gi, '<\\/script>');
    return '<!DOCTYPE html><html lang="zh-CN"><head><meta charset="UTF-8"><script>' + CONSOLE_BRIDGE + '</script></head><body><script>' + safe + '</script></body></html>';
}

/** HTML 文档注入控制台桥接（有 head 插 head，有 html 插 html，纯片段前置） */
function buildHTMLRunner(html) {
    const bridge = '<script>' + CONSOLE_BRIDGE + '</script>';
    const headMatch = String(html).match(/<head[^>]*>/i);
    if (headMatch) return html.replace(headMatch[0], headMatch[0] + bridge);
    const htmlMatch = html.match(/<html[^>]*>/i);
    if (htmlMatch) return html.replace(htmlMatch[0], htmlMatch[0] + '<head>' + bridge + '</head>');
    return bridge + html;
}

// ===== 控制台输出面板 =====

let consoleCollector = null; // 运行期间收集输出的回调（供 run_code 回读）

function appendConsole(level, text) {
    const empty = consoleList.querySelector('.list-empty');
    if (empty) empty.remove();
    // 超出上限丢弃最早的
    const items = consoleList.querySelectorAll('.console-item');
    if (items.length >= CONSOLE_MAX) items[0].remove();
    const item = document.createElement('div');
    item.className = 'console-item console-' + (level || 'log');
    const time = new Date().toLocaleTimeString('zh-CN', { hour12: false });
    item.innerHTML = '<span class="console-level"></span>'
        + '<span class="console-text"></span>'
        + '<span class="console-time">' + time + '</span>';
    item.querySelector('.console-level').textContent = level || 'log';
    item.querySelector('.console-text').textContent = text;
    consoleList.appendChild(item);
    consoleList.scrollTop = consoleList.scrollHeight;
}

function clearConsole() {
    consoleList.innerHTML = '<div class="list-empty">暂无输出。</div>';
}

clearConsoleBtn.addEventListener('click', clearConsole);

// 沙箱控制台消息回传监听
window.addEventListener('message', (event) => {
    const d = event.data;
    if (!d || typeof d !== 'object' || d.type !== 'code_console') return;
    const text = String(d.text || '');
    if (consoleCollector) consoleCollector(d.level || 'log', text);
    appendConsole(d.level || 'log', text);
});

/** 隐藏悬浮预览（运行仍在后台继续，停止按钮会同时结束运行） */
function hidePreview() {
    previewOverlay.classList.remove('visible');
}

/** 显示悬浮预览 */
function showPreview() {
    previewOverlay.classList.add('visible');
}

/** 清空预览并隐藏 */
function showEmptyPreview() {
    previewFrame.srcdoc = '';
    previewStatus.textContent = '';
    hidePreview();
}

previewCloseBtn.addEventListener('click', hidePreview);

// Esc 也可关闭预览，避免预览窗占用键盘焦点后无处退出
document.addEventListener('keydown', (event) => {
    if (event.key === 'Escape' && previewOverlay.classList.contains('visible')) {
        hidePreview();
    }
});

/**
 * 是否需要展示可视化预览
 *
 * HTML 文档一定需要；JS 只有在触碰 DOM / Canvas / 动画等会改变页面外观的 API 时
 * 才自动弹出预览，纯算法脚本仅把输出留在底部控制台。
 *
 * @param {string} name - 文件名
 * @param {string} content - 代码内容
 * @returns {boolean}
 */
function needsVisualPreview(name, content) {
    if (detectLanguage(name, content) === 'html') return true;
    const text = String(content || '');
    return /(document\s*[.\[]|window\s*\.\s*(?:document|location|on)|canvas|requestAnimationFrame|innerHTML|outerHTML|appendChild|insertBefore|createElement|querySelector|getElementById|document\.body|style\s*[.=])/i.test(text);
}

/**
 * 运行代码：构建沙箱文档并收集控制台输出
 * @param {string} content - 代码内容（JS 或 HTML）
 * @param {string} [name] - 文件名（用于类型识别与展示）
 * @returns {Promise<string[]>} 控制台输出行（"level: text"）
 */
function runCode(content, name) {
    return new Promise((resolve) => {
        const lang = detectLanguage(name, content);
        const doc = lang === 'html' ? buildHTMLRunner(content) : buildJSRunner(content);
        clearConsole();
        const label = name || (lang === 'html' ? 'HTML 文档' : 'JS 脚本');
        previewStatus.textContent = '运行中：' + label;
        const lines = [];
        consoleCollector = (level, text) => { lines.push(level + ': ' + text); };
        // 需要可视化的场景才弹出悬浮预览；纯算法脚本保持隐藏
        if (needsVisualPreview(name, content)) showPreview();
        else hidePreview();
        previewFrame.srcdoc = doc;
        setTimeout(() => {
            consoleCollector = null;
            previewStatus.textContent = '已运行：' + label + '（捕获输出 ' + lines.length + ' 行）';
            resolve(lines);
        }, OUTPUT_WAIT_MS);
    });
}

/** 停止运行（清空沙箱页面并隐藏预览） */
function stopRun() {
    previewFrame.srcdoc = '';
    setTimeout(showEmptyPreview, 50);
    previewStatus.textContent = '已停止';
    hidePreview();
}

// ===== 人机协同核心操作（智能体工具与手动按钮共用） =====

/** 最近一次运行汇总（供 LTPX 回执） */
let lastRunSummary = '';

/**
 * 写入代码到文档库并同步到编辑器
 * 文件按原文件名保存，保存后自动登记/刷新资源句柄（同一路径复用同一句柄）
 * @param {string} name - 文件名（无扩展名时按内容自动补）
 * @param {string} content - 完整文件内容
 * @returns {Promise<string>} 规范化后的文件名
 */
async function writeCodeFile(name, content) {
    const body = String(content || '');
    if (!body.trim()) throw new Error('代码内容为空');
    const fileName = await normalizeFileTarget(name);
    await saveFileToPath(DOCUMENTS_PATH + '/' + fileName, body);
    const handle = await requestFileHandle(fileName);
    fileNameInput.value = fileName;
    codeEditor.value = body;
    updateStats();
    renderFileList();
    appendStep('tool-result', '已保存 ' + fileName + '（' + body.length + ' 字符）'
        + (handle ? '，句柄 ' + handle : '（句柄登记失败，稍后可重试保存）') + ' 并同步到编辑器');
    return { name: fileName, handle: handle };
}

/**
 * 读取文档库文件（同时载入编辑器，供人查看与修改）
 * @param {string} name - 文件名
 * @returns {Promise<{name: string, content: string}>}
 */
async function readCodeFile(name) {
    const fileName = await normalizeFileTarget(name);
    const content = await fetchFileText(fileName);
    fileNameInput.value = fileName;
    codeEditor.value = content;
    updateStats();
    renderFileList();
    appendStep('tool-result', '读取文件：' + fileName + '（' + content.length + ' 字符）');
    return { name: fileName, content: content };
}

// ===== 行级读取与编辑（行区间文法与资源句柄的 |L 锚点一致） =====

/**
 * 解析行区间：L10-20 / L10 / L10-10，1 基、含端点
 * @param {string} raw - 行区间文本
 * @returns {{start:number,end:number,text:string}}
 */
function parseLineRange(raw) {
    const text = String(raw || '').trim().replace(/^\|/, '');
    const match = /^L(\d+)(?:-L?(\d+))?$/i.exec(text);
    if (!match) throw new Error('行区间格式应为 L10-20 或 L10（单行）');
    const start = parseInt(match[1], 10);
    const end = match[2] ? parseInt(match[2], 10) : start;
    if (!Number.isFinite(start) || !Number.isFinite(end) || start < 1 || end < start) {
        throw new Error('行区间非法：起始行必须 ≥1 且不大于结束行');
    }
    return { start: start, end: end, text: start === end ? 'L' + start : 'L' + start + '-' + end };
}

/** 拆分文档为行数组，并保留原始结尾是否有换行 */
function splitDocumentText(text) {
    const normalized = String(text == null ? '' : text).replace(/\r\n/g, '\n');
    const trailing = normalized.endsWith('\n');
    const lines = normalized.split('\n');
    if (trailing) lines.pop();
    return { lines: lines, trailing: trailing };
}

/** 按原始结尾风格拼回文本 */
function joinDocumentText(lines, trailing) {
    if (lines.length === 0) return '';
    return lines.join('\n') + (trailing ? '\n' : '');
}

/** 把工具传入的 content 拆成行（末尾换行不额外算一行） */
function contentToLines(content) {
    const normalized = String(content == null ? '' : content).replace(/\r\n/g, '\n');
    if (normalized === '') return [];
    const trailing = normalized.endsWith('\n');
    const lines = normalized.split('\n');
    if (trailing) lines.pop();
    return lines;
}

/** 读取文档并校验行区间 */
async function readDocumentRange(fileName, range) {
    const content = await fetchFileText(fileName);
    const doc = splitDocumentText(content);
    if (doc.lines.length === 0) throw new Error('文件为空，没有可读取的行');
    if (range.end > doc.lines.length) {
        throw new Error('行区间超出文件范围：' + fileName + ' 共 ' + doc.lines.length + ' 行');
    }
    return { content: content, lines: doc.lines, trailing: doc.trailing };
}

/** 把修改后的全文写回文档库，并同步编辑器与句柄 */
async function applyDocumentText(fileName, text) {
    await saveFileToPath(DOCUMENTS_PATH + '/' + fileName, text);
    const handle = await requestFileHandle(fileName);
    fileNameInput.value = fileName;
    codeEditor.value = text;
    updateStats();
    renderFileList();
    return handle;
}

/** 带行号的区间文本（行号右对齐，便于模型定位） */
function formatNumberedLines(lines, start) {
    const width = String(start + lines.length - 1).length;
    return lines.map((line, index) => String(start + index).padStart(width, ' ') + '| ' + line).join('\n');
}

/**
 * 读取指定行区间
 * @param {string} name - 文件名
 * @param {string} rangeText - L10-20 或 L10
 * @returns {Promise<string>} 带行号的区间文本
 */
async function readCodeLines(name, rangeText) {
    const fileName = await normalizeFileTarget(name);
    const range = parseLineRange(rangeText);
    const doc = await readDocumentRange(fileName, range);
    const selected = doc.lines.slice(range.start - 1, range.end);
    fileNameInput.value = fileName;
    codeEditor.value = doc.content;
    updateStats();
    renderFileList();
    const header = fileName + ' ' + range.text + '（共 ' + selected.length + ' 行）';
    const body = formatNumberedLines(selected, range.start);
    appendStep('tool-result', '读取 ' + header);
    const text = header + '\n' + body;
    return text.length > 16000
        ? text.slice(0, 16000) + '\n...（区间内容过长已截断，请缩小行区间）'
        : text;
}

/**
 * 按行写入：content 的行数必须与目标区间长度一致（原位覆盖）
 * @param {string} name - 文件名
 * @param {string} rangeText - 目标行区间
 * @param {string} content - 新内容（行数须与区间一致）
 */
async function writeCodeLines(name, rangeText, content) {
    const fileName = await normalizeFileTarget(name);
    const range = parseLineRange(rangeText);
    const doc = await readDocumentRange(fileName, range);
    const newLines = contentToLines(content);
    const expected = range.end - range.start + 1;
    if (newLines.length !== expected) {
        throw new Error('write_lines 要求 content 行数与行区间一致（' + expected + ' 行），当前 '
            + newLines.length + ' 行；如需改变行数请用 replace_lines');
    }
    doc.lines.splice(range.start - 1, expected, ...newLines);
    const text = joinDocumentText(doc.lines, doc.trailing);
    const handle = await applyDocumentText(fileName, text);
    appendStep('tool-result', '按行写入 ' + fileName + ' ' + range.text + '（' + expected + ' 行）'
        + (handle ? '，句柄 ' + handle : ''));
    return { name: fileName, handle: handle, lines: doc.lines.length, range: range.text };
}

/**
 * 按行置换：用 content 替换目标区间，行数可任意增减
 * @param {string} name - 文件名
 * @param {string} rangeText - 目标行区间
 * @param {string} content - 新内容
 */
async function replaceCodeLines(name, rangeText, content) {
    const fileName = await normalizeFileTarget(name);
    const range = parseLineRange(rangeText);
    const doc = await readDocumentRange(fileName, range);
    const newLines = contentToLines(content);
    doc.lines.splice(range.start - 1, range.end - range.start + 1, ...newLines);
    const text = joinDocumentText(doc.lines, doc.trailing);
    const handle = await applyDocumentText(fileName, text);
    appendStep('tool-result', '置换 ' + fileName + ' ' + range.text + ' → ' + newLines.length + ' 行'
        + (handle ? '，句柄 ' + handle : ''));
    return { name: fileName, handle: handle, lines: doc.lines.length, range: range.text };
}

/**
 * 互换两个行段落（行号均以交换前的文件为准）
 * @param {string} name - 文件名
 * @param {string} rangeAText - 段落 A
 * @param {string} rangeBText - 段落 B
 */
async function swapCodeParagraphs(name, rangeAText, rangeBText) {
    const fileName = await normalizeFileTarget(name);
    const a = parseLineRange(rangeAText);
    const b = parseLineRange(rangeBText);
    const doc = await readDocumentRange(fileName, a);
    if (b.end > doc.lines.length) {
        throw new Error('第二个行区间超出文件范围：' + fileName + ' 共 ' + doc.lines.length + ' 行');
    }
    if (a.start <= b.end && b.start <= a.end) {
        throw new Error('两个段落不能重叠');
    }
    const first = a.start <= b.start ? a : b;
    const second = a.start <= b.start ? b : a;
    const firstBlock = doc.lines.slice(first.start - 1, first.end);
    const secondBlock = doc.lines.slice(second.start - 1, second.end);
    const rebuilt = [
        ...doc.lines.slice(0, first.start - 1),
        ...secondBlock,
        ...doc.lines.slice(first.end, second.start - 1),
        ...firstBlock,
        ...doc.lines.slice(second.end)
    ];
    const text = joinDocumentText(rebuilt, doc.trailing);
    const handle = await applyDocumentText(fileName, text);
    appendStep('tool-result', '互换段落 ' + fileName + ' ' + first.text + ' ↔ ' + second.text
        + (handle ? '，句柄 ' + handle : ''));
    return { name: fileName, handle: handle, lines: rebuilt.length, first: first.text, second: second.text };
}

/**
 * 运行代码（指定文件则先载入，否则运行编辑器当前内容），收集控制台输出
 * @param {string} [name] - 可选文件名
 * @returns {Promise<{lines: string[], summary: string}>}
 */
async function runWorkspaceFile(name) {
    let content, fileName;
    if (name) {
        fileName = await normalizeFileTarget(name);
        content = await fetchFileText(fileName);
        fileNameInput.value = fileName;
        codeEditor.value = content;
    } else {
        content = codeEditor.value;
        fileName = fileNameInput.value.trim();
    }
    if (!content.trim()) throw new Error('代码内容为空，请先编写或载入文件');
    updateStats();
    renderFileList();
    appendStep('tool', '运行 ' + (fileName || '未命名代码'));
    const lines = await runCode(content, fileName);
    const preview = lines.slice(0, 40);
    const summary = (lines.length ? preview.join('\n') : '（无控制台输出）')
        + (lines.length > preview.length ? '\n...（其余 ' + (lines.length - preview.length) + ' 行省略）' : '');
    lastRunSummary = '文件：' + (fileName || '未命名') + '\n控制台输出 ' + lines.length + ' 行：\n' + summary;
    const hasError = lines.some(l => l.startsWith('error:'));
    appendStep(hasError ? 'error' : 'tool-result', '运行完成，捕获输出 ' + lines.length + ' 行' + (hasError ? '（含错误）' : ''));
    return { lines: lines, summary: summary };
}

// 暴露给 ltpx-agent.js 工具执行器复用
window.writeCodeFile = writeCodeFile;
window.readCodeFile = readCodeFile;
window.readCodeLines = readCodeLines;
window.writeCodeLines = writeCodeLines;
window.replaceCodeLines = replaceCodeLines;
window.swapCodeParagraphs = swapCodeParagraphs;
window.runWorkspaceFile = runWorkspaceFile;
window.listDocumentFiles = listDocumentFiles;

// ===== 编辑器统计 =====

/** 刷新行号栏：行数与 textarea 视觉行一致（含末尾空行），并同步纵向滚动 */
function updateLineNumbers() {
    if (!lineNumbers) return;
    const count = Math.max(1, String(codeEditor.value || '').split('\n').length);
    if (lineNumbers.dataset.count !== String(count)) {
        const values = new Array(count);
        for (let i = 0; i < count; i++) values[i] = String(i + 1);
        lineNumbers.textContent = values.join('\n');
        lineNumbers.dataset.count = String(count);
    }
    lineNumbers.scrollTop = codeEditor.scrollTop;
}

function updateStats() {
    const content = codeEditor.value;
    const lines = content ? String(content).split('\n').length : 0;
    const lang = detectLanguage(fileNameInput.value, content);
    codeStats.textContent = (content ? lines + ' 行 · ' + content.length + ' 字符 · ' : '') + (lang === 'html' ? 'HTML' : 'JavaScript');
    updateLineNumbers();
}

codeEditor.addEventListener('input', updateStats);
codeEditor.addEventListener('scroll', () => {
    if (lineNumbers) lineNumbers.scrollTop = codeEditor.scrollTop;
});
fileNameInput.addEventListener('input', () => renderFileList());

// ===== 手动按钮 =====

runBtn.addEventListener('click', async () => {
    try {
        await runWorkspaceFile();
    } catch (e) {
        showToast(e.message, 'error');
        appendStep('error', '运行失败：' + e.message);
    }
});

stopBtn.addEventListener('click', () => {
    stopRun();
    appendStep('info', '停止运行');
});

saveBtn.addEventListener('click', async () => {
    try {
        const content = codeEditor.value;
        if (!content.trim()) { showToast('代码内容为空', 'error'); return; }
        const saved = await writeCodeFile(fileNameInput.value, content);
        showToast('已保存：' + saved.name + (saved.handle ? '（' + saved.handle + '）' : ''), 'success');
    } catch (e) {
        showToast(e.message, 'error');
    }
});

newBtn.addEventListener('click', () => {
    fileNameInput.value = '';
    codeEditor.value = '';
    updateStats();
    renderFileList();
    showEmptyPreview();
    appendStep('info', '新建空白文件');
    codeEditor.focus();
});

// ===== LTPX AtoA：接收琉璃主窗口投递的工具调用 =====

/** 回传执行结果给琉璃主窗口（琉璃再转交月华） */
function postLTPXResult(requestId, success, text, error) {
    try {
        window.parent.postMessage({
            type: 'ltpx_result',
            request_id: requestId,
            success: !!success,
            text: text || '',
            error: error || '',
            keep_open: true // 保持页面展示代码与运行结果，供用户查看与继续编辑
        }, '*');
    } catch (e) {
        console.error('LTPX 回传结果失败:', e);
    }
}

window.addEventListener('message', async (event) => {
    const data = event.data;
    if (!data || typeof data !== 'object' || data.type !== 'ltpx_run') return;

    // 立即回执：告诉宿主「指令已收到」，停止 90 秒派发心跳；
    // 智能体还要走多轮模型调用，最终结果仍由 ltpx_result 返回。
    try {
        window.parent.postMessage({ type: 'ltpx_ack', request_id: data.request_id }, '*');
    } catch (e) { }

    const instruction = (data.arguments || {}).instruction || '';
    clearStepPanel();
    lastRunSummary = '';
    try {
        const result = await runCodeAgent(instruction, (step) => appendStep(step.kind, step.text));
        // 回执 = 智能体答复 + 最近运行汇总（传回月华系统）
        let receipt = '【编辑运行】\n' + (result.text || '');
        if (lastRunSummary) receipt += '\n—— 最近运行 ——\n' + lastRunSummary;
        postLTPXResult(data.request_id, result.success, receipt, result.error);
    } catch (e) {
        console.error('LTPX AtoA 执行失败:', e);
        appendStep('error', '执行失败：' + (e.message || e));
        postLTPXResult(data.request_id, false, '', e.message || '执行失败');
    }
});

// ===== 初始化 =====
clearStepPanel();
clearConsole();
updateStats();
renderFileList();
showEmptyPreview();
