/**
 * 数学工坊 - 页面逻辑
 * 职责：
 * - MathEngine：表达式求值（math.js）与方程符号求解（Nerdamer）的统一封装
 * - ChartRenderer：基于本地 ECharts 的函数图像渲染（多函数、自动采样、缩放平移）
 * - 左侧多行公式输入：点击「解析计算」自动识别内容（含 = 的行按方程求解，其余按表达式求值）
 * - 每一次计算固定绘制对应的函数图像（表达式绘 y = f(x)，方程绘 y = 左边 - 右边）
 * - 接收琉璃主窗口投递的 ltpx_run 指令，驱动 AtoA 智能体执行
 * - 「操作记录」记录手动计算与月华操作步骤；「历史记录」持久化最近计算（localStorage）
 */

// ===== DOM 引用 =====
const formulaInput = document.getElementById('formulaInput');
const execBtn = document.getElementById('execBtn');
const resultBody = document.getElementById('resultBody');
const resultCount = document.getElementById('resultCount');
const chartCanvas = document.getElementById('chartCanvas');
const stepList = document.getElementById('stepList');
const clearStepBtn = document.getElementById('clearStepBtn');
const historyList = document.getElementById('historyList');
const clearHistoryBtn = document.getElementById('clearHistoryBtn');
const themeToggle = document.getElementById('themeToggle');
const themeIcon = document.getElementById('themeIcon');
const themeLabel = document.getElementById('themeLabel');

/** 默认绘图范围 */
const PLOT_X_MIN = -10;
const PLOT_X_MAX = 10;

// ===== 深色/浅色模式切换 =====
const THEME_KEY = 'msTheme';
let isDarkMode = false;

function applyTheme(dark) {
    isDarkMode = dark;
    document.body.classList.toggle('dark-mode', dark);
    if (themeIcon) themeIcon.className = 'fas ' + (dark ? 'fa-sun' : 'fa-moon');
    if (themeLabel) themeLabel.textContent = dark ? '浅色' : '深色';
    // 图表随主题重建（ECharts 需要具体颜色值，主题切换时重绘当前图像）
    if (window.chartRenderer) {
        window.chartRenderer.setTheme(dark);
        const payload = lastResult;
        if (payload && payload.type === 'plot') {
            window.chartRenderer.render(payload.functions, payload.x_min, payload.x_max);
        } else if (payload && payload.plotFns && payload.plotFns.length > 0) {
            window.chartRenderer.render(payload.plotFns, PLOT_X_MIN, PLOT_X_MAX);
        } else {
            window.chartRenderer.showEmpty();
        }
    }
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

/** 显示 Toast 提示 */
function showToast(text, kind) {
    const toast = document.getElementById('toast');
    if (!toast) return;
    toast.textContent = text;
    toast.className = 'toast visible' + (kind ? ' ' + kind : '');
    clearTimeout(showToast._timer);
    showToast._timer = setTimeout(() => { toast.className = 'toast'; }, 2600);
}

// ===== MathEngine 类：表达式求值（math.js）+ 方程求解（Nerdamer） =====

class MathEngine {
    /** 数值/对象统一转字符串（兼容 Nerdamer 返回对象） */
    static formatValue(value) {
        if (value === null || value === undefined) return 'undefined';
        if (typeof value === 'object' && typeof value.toString === 'function') return value.toString();
        return String(value);
    }

    /** 数值格式化（mathjs 类型走 math.format，保持精度；无穷大/NaN 转符号显示） */
    static formatNumber(value) {
        if (typeof value === 'number') {
            if (!isFinite(value)) return value > 0 ? '∞' : (value < 0 ? '-∞' : 'NaN');
            return math.format(value, { precision: 14 });
        }
        return MathEngine.formatValue(value);
    }

    /**
     * 表达式求值（四则运算、函数求值、单位换算、复杂数学表达式）
     * @param {string} expr - 数学表达式
     * @returns {Object} { expr, tex, value }（tex 供 KaTeX 渲染，可能为空）
     */
    evaluate(expr) {
        const trimmed = String(expr || '').trim();
        if (!trimmed) throw new Error('表达式为空');
        // 求值（math.js 支持单位与复杂数学表达式）
        const value = math.evaluate(trimmed);
        if (typeof value === 'function') {
            throw new Error('「' + trimmed + '」是一个函数定义，无法直接求值');
        }
        // LaTeX 表示（用于 KaTeX 公式渲染；解析失败仅跳过公式展示）
        let tex = '';
        try {
            tex = math.parse(trimmed).toTex({ parenthesis: 'auto' });
        } catch (e) { tex = ''; }
        return {
            expr: trimmed,
            tex: tex,
            value: MathEngine.formatNumber(value)
        };
    }

    /**
     * 方程（组）符号求解（Nerdamer Solve 模块，支持多元多次）
     * @param {string[]|string} equations - 方程或方程组
     * @returns {Object} { equations, entries: [{ label, value }], display }
     */
    solve(equations) {
        if (typeof nerdamer === 'undefined') {
            throw new Error('方程求解引擎未加载');
        }
        // 归一化为方程数组
        let list = Array.isArray(equations) ? equations.slice() : [equations];
        if (list.length === 1 && typeof list[0] === 'string' && /[,，]/.test(list[0])) {
            list = list[0].split(/[,，]/);
        }
        list = list.map(s => String(s || '').trim()).filter(Boolean);
        if (list.length === 0) throw new Error('方程为空');
        for (const eq of list) {
            if (!eq.includes('=')) {
                throw new Error('「' + eq + '」不是方程：方程需要包含等号 =');
            }
        }

        // Nerdamer 求解：单方程传字符串，方程组传数组
        const input = list.length === 1 ? list[0] : list;
        let results;
        try {
            results = nerdamer.solveEquations(input);
        } catch (e) {
            throw new Error('无法求解该方程：' + (e && e.message ? e.message : '请检查方程格式'));
        }

        const fmt = MathEngine.formatValue;
        const entries = [];
        if (results === undefined || results === null) {
            throw new Error('未找到解（方程可能无解或解的形式超出符号求解范围）');
        }
        if (!Array.isArray(results)) {
            // 单个解值
            entries.push({ label: '解', value: fmt(results) });
        } else if (results.length > 0 && Array.isArray(results[0])) {
            // [变量, 值] 对形式（方程组或多变量方程）
            const grouped = new Map(); // 变量名 -> 解值数组
            for (const pair of results) {
                const name = fmt(pair[0]);
                const val = fmt(pair[1]);
                if (!grouped.has(name)) grouped.set(name, []);
                grouped.get(name).push(val);
            }
            for (const [name, values] of grouped) {
                for (const v of values) entries.push({ label: name, value: v });
            }
        } else {
            // 解值数组形式（单变量多解）：从等号左侧提取变量名作为标签
            const lhs = list[0].split('=')[0] || '';
            const FUNC_NAMES = ['sin', 'cos', 'tan', 'asin', 'acos', 'atan', 'sinh', 'cosh', 'tanh', 'log', 'ln', 'sqrt', 'exp', 'abs'];
            const vars = (lhs.match(/[a-zA-Z]+/g) || []).filter(t => !FUNC_NAMES.includes(t));
            const varName = vars.length > 0 ? vars[0] : 'x';
            for (const v of results) entries.push({ label: varName, value: fmt(v) });
        }
        if (entries.length === 0) {
            throw new Error('未找到解（方程可能无解或解的形式超出符号求解范围）');
        }

        return {
            equations: list,
            entries: entries,
            display: entries.map(e => e.label + ' = ' + e.value).join('；')
        };
    }

    /** 由方程字符串取绘图函数 y = 左边 - 右边（解即曲线零点） */
    equationToPlotFn(eq) {
        const idx = eq.indexOf('=');
        const lhs = eq.slice(0, idx).trim();
        const rhs = eq.slice(idx + 1).trim();
        return { expr: '(' + lhs + ')-(' + rhs + ')' };
    }
}

// 全局数学引擎实例
const mathEngine = new MathEngine();

// ===== ChartRenderer 类：ECharts 函数图像渲染 =====

class ChartRenderer {
    constructor(container) {
        this.container = container;
        this.chart = null;
        this.darkMode = isDarkMode;
    }

    setTheme(dark) {
        this.darkMode = dark;
    }

    /** 图表文本/轴线颜色随主题切换 */
    _themeColors() {
        return this.darkMode
            ? { text: 'rgba(240, 240, 244, 0.85)', axis: 'rgba(255, 255, 255, 0.25)', split: 'rgba(255, 255, 255, 0.10)' }
            : { text: 'rgba(60, 60, 60, 0.9)', axis: 'rgba(0, 0, 0, 0.20)', split: 'rgba(0, 0, 0, 0.08)' };
    }

    /** 惰性初始化图表实例（容器固定高度，加载后即可初始化） */
    _ensureChart() {
        if (typeof echarts === 'undefined') throw new Error('图表库未加载');
        if (!this.chart) {
            this.chart = echarts.init(chartCanvas);
        }
        return this.chart;
    }

    /** 基础坐标轴配置 */
    _baseOption() {
        const theme = this._themeColors();
        return {
            animation: true,
            grid: { left: 56, right: 24, top: 36, bottom: 60 },
            tooltip: {
                trigger: 'axis',
                confine: true,
                textStyle: { color: theme.text, fontSize: 11 }
            },
            xAxis: {
                type: 'value',
                axisLine: { lineStyle: { color: theme.axis } },
                axisLabel: { color: theme.text, fontSize: 10 },
                splitLine: { lineStyle: { color: theme.split } },
                scale: true
            },
            yAxis: {
                type: 'value',
                axisLine: { lineStyle: { color: theme.axis } },
                axisLabel: { color: theme.text, fontSize: 10 },
                splitLine: { lineStyle: { color: theme.split } },
                scale: true
            },
            dataZoom: [
                { type: 'inside' },
                { type: 'slider', height: 16, bottom: 8 }
            ]
        };
    }

    /** 初始空图像（仅坐标轴，无曲线） */
    showEmpty() {
        if (typeof echarts === 'undefined') return;
        try {
            const chart = this._ensureChart();
            chart.setOption(Object.assign(this._baseOption(), { legend: { show: false }, series: [] }), true);
            chart.resize();
        } catch (e) { /* 图表库未就绪时静默跳过 */ }
    }

    /**
     * 渲染函数图像
     * @param {Array<{expr: string}>} functions - 函数表达式列表（以 x 为自变量）
     * @param {number} xMin - 采样范围下界（默认 -10）
     * @param {number} xMax - 采样范围上界（默认 10）
     * @returns {Promise<void>}
     */
    async render(functions, xMin, xMax) {
        // ECharts 由标准依赖异步注入，等待就绪避免竞态
        if (window.scriptDependenciesReady) await window.scriptDependenciesReady;
        if (!Array.isArray(functions) || functions.length === 0) throw new Error('未提供函数表达式');

        const lo = isFinite(Number(xMin)) ? Number(xMin) : PLOT_X_MIN;
        const hi = isFinite(Number(xMax)) ? Number(xMax) : PLOT_X_MAX;
        const from = Math.min(lo, hi);
        const to = Math.max(lo, hi);
        const chart = this._ensureChart();

        // 每条函数曲线等距采样 200 点，断点（无穷/超大值）用 null 断开
        const N = 200;
        const palette = ['#6c9bcf', '#e8a13c', '#9d6bff', '#20c997', '#e05a6d', '#45c98a'];
        const theme = this._themeColors();
        const series = functions.map((fn, idx) => {
            const data = [];
            for (let i = 0; i <= N; i++) {
                const x = from + (to - from) * i / N;
                let y = null;
                try {
                    const v = math.evaluate(fn.expr, { x: x });
                    // 非数值、无穷与超大值（如 tan 渐近点）断开，避免拉平 y 轴
                    if (typeof v === 'number' && isFinite(v) && Math.abs(v) < 1e6) y = v;
                } catch (e) { y = null; }
                data.push([Number(x.toFixed(6)), y]);
            }
            return {
                name: fn.expr,
                type: 'line',
                showSymbol: false,
                connectNulls: false,
                data: data,
                lineStyle: { width: 2, color: palette[idx % palette.length] },
                itemStyle: { color: palette[idx % palette.length] }
            };
        });

        const option = Object.assign(this._baseOption(), {
            legend: {
                top: 2,
                textStyle: { color: theme.text, fontSize: 11 },
                itemWidth: 16, itemHeight: 8
            },
            series: series
        });
        chart.setOption(option, true);
        chart.resize();
    }

    handleResize() {
        if (this.chart) this.chart.resize();
    }
}

// 全局图表渲染器实例（ltpx-agent.js 经 window.chartRenderer 访问）
window.chartRenderer = new ChartRenderer(chartCanvas);
window.addEventListener('resize', () => window.chartRenderer.handleResize());

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

/** 追加一条操作记录 */
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

/** 清空操作记录 */
function clearStepPanel() {
    stepList.innerHTML = '<div class="step-empty">暂无操作记录。</div>';
}

clearStepBtn.addEventListener('click', clearStepPanel);

// ===== 历史记录（localStorage 持久化） =====

const HISTORY_KEY = 'msHistory';
const HISTORY_MAX = 30;

/** 最近一次渲染的结果载荷（供回执文本与历史写入使用） */
let lastResult = null;

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

/** 追加一条历史记录并刷新列表 */
function addHistory(kind, title, result) {
    const list = loadHistory();
    list.unshift({ kind: kind, title: title, result: result, time: new Date().toLocaleString('zh-CN', { hour12: false }) });
    saveHistory(list);
    renderHistory();
}

function renderHistory() {
    const list = loadHistory();
    historyList.innerHTML = '';
    resultCount.textContent = list.length > 0 ? '共 ' + list.length + ' 条' : '';
    if (list.length === 0) {
        historyList.innerHTML = '<div class="list-empty">暂无计算记录。</div>';
        return;
    }
    const ICONS = { calc: 'fa-calculator', solve: 'fa-equals', plot: 'fa-chart-line', agent: 'fa-robot', error: 'fa-circle-xmark' };
    list.forEach(item => {
        const el = document.createElement('div');
        el.className = 'history-item';
        el.innerHTML = '<span class="history-icon h-' + (item.kind || 'calc') + '"><i class="fas ' + (ICONS[item.kind] || ICONS.calc) + '"></i></span>'
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

function escapeHTML(text) {
    const div = document.createElement('div');
    div.textContent = String(text);
    return div.innerHTML;
}

/** KaTeX 公式渲染（katex 由标准依赖异步注入，未就绪时回退纯文本） */
function renderTexHTML(tex) {
    if (!tex) return '';
    try {
        if (typeof katex !== 'undefined') {
            return '<div class="katex-display-block">' + katex.renderToString(tex, { throwOnError: false, displayMode: false }) + '</div>';
        }
    } catch (e) { /* 回退纯文本 */ }
    return '';
}

/**
 * 渲染结果载荷到结果区（并记录最近一次结果供写入历史与回执）
 * 载荷类型：loading / error / calc / solve / plot
 */
function renderResult(payload) {
    if (!payload) return;
    lastResult = payload;
    if (payload.type === 'loading') {
        resultBody.innerHTML = '<div class="result-loading"><div class="spinner"></div><p>正在计算...</p></div>';
        return;
    }
    if (payload.type === 'error') {
        resultBody.innerHTML = '<div class="result-error"><i class="fas fa-circle-xmark"></i><p>' + escapeHTML(payload.error || '计算失败') + '</p></div>';
        return;
    }
    if (payload.type === 'calc') {
        // 多行表达式逐条展示
        const blocks = (payload.items || []).map(item =>
            '<div class="math-item">'
            + '<div class="math-expression"><i class="fas fa-square-root-variable"></i><span>' + escapeHTML(item.expr) + '</span></div>'
            + renderTexHTML(item.tex)
            + '<div class="math-value">' + escapeHTML(item.value) + '</div>'
            + '</div>'
        ).join('');
        resultBody.innerHTML = '<div class="math-block">' + blocks + '</div>';
        // 同步绘制图像
        if (payload.plotFns && payload.plotFns.length > 0) {
            window.chartRenderer.render(payload.plotFns, PLOT_X_MIN, PLOT_X_MAX).catch(err => showToast('图像绘制失败：' + err.message, 'error'));
        }
        return;
    }
    if (payload.type === 'solve') {
        const items = (payload.entries || []).map((entry, i) =>
            '<div class="solve-item"><span class="solve-index">' + (i + 1) + '</span><span>' + escapeHTML(entry.label + ' = ' + entry.value) + '</span></div>'
        ).join('');
        const eqLine = '<div class="math-expression"><i class="fas fa-equals"></i><span>' + escapeHTML((payload.equations || []).join('  ,  ')) + '</span></div>';
        resultBody.innerHTML = '<div class="math-block">' + eqLine + '<div class="solve-list">' + items + '</div>'
            + '<div class="math-note"><i class="fas fa-circle-info"></i>图像为 y = 左边 − 右边，解即曲线与 x 轴交点</div></div>';
        // 同步绘制图像
        if (payload.plotFns && payload.plotFns.length > 0) {
            window.chartRenderer.render(payload.plotFns, PLOT_X_MIN, PLOT_X_MAX).catch(err => showToast('图像绘制失败：' + err.message, 'error'));
        }
        return;
    }
    if (payload.type === 'plot') {
        const fnNames = (payload.functions || []).map(f => f.expr).join('、');
        const items = (payload.functions || []).map(f =>
            '<div class="solve-item"><span class="solve-index" style="background: var(--accent)"><i class="fas fa-chart-line"></i></span><span>' + escapeHTML('y = ' + f.expr) + '</span></div>'
        ).join('');
        resultBody.innerHTML = '<div class="math-block">'
            + '<div class="math-expression"><i class="fas fa-chart-line"></i><span>绘制范围 x ∈ [' + escapeHTML(payload.x_min) + ', ' + escapeHTML(payload.x_max) + ']</span></div>'
            + '<div class="solve-list">' + items + '</div>'
            + '<div class="math-note"><i class="fas fa-circle-info"></i>图表已绘制：' + escapeHTML(fnNames) + '，支持滚轮缩放与拖拽平移</div></div>';
        // 同步渲染图表
        window.chartRenderer.render(payload.functions, payload.x_min, payload.x_max).catch(err => {
            showToast('图像绘制失败：' + err.message, 'error');
        });
        return;
    }
    resultBody.innerHTML = '<div class="result-empty"><i class="fas fa-circle-info"></i> 暂无结果</div>';
}

/**
 * 由最近一次结果载荷生成回执汇总文本（传回月华）
 */
function buildReceiptText() {
    const p = lastResult;
    if (!p || p.type === 'error') {
        return '【数学计算】\n结果：计算失败' + (p && p.error ? '（' + p.error + '）' : '');
    }
    const lines = ['【数学计算】'];
    if (p.type === 'calc') {
        const items = p.items || [];
        lines.push('表达式：' + items.map(i => i.expr).join('；'));
        lines.push('类型：表达式求值');
        lines.push('结果：' + items.map(i => i.expr + ' = ' + i.value).join('；'));
        lines.push('图表：已绘制 ' + items.map(i => 'y = ' + i.expr).join('、') + '，范围 [' + PLOT_X_MIN + ',' + PLOT_X_MAX + ']');
    } else if (p.type === 'solve') {
        lines.push('表达式：' + (p.equations || []).join('；'));
        lines.push('类型：方程求解');
        lines.push('结果：' + (p.entries || []).map(e => e.label + ' = ' + e.value).join('；'));
        lines.push('图表：已绘制 ' + (p.plotFns || []).map(f => 'y = ' + f.expr).join('、') + '，范围 [' + PLOT_X_MIN + ',' + PLOT_X_MAX + ']');
    } else if (p.type === 'plot') {
        const fnNames = (p.functions || []).map(f => f.expr).join('、');
        lines.push('表达式：' + fnNames);
        lines.push('类型：函数绘图');
        lines.push('结果：已绘制 ' + fnNames + '，范围 [' + p.x_min + ', ' + p.x_max + ']');
        lines.push('图表：已绘制 ' + fnNames + '，范围 [' + p.x_min + ',' + p.x_max + ']');
    }
    return lines.join('\n');
}

// ===== 解析计算（自动识别内容：含 = 的行按方程求解，其余按表达式求值） =====

/** 拆分多行输入为表达式列表 */
function splitLines(text) {
    return String(text || '').split(/\n+/).map(s => s.trim()).filter(Boolean);
}

/**
 * 表达式求值并渲染（支持单条或多条表达式，图像绘制所有表达式曲线）
 * 供智能体 evaluate_expression 工具与手动输入复用
 * @param {string[]|string} expressions - 表达式列表
 * @param {string[]} [extraPlotFns] - 额外需要一并绘制的函数表达式（如 y= 定义行）
 */
async function runCalc(expressions, extraPlotFns) {
    const list = Array.isArray(expressions) ? expressions : [expressions];
    const items = list.map(e => mathEngine.evaluate(e));
    const plotFns = list.map(e => ({ expr: String(e).trim() }))
        .concat((extraPlotFns || []).map(e => ({ expr: String(e).trim() })));
    const payload = {
        type: 'calc',
        items: items,
        plotFns: plotFns
    };
    renderResult(payload);
    const summary = items.map(i => i.expr + ' = ' + i.value).join('；');
    addHistory('calc', summary, payload);
    appendStep('tool-result', '表达式求值：' + summary);
    // 单条时返回单项（兼容智能体工具返回格式），多条返回数组
    return items.length === 1 ? items[0] : items;
}

/**
 * 方程求解并渲染（图像绘制每条方程的 y = 左边 - 右边）
 * 供智能体 solve_equation 工具与手动输入复用
 */
async function runSolve(equations) {
    const result = mathEngine.solve(equations);
    const payload = {
        type: 'solve',
        equations: result.equations,
        entries: result.entries,
        plotFns: result.equations.map(eq => mathEngine.equationToPlotFn(eq))
    };
    renderResult(payload);
    addHistory('solve', result.display, payload);
    appendStep('tool-result', '方程求解：' + result.display);
    return result;
}

/**
 * 函数绘图并渲染（智能体 plot_function 工具专用入口）
 */
async function runPlot(functions, xMin, xMax) {
    const list = (Array.isArray(functions) ? functions : [functions])
        .map(f => ({ expr: String(typeof f === 'object' && f ? f.expr : f || '').trim() }))
        .filter(f => f.expr);
    if (list.length === 0) throw new Error('未提供函数表达式');
    const lo = (xMin === undefined || xMin === null || xMin === '') ? PLOT_X_MIN : Number(xMin);
    const hi = (xMax === undefined || xMax === null || xMax === '') ? PLOT_X_MAX : Number(xMax);
    if (!isFinite(lo) || !isFinite(hi)) throw new Error('绘图范围必须是数字');
    const payload = { type: 'plot', functions: list, x_min: lo, x_max: hi };
    renderResult(payload);
    const names = list.map(f => f.expr).join('、');
    addHistory('plot', '绘制 ' + names, payload);
    return '已绘制 ' + names + '，范围 [' + lo + ', ' + hi + ']';
}

// 暴露给 ltpx-agent.js 工具执行器复用
window.runCalc = runCalc;
window.runSolve = runSolve;
window.runPlot = runPlot;

/**
 * 「解析计算」入口：逐行自动识别内容并分流，固定绘制图像
 * - y = 表达式 / f(x) = 表达式：函数定义，取等号右侧绘图（恢复旧绘图模式行为）
 * - 其余含 = 的行：方程（组）求解
 * - 无 = 的行：可求值则求值；含自由变量（如 x）则自动按函数绘图
 */
async function handleExecute() {
    const lines = splitLines(formulaInput.value);
    if (lines.length === 0) { formulaInput.focus(); return; }
    renderResult({ type: 'loading' });
    appendStep('info', '解析计算：' + lines.join('；'));
    try {
        const equations = []; // 待求解方程
        const functions = []; // 函数定义/含变量表达式（绘图）
        const exprs = [];     // 可数值求值的表达式

        for (const line of lines) {
            if (line.includes('=')) {
                const idx = line.indexOf('=');
                const lhs = line.slice(0, idx).trim();
                const rhs = line.slice(idx + 1).trim();
                // y = ... 或 f(x) = ... 形式按函数定义处理，取右侧表达式绘图
                if (lhs === 'y' || /^\w+\s*\(/.test(lhs)) {
                    if (!rhs) throw new Error('「' + line + '」缺少等号右侧表达式');
                    functions.push(rhs);
                } else {
                    equations.push(line);
                }
            } else {
                // 先试求数值：含自由变量（如 x）无法求值时，自动降级为函数绘图
                try {
                    math.evaluate(line);
                    exprs.push(line);
                } catch (err) {
                    if (/undefined symbol|is not defined/i.test(err.message || '')) {
                        functions.push(line);
                    } else {
                        throw err;
                    }
                }
            }
        }

        if (equations.length > 0) {
            if (functions.length === 0 && exprs.length === 0) {
                // 纯方程（组）：现有求解逻辑（图像为 y = 左边 - 右边）
                await runSolve(equations);
            } else {
                // 方程与函数/表达式混合：求解方程，图像叠加方程差值曲线与函数曲线
                const result = mathEngine.solve(equations);
                const payload = {
                    type: 'solve',
                    equations: result.equations,
                    entries: result.entries,
                    plotFns: result.equations.map(eq => mathEngine.equationToPlotFn(eq))
                        .concat(functions.map(f => ({ expr: f })), exprs.map(e => ({ expr: e })))
                };
                renderResult(payload);
                addHistory('solve', result.display, payload);
                appendStep('tool-result', '方程求解：' + result.display);
            }
        } else if (exprs.length > 0) {
            // 表达式求值（函数定义行一并绘入图像）
            await runCalc(exprs, functions);
        } else if (functions.length > 0) {
            // 纯函数绘图（同旧绘图模式行为）
            const payload = { type: 'plot', functions: functions.map(f => ({ expr: f })), x_min: PLOT_X_MIN, x_max: PLOT_X_MAX };
            renderResult(payload);
            addHistory('plot', '绘制 ' + functions.join('、'), payload);
            appendStep('tool-result', '函数绘图：' + functions.join('、'));
        }
    } catch (e) {
        const msg = (e && e.message) || String(e);
        renderResult({ type: 'error', error: msg });
        addHistory('error', lines.join('；'), { type: 'error', error: msg });
        appendStep('error', '计算失败：' + msg);
        showToast(msg, 'error');
    }
}

execBtn.addEventListener('click', handleExecute);
formulaInput.addEventListener('keydown', (e) => {
    // Ctrl + Enter 快捷执行（Enter 保留换行）
    if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
        e.preventDefault();
        handleExecute();
    }
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
            keep_open: true // 计算结果保持页面展示，供用户观察图像与过程
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
        const result = await runMathAgent(instruction, (step) => appendStep(step.kind, step.text));
        // 回执 = 结构化计算汇总（传回月华系统）+ 智能体答复
        const receipt = buildReceiptText() + '\n答复：' + (result.text || '');
        postLTPXResult(data.request_id, result.success, receipt, result.error);
        // 将智能体执行结果写入历史记录
        addHistory('agent', '月华指令：' + instruction, lastResult || { type: 'error', error: result.text || '执行失败' });
    } catch (e) {
        console.error('LTPX AtoA 执行失败:', e);
        appendStep('error', '执行失败：' + (e.message || e));
        renderResult({ type: 'error', error: (e.message || '执行失败') });
        addHistory('error', '月华指令执行失败：' + instruction, { type: 'error', error: (e.message || '执行失败') });
        postLTPXResult(data.request_id, false, '', e.message || '执行失败');
    }
});

// ===== 初始化 =====
clearStepPanel();
renderHistory();
// 标准依赖就绪后绘制初始空图像
if (window.scriptDependenciesReady) {
    window.scriptDependenciesReady.then(() => window.chartRenderer.showEmpty());
} else {
    window.chartRenderer.showEmpty();
}
