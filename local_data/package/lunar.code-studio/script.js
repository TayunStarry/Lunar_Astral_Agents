/**
 * 代码工坊 - 页面逻辑
 * 职责：
 * - 代码编辑器：单文件 JavaScript（ES2023+，浏览器 WebAPI）与完整 HTML 文档的编写
 * - 运行器：JS 包装为带控制台桥接的沙箱页面运行，HTML 注入桥接后直接渲染（iframe srcdoc）
 * - 控制台输出：沙箱内 console/error 经 postMessage 实时回传展示
 * - 文件管理：工作区文件保存（/file/write）、列表（/file/list）、读取（/file/read）
 * - 人机协同：月华生成的代码同步到编辑器，用户可修改后再次运行
 * - LTPX AtoA：接收琉璃主窗口投递的 ltpx_run 指令，驱动智能体完成"写码 → 运行 → 修正"循环
 */

// ===== DOM 引用 =====
const codeEditor = document.getElementById('codeEditor');
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
const consoleList = document.getElementById('consoleList');
const clearConsoleBtn = document.getElementById('clearConsoleBtn');
const stepList = document.getElementById('stepList');
const clearStepBtn = document.getElementById('clearStepBtn');
const themeToggle = document.getElementById('themeToggle');
const themeIcon = document.getElementById('themeIcon');
const themeLabel = document.getElementById('themeLabel');

// ===== 常量 =====

/** 工作区目录（相对于 local_data，经 /file/* 接口读写） */
const WORKSPACE_PATH = 'package/lunar.code-studio/workspace';
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

// ===== 文件管理（/file/write、/file/list、/file/read） =====

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

/** 保存文件到工作区（POST /file/write，X-File-Name 相对 local_data） */
function saveFileToWorkspace(name, content) {
    return new Promise((resolve, reject) => {
        const xhr = new XMLHttpRequest();
        xhr.addEventListener('load', () => {
            if (xhr.status === 200) resolve();
            else reject(new Error(xhr.responseText || '保存失败（HTTP ' + xhr.status + '）'));
        });
        xhr.addEventListener('error', () => reject(new Error('保存失败')));
        xhr.addEventListener('timeout', () => reject(new Error('保存超时')));
        xhr.open('POST', '/file/write');
        xhr.setRequestHeader('X-File-Name', encodeFileName(WORKSPACE_PATH + '/' + name));
        xhr.setRequestHeader('X-Overwrite', 'true');
        xhr.send(new Blob([content], { type: 'text/plain; charset=utf-8' }));
    });
}

/** 读取工作区文件内容（GET /file/read） */
async function fetchFileText(name) {
    const resp = await fetch('/file/read/' + WORKSPACE_PATH + '/' + encodeURIComponent(name), { cache: 'no-store' });
    if (!resp.ok) throw new Error('读取文件失败：' + name + '（HTTP ' + resp.status + '）');
    return await resp.text();
}

/** 刷新工作区文件列表（GET /file/list） */
async function renderFileList() {
    try {
        const resp = await fetch('/file/list/' + WORKSPACE_PATH, { cache: 'no-store' });
        if (!resp.ok) throw new Error('HTTP ' + resp.status);
        const files = await resp.json();
        const list = (Array.isArray(files) ? files : []).filter(f => !f.isDir);
        fileList.innerHTML = '';
        if (list.length === 0) {
            fileList.innerHTML = '<div class="list-empty">暂无文件。</div>';
            return;
        }
        list.sort((a, b) => String(a.name).localeCompare(String(b.name)));
        list.forEach(f => {
            const name = String(f.name || '');
            const el = document.createElement('div');
            el.className = 'file-item' + (fileNameInput.value.trim() === name ? ' active' : '');
            const iconCls = /\.html?$/i.test(name) ? 'f-html' : (/\.js$/i.test(name) ? 'f-js' : 'f-other');
            const icon = /\.html?$/i.test(name) ? 'fa-file-code' : (/\.js$/i.test(name) ? 'fa-js' : 'fa-file');
            el.innerHTML = '<span class="file-icon ' + iconCls + '"><i class="fas ' + icon + '"></i></span>'
                + '<span class="file-name"></span>'
                + '<span class="file-meta">' + (typeof f.size === 'number' ? (f.size / 1024).toFixed(1) + ' KB' : '') + '</span>';
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
        fileList.innerHTML = '<div class="list-empty">暂无文件。</div>';
    }
}

refreshFilesBtn.addEventListener('click', () => { renderFileList(); showToast('文件列表已刷新', 'success'); });

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

/** 空预览页 */
function showEmptyPreview() {
    previewFrame.srcdoc = '<!DOCTYPE html><html><head><meta charset="utf-8"></head><body style="font-family:sans-serif;color:#9aa;display:flex;align-items:center;justify-content:center;height:100vh;margin:0">等待运行…</body></html>';
    previewStatus.textContent = '';
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
        previewFrame.srcdoc = doc;
        setTimeout(() => {
            consoleCollector = null;
            previewStatus.textContent = '已运行：' + label + '（捕获输出 ' + lines.length + ' 行）';
            resolve(lines);
        }, OUTPUT_WAIT_MS);
    });
}

/** 停止运行（清空沙箱页面） */
function stopRun() {
    previewFrame.srcdoc = '';
    setTimeout(showEmptyPreview, 50);
    previewStatus.textContent = '已停止';
}

// ===== 人机协同核心操作（智能体工具与手动按钮共用） =====

/** 最近一次运行汇总（供 LTPX 回执） */
let lastRunSummary = '';

/**
 * 写入代码到工作区并同步到编辑器
 * @param {string} name - 文件名（无扩展名时按内容自动补）
 * @param {string} content - 完整文件内容
 * @returns {Promise<string>} 规范化后的文件名
 */
async function writeCodeFile(name, content) {
    const body = String(content || '');
    if (!body.trim()) throw new Error('代码内容为空');
    const fileName = normalizeFileName(name, body);
    await saveFileToWorkspace(fileName, body);
    fileNameInput.value = fileName;
    codeEditor.value = body;
    updateStats();
    renderFileList();
    appendStep('tool-result', '已保存 ' + fileName + '（' + body.length + ' 字符）并同步到编辑器');
    return fileName;
}

/**
 * 读取工作区文件（同时载入编辑器，供人查看与修改）
 * @param {string} name - 文件名
 * @returns {Promise<{name: string, content: string}>}
 */
async function readCodeFile(name) {
    const fileName = normalizeFileName(name, '');
    const content = await fetchFileText(fileName);
    fileNameInput.value = fileName;
    codeEditor.value = content;
    updateStats();
    renderFileList();
    appendStep('tool-result', '读取文件：' + fileName + '（' + content.length + ' 字符）');
    return { name: fileName, content: content };
}

/**
 * 运行代码（指定文件则先载入，否则运行编辑器当前内容），收集控制台输出
 * @param {string} [name] - 可选文件名
 * @returns {Promise<{lines: string[], summary: string}>}
 */
async function runWorkspaceFile(name) {
    let content, fileName;
    if (name) {
        fileName = normalizeFileName(name, '');
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
window.runWorkspaceFile = runWorkspaceFile;
window.listWorkspaceFiles = renderFileList;

// ===== 编辑器统计 =====

function updateStats() {
    const content = codeEditor.value;
    const lines = content ? content.split('\n').length : 0;
    const lang = detectLanguage(fileNameInput.value, content);
    codeStats.textContent = (content ? lines + ' 行 · ' + content.length + ' 字符 · ' : '') + (lang === 'html' ? 'HTML' : 'JavaScript');
}

codeEditor.addEventListener('input', updateStats);
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
        const name = await writeCodeFile(fileNameInput.value, content);
        showToast('已保存：' + name, 'success');
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

    const instruction = (data.arguments || {}).instruction || '';
    clearStepPanel();
    lastRunSummary = '';
    try {
        const result = await runCodeAgent(instruction, (step) => appendStep(step.kind, step.text));
        // 回执 = 智能体答复 + 最近运行汇总（传回月华系统）
        let receipt = '【代码工坊】\n' + (result.text || '');
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
