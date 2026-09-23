/**
 * 图表工坊 - 页面逻辑
 * 职责：
 * - 图表源码编辑器：用户编写 Markdown（```echarts / ```mermaid 代码块），防抖实时渲染预览
 * - 图表预览区：复用月华消息终端的 ECharts / Mermaid 渲染器（chart-renderer.js）实时渲染
 * - 历史记录：localStorage 持久化最近图表源码，点击回看
 * - 接收琉璃主窗口投递的 ltpx_run 指令，驱动 AtoA 图表智能体执行（月华自然语言构建图表）
 * - 「月华操作步骤」面板实时展示智能体执行步骤（仅月华指令写入）
 */

// ===== DOM 引用 =====
const chartSource = document.getElementById('chartSource');
const renderBtn = document.getElementById('renderBtn');
const sampleBtn = document.getElementById('sampleBtn');
const clearEditorBtn = document.getElementById('clearEditorBtn');
const editorStats = document.getElementById('editorStats');
const previewStats = document.getElementById('previewStats');
const chartPreview = document.getElementById('chartPreview');
const stepList = document.getElementById('stepList');
const clearStepBtn = document.getElementById('clearStepBtn');
const historyList = document.getElementById('historyList');
const clearHistoryBtn = document.getElementById('clearHistoryBtn');
const themeToggle = document.getElementById('themeToggle');
const themeIcon = document.getElementById('themeIcon');
const themeLabel = document.getElementById('themeLabel');

// ===== 深色/浅色模式切换 =====
const THEME_KEY = 'csTheme';
function applyTheme(dark) {
    document.body.classList.toggle('dark-mode', dark);
    if (themeIcon) themeIcon.className = 'fas ' + (dark ? 'fa-sun' : 'fa-moon');
    if (themeLabel) themeLabel.textContent = dark ? '浅色' : '深色';
    // Mermaid 主题在 initialize 时固化，切换主题需重建并重渲染
    resetMermaidTheme();
    renderNow();
}
if (themeToggle) {
    themeToggle.addEventListener('click', () => {
        const dark = !document.body.classList.contains('dark-mode');
        applyTheme(dark);
        try { localStorage.setItem(THEME_KEY, dark ? 'dark' : 'light'); } catch (e) { }
    });
}

// ===== Toast =====
let toastTimer = null;
function showToast(msg, type) {
    const el = document.getElementById('toast');
    el.textContent = msg;
    el.className = 'toast visible' + (type ? ' ' + type : '');
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => { el.className = 'toast' + (type ? ' ' + type : ''); }, 2400);
}

// ===== 月华操作步骤面板 =====
const STEP_ICONS = {
    info: 'fa-circle-info',
    thinking: 'fa-brain',
    tool: 'fa-wrench',
    'tool-result': 'fa-arrow-right',
    done: 'fa-check-circle',
    error: 'fa-circle-xmark',
    warn: 'fa-triangle-exclamation'
};

/** 追加一条操作步骤到面板 */
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

/** 清空步骤面板 */
function clearStepPanel() {
    stepList.innerHTML = '<div class="step-empty">暂无月华操作记录。</div>';
}

clearStepBtn.addEventListener('click', clearStepPanel);

// ===== 编辑器与实时渲染 =====

/** 示例源码：ECharts + Mermaid 各一块 */
const SAMPLE_SOURCE = [
    '# 图表工坊示例',
    '',
    '月销售额趋势（ECharts）：',
    '',
    '```echarts',
    '{',
    '  "title": { "text": "月销售额趋势（万元）" },',
    '  "tooltip": {},',
    '  "xAxis": { "type": "category", "data": ["1月", "2月", "3月", "4月", "5月", "6月"] },',
    '  "yAxis": { "type": "value" },',
    '  "series": [{ "type": "line", "data": [82, 93, 101, 96, 118, 132], "smooth": true }]',
    '} ',
    '```',
    '',
    '发布流程（Mermaid）：',
    '',
    '```mermaid',
    'flowchart LR',
    '  A[编写图表] --> B{语法校验}',
    '  B -- 通过 --> C[实时渲染]',
    '  B -- 失败 --> D[修正源码]',
    '  D --> B',
    '```'
].join('\n');

/** 编辑器防抖渲染定时器 */
let renderTimer = null;

/** 当前编辑器源码（供智能体 read_editor / appendChartBlock 使用） */
function getEditorSource() { return chartSource.value; }

/** 立即渲染当前编辑器源码到预览区，并刷新统计 */
async function renderNow() {
    const source = chartSource.value;
    await renderChartPreview(chartPreview, source);
    const counts = countChartBlocks(source);
    previewStats.textContent = '图表 ' + counts.total + '（E ' + counts.echarts + ' / M ' + counts.mermaid + '）';
}

/** 编辑器输入 → 500ms 防抖实时渲染 */
chartSource.addEventListener('input', () => {
    updateEditorStats();
    clearTimeout(renderTimer);
    renderTimer = setTimeout(renderNow, 500);
});

function updateEditorStats() {
    editorStats.textContent = chartSource.value.length + ' 字符';
}

/** 渲染按钮：立即渲染并写入历史 */
renderBtn.addEventListener('click', async () => {
    await renderNow();
    addHistoryFromSource('manual');
    showToast('图表已渲染', 'success');
});

sampleBtn.addEventListener('click', () => {
    chartSource.value = SAMPLE_SOURCE;
    updateEditorStats();
    renderNow();
    showToast('已载入示例源码', 'success');
});

clearEditorBtn.addEventListener('click', () => {
    chartSource.value = '';
    updateEditorStats();
    renderNow();
});

/**
 * 追加一个图表代码块到编辑器末尾并实时渲染（智能体工具与手动操作共用）
 * @param {string} lang - 语言（echarts / mermaid）
 * @param {string} code - 代码块内容
 * @param {string} title - 图表标题
 */
function appendChartBlock(lang, code, title) {
    if (chartSource.value.trim()) chartSource.value += '\n\n';
    chartSource.value += '```' + lang + '\n' + code + '\n```';
    updateEditorStats();
    renderNow();
    addHistoryFromSource('agent', title);
}

// ===== 历史记录（localStorage 持久化） =====

const HISTORY_KEY = 'csChartHistory';
const HISTORY_MAX = 30;

function loadHistory() {
    try {
        const raw = localStorage.getItem(HISTORY_KEY);
        const list = raw ? JSON.parse(raw) : [];
        return Array.isArray(list) ? list : [];
    } catch (e) {
        return [];
    }
}

function saveHistory(list) {
    try {
        localStorage.setItem(HISTORY_KEY, JSON.stringify(list.slice(0, HISTORY_MAX)));
    } catch (e) {
        console.warn('历史记录保存失败:', e);
    }
}

/** 按当前编辑器源码写入一条历史（提取首个图表标题或首行文本作摘要） */
function addHistoryFromSource(kind, title) {
    const source = chartSource.value;
    if (!source.trim()) return;
    const counts = countChartBlocks(source);
    let k = kind === 'agent' ? 'agent' : (counts.echarts && counts.mermaid ? 'mixed' : (counts.mermaid ? 'mermaid' : 'echarts'));
    let name = title || (source.split('\n').find(l => l.trim() && !l.trim().startsWith('```')) || '未命名图表').replace(/^#+\s*/, '').slice(0, 40);
    const list = loadHistory();
    list.unshift({ kind: k, title: name, source: source, time: new Date().toLocaleString('zh-CN', { hour12: false }) });
    saveHistory(list);
    renderHistory();
}

function renderHistory() {
    const list = loadHistory();
    historyList.innerHTML = '';
    if (list.length === 0) {
        historyList.innerHTML = '<div class="history-empty">暂无图表记录。</div>';
        return;
    }
    const ICONS = { echarts: 'fa-chart-column', mermaid: 'fa-diagram-project', mixed: 'fa-layer-group', agent: 'fa-robot', error: 'fa-circle-xmark' };
    list.forEach(item => {
        const el = document.createElement('div');
        el.className = 'history-item';
        el.innerHTML = '<span class="history-icon h-' + (item.kind || 'echarts') + '"><i class="fas ' + (ICONS[item.kind] || ICONS.echarts) + '"></i></span>'
            + '<span class="history-body"><span class="history-title"></span><span class="history-time"></span></span>';
        el.querySelector('.history-title').textContent = item.title || '';
        el.querySelector('.history-time').textContent = item.time || '';
        el.addEventListener('click', () => {
            chartSource.value = item.source || '';
            updateEditorStats();
            renderNow();
        });
        historyList.appendChild(el);
    });
}

clearHistoryBtn.addEventListener('click', () => {
    saveHistory([]);
    renderHistory();
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
            keep_open: true // 图表渲染后保持页面展示结果，由用户手动关闭
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
    try {
        const result = await runCSAgent(instruction, (step) => appendStep(step.kind, step.text));
        postLTPXResult(data.request_id, result.success, result.text, result.error);
    } catch (e) {
        console.error('LTPX AtoA 执行失败:', e);
        appendStep('error', '执行失败：' + (e.message || e));
        postLTPXResult(data.request_id, false, '', e.message || '执行失败');
    }
});

// ===== 初始化 =====
(async function init() {
    applyTheme(localStorage.getItem(THEME_KEY) === 'dark');
    chartSource.value = SAMPLE_SOURCE;
    updateEditorStats();
    // 等全部就绪后再初始化渲染器并做首次渲染，避免竞态
    try { if (window.scriptDependenciesReady) await window.scriptDependenciesReady; } catch (e) { }
    await ensureMarked();
    initMermaid();
    renderHistory();
    renderNow();
})();
