// ===== 全局元素引用 =====
const E = {
    sidebar: document.getElementById('sidebar'),
    sidebarTrigger: document.getElementById('sidebarTrigger'),
    sidebarArrow: document.getElementById('sidebarArrow'),
    inputText: document.getElementById('inputText'),
    charCount: document.getElementById('charCount'),
    dropZone: document.getElementById('dropZone'),
    dropOverlay: document.getElementById('dropOverlay'),
    btnTranslate: document.getElementById('btnTranslate'),
    targetLang: document.getElementById('targetLang'),
    modeFull: document.getElementById('modeFull'),
    modeLine: document.getElementById('modeLine'),
    styleGroup: document.getElementById('styleGroup'),
    styleButtonsGrid: document.getElementById('styleButtonsGrid'),
    stylePlaceholder: document.getElementById('stylePlaceholder'),
    outputLoading: document.getElementById('outputLoading'),
    loadingText: document.getElementById('loadingText'),
    outputEmpty: document.getElementById('outputEmpty'),
    outputText: document.getElementById('outputText'),
    outputError: document.getElementById('outputError'),
    outputCharCount: document.getElementById('outputCharCount'),
    btnCopy: document.getElementById('btnCopy'),
    themeToggle: document.getElementById('themeToggle'),
    themeLabel: document.getElementById('themeLabel'),
    toast: document.getElementById('toast'),
    // 翻页
    inputFooter: document.getElementById('inputFooter'),
    inputPageInfo: document.getElementById('inputPageInfo'),
    inputFirst: document.getElementById('inputFirst'),
    inputPrev: document.getElementById('inputPrev'),
    inputNext: document.getElementById('inputNext'),
    inputLast: document.getElementById('inputLast'),
    outputFooter: document.getElementById('outputFooter'),
    outputPageInfo: document.getElementById('outputPageInfo'),
    outputFirst: document.getElementById('outputFirst'),
    outputPrev: document.getElementById('outputPrev'),
    outputNext: document.getElementById('outputNext'),
    outputLast: document.getElementById('outputLast'),
};

// ===== 状态 =====
let currentMode = 'full';            // 默认全文翻译
let isTranslating = false;
let abortController = null;          // 用于中断流式请求

// 翻页状态
let inputFullText = '';              // 输入全文
let inputPages = [];                 // 输入分页数组
let inputPageIdx = 0;               // 当前输入页码 (0-based)

let outputFullText = '';            // 输出全文
let outputPages = [];               // 输出分页数组
let outputPageIdx = 0;             // 当前输出页码 (0-based)

// ===== 初始化 =====
function init() {
    // 默认全文模式
    switchMode('full');

    // 输入事件
    E.inputText.addEventListener('input', onInputChange);
    E.inputText.addEventListener('keydown', (e) => {
        if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
            e.preventDefault();
            onTranslate();
        }
    });

    // 拖拽
    E.dropZone.addEventListener('dragover', onDragOver);
    E.dropZone.addEventListener('dragleave', onDragLeave);
    E.dropZone.addEventListener('drop', onDrop);

    // 翻译按钮
    E.btnTranslate.addEventListener('click', onTranslate);

    // 模式切换
    E.modeFull.addEventListener('click', () => switchMode('full'));
    E.modeLine.addEventListener('click', () => switchMode('line'));

    // 样式转换（事件委托）
    E.styleButtonsGrid.addEventListener('click', onStyleConvert);

    // 复制
    E.btnCopy.addEventListener('click', onCopy);

    // 主题
    E.themeToggle.addEventListener('click', onToggleTheme);

    // ===== 侧边栏交互 =====
    // 鼠标靠近左边缘 → 展开
    E.sidebarTrigger.addEventListener('mouseenter', openSidebar);
    // 鼠标离开侧边栏 → 收起（延迟防抖）
    E.sidebar.addEventListener('mouseleave', scheduleCloseSidebar);
    E.sidebar.addEventListener('mouseenter', cancelCloseSidebar);
    // 点击把手切换
    document.querySelector('.sidebar-handle').addEventListener('click', toggleSidebar);

    // 翻页按钮
    E.inputFirst.addEventListener('click', () => inputGoToPage(0));
    E.inputPrev.addEventListener('click', () => inputGoToPage(inputPageIdx - 1));
    E.inputNext.addEventListener('click', () => inputGoToPage(inputPageIdx + 1));
    E.inputLast.addEventListener('click', () => inputGoToPage(inputPages.length - 1));
    E.outputFirst.addEventListener('click', () => outputGoToPage(0));
    E.outputPrev.addEventListener('click', () => outputGoToPage(outputPageIdx - 1));
    E.outputNext.addEventListener('click', () => outputGoToPage(outputPageIdx + 1));
    E.outputLast.addEventListener('click', () => outputGoToPage(outputPages.length - 1));

    // 窗口大小变化 → 重新计算分页
    window.addEventListener('resize', debounce(() => {
        saveInputPage();
        recalcInputPages(false);
        recalcOutputPages(false);
    }, 300));

    updateStyleVisibility();
}

// ===== 侧边栏 =====
let sidebarCloseTimer = null;

function openSidebar() {
    E.sidebar.classList.add('open');
    cancelCloseSidebar();
}

function scheduleCloseSidebar() {
    sidebarCloseTimer = setTimeout(() => {
        E.sidebar.classList.remove('open');
    }, 400);
}

function cancelCloseSidebar() {
    if (sidebarCloseTimer) {
        clearTimeout(sidebarCloseTimer);
        sidebarCloseTimer = null;
    }
}

function toggleSidebar() {
    if (E.sidebar.classList.contains('open')) {
        E.sidebar.classList.remove('open');
    } else {
        E.sidebar.classList.add('open');
    }
    cancelCloseSidebar();
}

// ===== 翻页系统 =====
function getLinesPerPage(panelBody) {
    // 用隐藏元素测量行高
    const test = document.createElement('span');
    test.textContent = 'Ag';
    test.style.cssText = 'position:absolute;visibility:hidden;font-size:0.9rem;font-family:inherit;line-height:1.65;';
    panelBody.appendChild(test);
    const lh = test.getBoundingClientRect().height;
    panelBody.removeChild(test);
    const h = panelBody.clientHeight - 32; // 扣除 padding
    return Math.max(1, Math.floor(h / lh));
}

function splitIntoPages(text, linesPerPage) {
    if (!text) return [''];
    const lines = text.split('\n');
    const pages = [];
    for (let i = 0; i < lines.length; i += linesPerPage) {
        pages.push(lines.slice(i, i + linesPerPage).join('\n'));
    }
    return pages;
}

function pagesToFullText(pages) {
    return pages.join('\n');
}

// ---- 输入翻页 ----
function saveInputPage() {
    if (inputPages.length > 0 && inputPageIdx < inputPages.length) {
        inputPages[inputPageIdx] = E.inputText.value;
        inputFullText = pagesToFullText(inputPages);
    }
}

function recalcInputPages(keepIdx) {
    const lpp = getLinesPerPage(document.getElementById('inputBody'));
    const text = inputFullText;
    inputPages = splitIntoPages(text, lpp);
    if (inputPages.length === 0) inputPages = [''];

    // 显示/隐藏翻页栏
    const multiPage = inputPages.length > 1;
    E.inputFooter.style.display = multiPage ? '' : 'none';

    if (keepIdx) {
        inputPageIdx = Math.min(inputPageIdx, inputPages.length - 1);
    } else {
        inputPageIdx = Math.min(inputPageIdx, inputPages.length - 1);
    }
    renderInputPage();
}

function renderInputPage() {
    if (inputPages.length === 0) {
        E.inputText.value = '';
        E.inputPageInfo.textContent = '第 1/1 页';
        return;
    }
    E.inputText.value = inputPages[inputPageIdx] || '';
    E.inputPageInfo.textContent = `第 ${inputPageIdx + 1}/${inputPages.length} 页`;
    updateInputNavButtons();
}

function inputGoToPage(idx) {
    if (idx < 0 || idx >= inputPages.length) return;
    saveInputPage();
    inputPageIdx = idx;
    renderInputPage();
}

function updateInputNavButtons() {
    E.inputFirst.disabled = inputPageIdx === 0;
    E.inputPrev.disabled = inputPageIdx === 0;
    E.inputNext.disabled = inputPageIdx >= inputPages.length - 1;
    E.inputLast.disabled = inputPageIdx >= inputPages.length - 1;
}

function appendToInputText(newText) {
    inputFullText += (inputFullText ? '\n' : '') + newText;
    recalcInputPages(true);
    inputGoToPage(inputPages.length - 1); // 跳到最新页
    E.charCount.textContent = `${inputFullText.length} 字`;
    updateStyleVisibility();
}

// ---- 输出翻页 ----
function recalcOutputPages(keepIdx) {
    const lpp = getLinesPerPage(document.getElementById('outputBody'));
    outputPages = splitIntoPages(outputFullText, lpp);
    if (outputPages.length === 0) outputPages = [''];

    const multiPage = outputPages.length > 1;
    E.outputFooter.style.display = multiPage ? '' : 'none';

    if (keepIdx) {
        outputPageIdx = Math.min(outputPageIdx, outputPages.length - 1);
    }
    renderOutputPage();
    updateOutputNavButtons();
}

function renderOutputPage() {
    if (outputPages.length === 0) {
        E.outputText.textContent = '';
        E.outputPageInfo.textContent = '第 1/1 页';
        return;
    }
    E.outputText.textContent = outputPages[outputPageIdx] || '';
    E.outputPageInfo.textContent = `第 ${outputPageIdx + 1}/${outputPages.length} 页`;
}

function outputGoToPage(idx) {
    if (idx < 0 || idx >= outputPages.length) return;
    outputPageIdx = idx;
    renderOutputPage();
    updateOutputNavButtons();
}

function updateOutputNavButtons() {
    E.outputFirst.disabled = outputPageIdx === 0;
    E.outputPrev.disabled = outputPageIdx === 0;
    E.outputNext.disabled = outputPageIdx >= outputPages.length - 1;
    E.outputLast.disabled = outputPageIdx >= outputPages.length - 1;
}

function appendToOutputText(newText) {
    outputFullText += newText;
    recalcOutputPages(true);
    // 流式输出时自动跳到最后一页
    outputGoToPage(outputPages.length - 1);
    E.outputCharCount.textContent = `${outputFullText.length} 字`;
}

// ===== 输入变化 =====
function onInputChange() {
    // 从 textarea 同步回全文
    if (inputPages.length > 0 && inputPageIdx < inputPages.length) {
        inputPages[inputPageIdx] = E.inputText.value;
        inputFullText = pagesToFullText(inputPages);
    } else {
        inputFullText = E.inputText.value;
        recalcInputPages(false);
    }
    E.charCount.textContent = `${inputFullText.length} 字`;
    updateStyleVisibility();
}

// ===== 样式转换可见性 =====
function updateStyleVisibility() {
    const len = inputFullText.length;
    if (len > 0 && len < 500) {
        E.styleGroup.style.display = '';
        E.stylePlaceholder.style.display = 'none';
    } else if (len >= 500) {
        E.styleGroup.style.display = 'none';
        E.stylePlaceholder.style.display = '';
    } else {
        E.styleGroup.style.display = '';
        E.stylePlaceholder.style.display = 'none';
    }
}

// ===== 拖拽 =====
function onDragOver(e) { e.preventDefault(); e.stopPropagation(); E.dropZone.classList.add('drag-over'); }
function onDragLeave(e) { e.preventDefault(); e.stopPropagation(); E.dropZone.classList.remove('drag-over'); }

function onDrop(e) {
    e.preventDefault(); e.stopPropagation();
    E.dropZone.classList.remove('drag-over');
    const file = e.dataTransfer.files[0];
    if (!file) return;

    const validExts = ['.txt', '.md', '.json', '.csv', '.xml', '.yaml', '.yml',
        '.html', '.css', '.js', '.ts', '.ini', '.cfg', '.conf', '.lang', '.properties', '.toml', '.log'];
    const nameL = file.name.toLowerCase();
    if (!validExts.some(ext => nameL.endsWith(ext))) {
        showToast('不支持的文件类型，请拖入文本文件', 'error');
        return;
    }

    const reader = new FileReader();
    reader.onload = ev => {
        inputFullText = ev.target.result;
        recalcInputPages(false);
        inputGoToPage(0);
        E.charCount.textContent = `${inputFullText.length} 字`;
        updateStyleVisibility();
        showToast(`已导入: ${file.name}`, 'success');
    };
    reader.onerror = () => showToast('文件读取失败', 'error');
    reader.readAsText(file);
}

// ===== 模式切换 =====
function switchMode(mode) {
    currentMode = mode;
    E.modeFull.classList.toggle('active', mode === 'full');
    E.modeLine.classList.toggle('active', mode === 'line');
}

// ===== 翻译入口 =====
async function onTranslate() {
    const text = inputFullText.trim();
    if (!text) { showToast('请先输入或拖入待翻译文本', 'error'); return; }
    if (isTranslating) return;

    isTranslating = true;
    const lang = E.targetLang.value;
    const langLabel = E.targetLang.options[E.targetLang.selectedIndex].text;

    // 重置输出
    outputFullText = '';
    outputPages = [''];
    outputPageIdx = 0;
    E.outputEmpty.style.display = 'none';
    E.outputText.style.display = '';
    E.outputText.textContent = '';
    E.outputError.style.display = 'none';
    E.outputFooter.style.display = 'none';
    E.outputCharCount.textContent = '0 字';
    E.outputLoading.style.display = '';
    E.loadingText.textContent = `正在翻译为${langLabel}...`;

    try {
        if (currentMode === 'line') {
            await translateLineByLine(text, lang, langLabel);
        } else {
            await translateFullTextStream(text, lang, langLabel);
        }
    } catch (err) {
        if (err.name === 'AbortError') return;
        E.outputError.style.display = '';
        E.outputError.innerHTML = `<i class="fas fa-exclamation-circle"></i>${escapeHtml(err.message || '翻译失败')}`;
    } finally {
        E.outputLoading.style.display = 'none';
        isTranslating = false;
        abortController = null;
    }
}

// ===== 逐行翻译（真·逐行，一行一行翻译） =====
async function translateLineByLine(text, targetLang, langLabel) {
    const lines = text.split('\n');
    const lineMap = []; // { isKV, key, value } for KV lines, { text } for plain lines

    for (const line of lines) {
        const eqIdx = line.indexOf('=');
        if (eqIdx > 0) {
            const key = line.substring(0, eqIdx).trim();
            const value = line.substring(eqIdx + 1);
            if (value.trim()) {
                lineMap.push({ isKV: true, key, value });
            } else {
                lineMap.push({ isKV: false, text: line });
            }
        } else {
            lineMap.push({ isKV: false, text: line });
        }
    }

    // 逐行翻译 KV 行
    let translatedCount = 0;
    const kvIndices = [];
    lineMap.forEach((m, i) => { if (m.isKV) kvIndices.push(i); });

    const resultLines = new Array(lineMap.length).fill('');

    // 先填充非 KV 行
    lineMap.forEach((m, i) => {
        if (!m.isKV) resultLines[i] = m.text;
    });

    for (let ki = 0; ki < kvIndices.length; ki++) {
        const i = kvIndices[ki];
        const m = lineMap[i];

        E.loadingText.textContent = `正在翻译第 ${ki + 1}/${kvIndices.length} 行...`;

        const messages = [
            { role: 'system', content: `你是一个专业的翻译引擎。请将以下文本翻译为${targetLang}。只输出翻译结果，不要添加任何解释。` },
            { role: 'user', content: m.value }
        ];

        try {
            const translated = await callAI(messages);
            resultLines[i] = `${m.key}=${translated.trim()}`;
        } catch (err) {
            resultLines[i] = `${m.key}=${m.value}`; // 翻译失败保留原文
        }

        // 每翻译一行就更新输出
        outputFullText = resultLines.join('\n');
        recalcOutputPages(true);
        outputGoToPage(outputPages.length - 1);
        E.outputCharCount.textContent = `${outputFullText.length} 字`;
    }

    showToast(`逐行翻译完成，共 ${kvIndices.length} 行`, 'success');
}

// ===== 全文翻译（流式输出） =====
async function translateFullTextStream(text, targetLang, langLabel) {
    const MAX_CHARS = 8000;

    if (text.length <= MAX_CHARS) {
        const messages = [
            { role: 'system', content: `你是一个专业的翻译引擎。请将以下文本翻译为${targetLang}。只输出翻译结果，不要添加任何解释。` },
            { role: 'user', content: text }
        ];
        await streamAI(messages);
    } else {
        // 超过8000字分批，每批用流式
        const chunks = splitTextAtNaturalBreaks(text, MAX_CHARS);
        for (let i = 0; i < chunks.length; i++) {
            if (!isTranslating) return;
            E.loadingText.textContent = `正在翻译第 ${i + 1}/${chunks.length} 段...`;

            const messages = [
                { role: 'system', content: `你是一个专业的翻译引擎。这是全文翻译的第 ${i + 1}/${chunks.length} 段。请翻译为${targetLang}。只输出翻译结果。` },
                { role: 'user', content: chunks[i] }
            ];

            // 段间加分隔空行
            if (i > 0) appendToOutputText('\n\n');
            await streamAI(messages);
        }
    }

    showToast('全文翻译完成', 'success');
}

// ===== 流式 AI 调用 =====
async function streamAI(messages) {
    abortController = new AbortController();

    const response = await fetch('/v1/chat/completions', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
            model: 'system-multimodal',
            messages: messages,
            stream: true,
            temperature: 0.3
        }),
        signal: abortController.signal
    });

    if (!response.ok) {
        const errText = await response.text();
        throw new Error(`API 请求失败 (${response.status})`);
    }

    const reader = response.body.getReader();
    const decoder = new TextDecoder();
    let buffer = '';

    while (true) {
        const { done, value } = await reader.read();
        if (done) break;

        buffer += decoder.decode(value, { stream: true });
        const lines = buffer.split('\n');
        buffer = lines.pop() || '';

        for (const line of lines) {
            const trimmed = line.trim();
            if (!trimmed.startsWith('data: ')) continue;
            const data = trimmed.slice(6);
            if (data === '[DONE]') break;

            try {
                const parsed = JSON.parse(data);
                const content = parsed.choices?.[0]?.delta?.content;
                if (content) appendToOutputText(content);
            } catch (e) { /* skip malformed JSON */ }
        }
    }
}

// ===== 非流式 AI 调用（逐行翻译用） =====
async function callAI(messages) {
    abortController = new AbortController();

    const response = await fetch('/v1/chat/completions', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
            model: 'system-multimodal',
            messages: messages,
            stream: false,
            temperature: 0.3
        }),
        signal: abortController.signal
    });

    if (!response.ok) {
        const errText = await response.text();
        throw new Error(`API 请求失败 (${response.status})`);
    }

    const data = await response.json();
    if (!data.choices?.[0]?.message?.content) {
        throw new Error('API 返回格式异常');
    }
    return data.choices[0].message.content;
}

// ===== 文本拆分 =====
function splitTextAtNaturalBreaks(text, maxLen) {
    const chunks = [];
    let remaining = text;

    while (remaining.length > 0) {
        if (remaining.length <= maxLen) { chunks.push(remaining); break; }

        const search = remaining.substring(0, maxLen);
        // 段落分隔
        let idx = search.lastIndexOf('\n\n');
        if (idx > maxLen * 0.5) { chunks.push(remaining.substring(0, idx + 2)); remaining = remaining.substring(idx + 2); continue; }

        // 句末标点
        const ends = ['. ', '。', '！\n', '!\n', '?\n', '？\n', '.\n', '；\n', ';\n', '...', '……'];
        let best = -1, bestLen = 0;
        for (const e of ends) {
            const i = search.lastIndexOf(e);
            if (i > best) { best = i; bestLen = e.length; }
        }
        if (best > maxLen * 0.4) { chunks.push(remaining.substring(0, best + bestLen)); remaining = remaining.substring(best + bestLen); continue; }

        // 换行
        idx = search.lastIndexOf('\n');
        if (idx > maxLen * 0.5) { chunks.push(remaining.substring(0, idx + 1)); remaining = remaining.substring(idx + 1); continue; }

        // 空格
        idx = search.lastIndexOf(' ');
        if (idx > maxLen * 0.5) { chunks.push(remaining.substring(0, idx + 1)); remaining = remaining.substring(idx + 1); continue; }

        // 硬截断
        chunks.push(remaining.substring(0, maxLen));
        remaining = remaining.substring(maxLen);
    }
    return chunks;
}

// ===== 样式转换 =====
function applyStyleConvert(style, text) {
    switch (style) {
        case 'upper':               return text.toUpperCase();
        case 'lower':               return text.toLowerCase();
        case 'capitalize':          return text.replace(/\S+/g, w => w.charAt(0).toUpperCase() + w.slice(1).toLowerCase());
        case 'uncapitalize':        return text.replace(/\S+/g, w => w.charAt(0).toLowerCase() + w.slice(1));
        case 'space_to_underscore': return text.replace(/ /g, '_');
        case 'underscore_space_to_camel': return text.replace(/[_\s]+(.)?/g, (_, c) => c ? c.toUpperCase() : '').replace(/^[A-Z]/, m => m.toLowerCase());
        case 'camel_to_underscore': return text.replace(/[A-Z]/g, m => '_' + m.toLowerCase()).replace(/^_/, '');
        case 'camel_to_space':      return text.replace(/[A-Z]/g, m => ' ' + m.toLowerCase()).replace(/^ /, '');
        case 'space_to_hyphen':     return text.replace(/ /g, '-');
        case 'underscore_to_hyphen':return text.replace(/_/g, '-');
        case 'hyphen_to_underscore':return text.replace(/-/g, '_');
        case 'underscore_to_space': return text.replace(/_/g, ' ');
        case 'underscore_to_dot':   return text.replace(/_/g, '.');
        case 'dot_to_underscore':   return text.replace(/\./g, '_');
        default:                    return text;
    }
}

const STYLE_LABELS = {
    upper: '全大写', lower: '全小写', capitalize: '首字母大写', uncapitalize: '首字母小写',
    space_to_underscore: '空格→下划线', underscore_space_to_camel: '→驼峰',
    camel_to_underscore: '驼峰→下划线', camel_to_space: '驼峰→空格',
    space_to_hyphen: '空格→中横线', underscore_to_hyphen: '下划线→中横线',
    hyphen_to_underscore: '中横线→下划线', underscore_to_space: '下划线→空格',
    underscore_to_dot: '下划线→小数点', dot_to_underscore: '小数点→下划线'
};

function onStyleConvert(e) {
    const btn = e.target.closest('.btn-style');
    if (!btn) return;
    const style = btn.dataset.style;

    // 优先转换输出
    if (outputFullText.trim()) {
        outputFullText = applyStyleConvert(style, outputFullText);
        recalcOutputPages(false);
        outputGoToPage(0);
        E.outputCharCount.textContent = `${outputFullText.length} 字`;
        showToast(`翻译结果已转换: ${STYLE_LABELS[style]}`, 'success');
        return;
    }

    // 无输出时转换输入
    if (!inputFullText.trim()) { showToast('请先输入文本', 'error'); return; }
    inputFullText = applyStyleConvert(style, inputFullText);
    recalcInputPages(false);
    inputGoToPage(0);
    E.charCount.textContent = `${inputFullText.length} 字`;
    updateStyleVisibility();
    showToast(`已转换: ${STYLE_LABELS[style]}`, 'success');
}

// ===== 复制（全部输出） =====
async function onCopy() {
    if (!outputFullText.trim()) return;
    try {
        await navigator.clipboard.writeText(outputFullText);
        showToast('已复制全部翻译结果到剪贴板', 'success');
    } catch {
        const ta = document.createElement('textarea');
        ta.value = outputFullText;
        ta.style.cssText = 'position:fixed;opacity:0;';
        document.body.appendChild(ta);
        ta.select();
        document.execCommand('copy');
        document.body.removeChild(ta);
        showToast('已复制到剪贴板', 'success');
    }
}

// ===== 主题 =====
function onToggleTheme() {
    const isDark = document.body.classList.toggle('dark-mode');
    E.themeLabel.textContent = isDark ? '浅色模式' : '深色模式';
    E.themeToggle.querySelector('i').className = isDark ? 'fas fa-sun' : 'fas fa-moon';
    localStorage.setItem('translate-theme', isDark ? 'dark' : 'light');
    // 重新计算分页（行高可能变化）
    setTimeout(() => { recalcInputPages(true); recalcOutputPages(true); }, 400);
}

function loadTheme() {
    if (localStorage.getItem('translate-theme') === 'dark') {
        document.body.classList.add('dark-mode');
        E.themeLabel.textContent = '浅色模式';
        E.themeToggle.querySelector('i').className = 'fas fa-sun';
    }
}

// ===== Toast =====
function showToast(msg, type) {
    const t = E.toast;
    t.textContent = msg;
    t.className = `toast ${type} visible`;
    clearTimeout(t._timeout);
    t._timeout = setTimeout(() => t.classList.remove('visible'), 2500);
}

// ===== 工具函数 =====
function escapeHtml(str) {
    const d = document.createElement('div');
    d.textContent = str;
    return d.innerHTML;
}

function debounce(fn, ms) {
    let timer;
    return (...args) => { clearTimeout(timer); timer = setTimeout(() => fn(...args), ms); };
}

// ===== 启动 =====
document.addEventListener('DOMContentLoaded', () => {
    loadTheme();
    init();
});
