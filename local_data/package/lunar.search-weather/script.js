/**
 * 实时天气与新闻模块 - 页面逻辑
 * 职责：
 * - 支持用户手动查询天气 / 新闻（复用 ltpx-agent.js 中的 fetchWeather / fetchNews）
 * - 接收琉璃主窗口投递的 ltpx_run 指令，驱动 AtoA 智能体执行
 * - 以「月华操作步骤」面板实时展示月华操作步骤与结构（收到指令 → 智能体分析 → 工具调用 → 结果 → 答复），仅月华指令写入
 * - 以「查询结果」区域渲染天气卡片 / 新闻列表
 * - 以「历史记录」区域持久化最近查询（localStorage），点击可回看结果
 */

// ===== DOM 引用 =====
const cityInput = document.getElementById('cityInput');
const weatherBtn = document.getElementById('weatherBtn');
const newsBtn = document.getElementById('newsBtn');
const resultBody = document.getElementById('resultBody');
const stepList = document.getElementById('stepList');
const clearStepBtn = document.getElementById('clearStepBtn');
const historyList = document.getElementById('historyList');
const clearHistoryBtn = document.getElementById('clearHistoryBtn');

// ===== 步骤面板 =====
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

// ===== 历史记录（localStorage 持久化） =====

const HISTORY_KEY = 'swQueryHistory';
const HISTORY_MAX = 30;

/** 最近一次渲染的结果载荷（供智能体执行后写入历史） */
let lastResult = null;

/** 读取历史记录 */
function loadHistory() {
    try {
        const raw = localStorage.getItem(HISTORY_KEY);
        const list = raw ? JSON.parse(raw) : [];
        return Array.isArray(list) ? list : [];
    } catch (e) {
        return [];
    }
}

/** 保存历史记录 */
function saveHistory(list) {
    try {
        localStorage.setItem(HISTORY_KEY, JSON.stringify(list.slice(0, HISTORY_MAX)));
    } catch (e) {
        console.warn('历史记录保存失败:', e);
    }
}

/** 追加一条历史记录并刷新列表 */
function addHistory(kind, title, result) {
    const list = loadHistory();
    list.unshift({ kind: kind, title: title, result: result, time: new Date().toLocaleString('zh-CN', { hour12: false }) });
    saveHistory(list);
    renderHistory();
}

/** 渲染历史列表 */
function renderHistory() {
    const list = loadHistory();
    historyList.innerHTML = '';
    if (list.length === 0) {
        historyList.innerHTML = '<div class="history-empty">暂无查询记录。</div>';
        return;
    }
    const ICONS = { weather: 'fa-cloud-sun-rain', news: 'fa-newspaper', agent: 'fa-robot', error: 'fa-circle-xmark' };
    list.forEach(item => {
        const el = document.createElement('div');
        el.className = 'history-item';
        el.innerHTML = '<span class="history-icon h-' + (item.kind || 'info') + '"><i class="fas ' + (ICONS[item.kind] || ICONS.error) + '"></i></span>'
            + '<span class="history-body"><span class="history-title"></span><span class="history-time"></span></span>';
        el.querySelector('.history-title').textContent = item.title || '';
        el.querySelector('.history-time').textContent = item.time || '';
        el.addEventListener('click', () => {
            renderResult(item.result || { type: 'error', error: '记录结果缺失' });
        });
        historyList.appendChild(el);
    });
}

clearHistoryBtn.addEventListener('click', () => {
    saveHistory([]);
    renderHistory();
});

// ===== 结果渲染 =====

/** 渲染查询结果到结果区（并记录最近一次结果供写入历史） */
function renderResult(payload) {
    if (!payload) return;
    lastResult = payload;
    if (payload.type === 'loading') {
        resultBody.innerHTML = '<div class="result-loading"><div class="spinner"></div><p>正在查询...</p></div>';
        return;
    }
    if (payload.type === 'error') {
        resultBody.innerHTML = '<div class="result-error"><i class="fas fa-circle-xmark"></i><p>' + escapeHTML(payload.error || '查询失败') + '</p></div>';
        return;
    }
    if (payload.type === 'weather') renderWeather(payload.data);
    else if (payload.type === 'news') renderNews(payload.data);
    else resultBody.innerHTML = '<div class="result-empty"><i class="fas fa-circle-info"></i> 暂无结果</div>';
}

function escapeHTML(text) {
    const div = document.createElement('div');
    div.textContent = String(text);
    return div.innerHTML;
}

/** 渲染天气卡片 */
function renderWeather(data) {
    if (!data) return;
    const c = data.current || {};
    const html = [
        '<div class="weather-block">',
        '  <div class="weather-city"><i class="fas fa-location-dot"></i> ' + escapeHTML(data.city) + '</div>',
        '  <div class="weather-main">',
        '    <div class="weather-temp">' + (c.temperature || 0) + '<span class="weather-unit">°C</span></div>',
        '    <div class="weather-cond"><i class="fas fa-cloud-sun"></i> ' + escapeHTML(c.condition || '未知') + '</div>',
        '  </div>',
        '  <div class="weather-meta">',
        '    <div class="weather-meta-item"><i class="fas fa-temperature-half"></i><span>体感 ' + (c.feels_like || 0) + '°C</span></div>',
        '    <div class="weather-meta-item"><i class="fas fa-droplet"></i><span>湿度 ' + (c.humidity || 0) + '%</span></div>',
        '    <div class="weather-meta-item"><i class="fas fa-wind"></i><span>' + escapeHTML(c.wind || '-') + '</span></div>',
        '  </div>',
        '  <div class="weather-forecast">'
    ].join('');
    const forecastHtml = (data.forecast || []).map(f =>
        '<div class="forecast-card">'
        + '<div class="forecast-date">' + escapeHTML(f.date || '') + '</div>'
        + '<div class="forecast-cond">' + escapeHTML(f.condition || '未知') + '</div>'
        + '<div class="forecast-temp">' + (f.low || 0) + '° ~ ' + (f.high || 0) + '°</div>'
        + '</div>'
    ).join('');
    const footer = '</div></div>'
        + '<div class="weather-source">数据来源：' + escapeHTML(data.source || '-') + '</div>';
    resultBody.innerHTML = html + forecastHtml + footer;
}

/** 渲染新闻列表 */
function renderNews(data) {
    if (!data || !Array.isArray(data.news)) return;
    const items = data.news.map((n, i) =>
        '<div class="news-item"><span class="news-index">' + (i + 1) + '</span><span class="news-text">' + escapeHTML(n) + '</span></div>'
    ).join('');
    resultBody.innerHTML = '<div class="news-block">'
        + '<div class="news-header"><i class="fas fa-newspaper"></i> ' + escapeHTML(data.date || '今日') + ' 要闻</div>'
        + items
        + '</div>';
}

// ===== 手动查询 =====

/** 手动查询天气（不写入「月华操作步骤」，仅渲染结果并记录历史） */
async function handleManualWeather() {
    const city = cityInput.value.trim();
    if (!city) { cityInput.focus(); return; }
    renderResult({ type: 'loading' });
    try {
        const data = await fetchWeather(city);
        renderResult({ type: 'weather', data: data });
        addHistory('weather', data.city + ' 天气', { type: 'weather', data: data });
    } catch (e) {
        renderResult({ type: 'error', error: e.message });
        addHistory('error', city + ' 天气查询失败', { type: 'error', error: e.message });
    }
}

/** 手动获取新闻（不写入「月华操作步骤」，仅渲染结果并记录历史） */
async function handleManualNews() {
    renderResult({ type: 'loading' });
    try {
        const data = await fetchNews();
        renderResult({ type: 'news', data: data });
        addHistory('news', (data.date || '今日') + ' 要闻', { type: 'news', data: data });
    } catch (e) {
        renderResult({ type: 'error', error: e.message });
        addHistory('error', '新闻获取失败', { type: 'error', error: e.message });
    }
}

weatherBtn.addEventListener('click', handleManualWeather);
newsBtn.addEventListener('click', handleManualNews);
cityInput.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') { e.preventDefault(); handleManualWeather(); }
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
            keep_open: true // 查询执行后保持页面展示结果，由用户手动关闭
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
        const result = await runSWAgent(instruction, (step) => appendStep(step.kind, step.text));
        postLTPXResult(data.request_id, result.success, result.text, result.error);
        // 将智能体执行结果写入历史记录
        addHistory('agent', '月华指令：' + instruction, lastResult || { type: 'error', error: result.text || '查询失败' });
    } catch (e) {
        console.error('LTPX AtoA 执行失败:', e);
        appendStep('error', '执行失败：' + (e.message || e));
        renderResult({ type: 'error', error: (e.message || '执行失败') });
        addHistory('error', '月华指令执行失败：' + instruction, { type: 'error', error: (e.message || '执行失败') });
        postLTPXResult(data.request_id, false, '', e.message || '执行失败');
    }
});

// ===== 初始化 =====
renderHistory();
