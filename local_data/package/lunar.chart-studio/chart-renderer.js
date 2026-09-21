// ============================================================
// 『 星月智能 』图表工坊 — 图表渲染核心
// 渲染器与配置复制自月华消息终端（lunar_astral/hierarchy/assets/client）：
// - ECharts：markdown ```echarts 代码块 → JSON.parse → echarts.init + setOption
// - Mermaid：markdown ```mermaid 代码块 → mermaid.parse/render → SVG 容器
// - Mermaid 初始化配置与月华一致（startOnLoad:false / securityLevel:loose / fontFamily:inherit）
// 差异适配：实时编辑器会反复重渲染，故 ECharts 实例统一登记并在重渲染前 dispose，
// 尺寸观察器随实例一起断开；主题切换时重建 Mermaid 初始化。
// 尺寸策略（图表工坊）：容器高度由内容决定，不设上限，滚动交给预览区。
// - ECharts：画布尺寸恒等于容器可见尺寸（容器高度由 CSS aspect-ratio 按宽高比给出），
//   字号与留白都是自然大小，绘图区不缩不放，比例恒定
// - Mermaid：按 viewBox 自然尺寸排版后等比缩放到容器宽度，容器高度取缩放后的实际高度
// 两者都不会出现「画布外内容被容器裁掉」，也不会把图形整体缩小到看不清
// 取景策略（图表工坊）：取景窗与画布的位置、尺寸恒定不动，绘制在画布显示区之外的图表内容
// 通过「拖动 → 对图表内容整体施加偏移量 → 重绘」来查看
// - 不缩放：滚轮不再改变图表尺寸（滚轮事件交还预览区滚动）
// - 不平移画布：不对画布/内容层做 transform，画布位置与尺寸始终不变
// - 拖动：偏移量交给渲染器重新绘制内容 —— ECharts 平移 option 的定位锚点后 setOption 重绘，
//   Mermaid 改写 SVG viewBox 重绘（图形本体不动）；原先因超出渲染范围而隐藏的部分随重绘进入可见区
// ============================================================

// ---------- 渲染器状态 ----------
let mermaidInitialized = false;
/** 当前存活的图表运行时（重渲染前统一断开尺寸观察器并 dispose，防泄漏） */
const activeChartRuntimes = [];
/** Mermaid 相对自然尺寸的最大放大倍数（小图放大铺满容器，同时保持文字清晰） */
const MERMAID_MAX_UPSCALE = 1.5;
/** 内容偏移的最大范围（视口尺寸的倍数）：拖动时图表内容最多整体偏移半个视口，避免把图表拖出视野 */
const CONTENT_OFFSET_LIMIT = 0.5;

// ---------- Markdown / Mermaid / ECharts 渲染（复制自月华消息终端 markdown.js） ----------

/** 等待 marked 就绪（standard_dependency 异步注入，最多等 5 秒） */
async function ensureMarked() {
    if (window.marked) return true;
    for (let i = 0; i < 50; i++) {
        await new Promise(r => setTimeout(r, 100));
        if (window.marked) return true;
    }
    return false;
}

/** 初始化 Mermaid（配置与月华消息终端一致；主题随深色模式） */
function initMermaid() {
    if (mermaidInitialized || !window.mermaid) return;
    window.mermaid.initialize({
        startOnLoad: false,
        theme: document.body.classList.contains('dark-mode') ? 'dark' : 'default',
        securityLevel: 'loose',
        fontFamily: 'inherit'
    });
    mermaidInitialized = true;
}

/** 主题切换后重建 Mermaid 初始化（主题是初始化时固化的） */
function resetMermaidTheme() {
    mermaidInitialized = false;
    initMermaid();
}

function processThinkTags(html) {
    return html.replace(/<think>([\s\S]*?)<\/think>/gi, (match, content) => {
        return `<div class="think-block"><div class="think-summary"><i class="fas fa-chevron-right toggle-icon"></i> 思考过程</div><div class="think-content">${content}</div></div>`;
    });
}

async function renderMarkdown(rawContent) {
    let html = processThinkTags(rawContent);
    if (window.marked) {
        html = await window.marked.parse(html);
    } else {
        html = '<p>' + escapeHTML(rawContent).replace(/\n/g, '<br>') + '</p>';
    }
    return html;
}

function highlightCode(container) {
    if (!window.hljs) return;
    container.querySelectorAll('pre code').forEach((block) => {
        if (block.parentElement && block.parentElement.classList.contains('hljs')) return;
        const langClass = Array.from(block.classList).find(c => c.startsWith('language-'));
        if (langClass && (langClass === 'language-echarts' || langClass === 'language-mermaid')) return;
        try {
            window.hljs.highlightElement(block);
        } catch (e) {
            console.warn('代码高亮失败', e);
        }
    });
}

/** ECharts 渲染：```echarts 代码块 → JSON 配置 → 图表容器（与月华消息终端同配置） */
function renderECharts(container) {
    if (!window.echarts) return;
    container.querySelectorAll('pre code.language-echarts').forEach((block) => {
        try {
            const rawOption = JSON.parse(block.textContent.trim());
            const chartDiv = document.createElement('div');
            chartDiv.className = 'echarts-container';
            const viewport = document.createElement('div');
            viewport.className = 'chart-viewport';
            const stage = document.createElement('div');
            stage.className = 'chart-viewport-inner';
            const inner = document.createElement('div');
            inner.style.width = '100%';
            inner.style.height = '100%';
            stage.appendChild(inner);
            viewport.appendChild(stage);
            chartDiv.appendChild(viewport);
            const pre = block.parentElement;
            if (pre && pre.tagName === 'PRE') {
                pre.replaceWith(chartDiv);
            } else {
                block.replaceWith(chartDiv);
            }
            // 画布尺寸 = 容器可见尺寸：容器高度由 CSS aspect-ratio 按宽高比给出，
            // 图表按真实像素渲染（字号、坐标轴、留白都是自然大小），不缩不放、比例恒定，
            // 画布的位置与尺寸恒定，查看画布显示区之外的内容靠「内容偏移 + 重绘」实现
            stage.style.width = (viewport.clientWidth || 700) + 'px';
            stage.style.height = (viewport.clientHeight || 420) + 'px';
            const chart = window.echarts.init(inner);
            chartDiv._echartsInstance = chart;
            /** 图表内容偏移量（px）：拖动只改它，画布与取景窗不动 */
            const offset = { x: 0, y: 0 };
            /** 基准配置（按画布尺寸归一化后的 option）及其尺寸签名，仅在画布尺寸变化时重算 */
            let baseOption = null, baseSize = '';
            /** 重绘：把内容偏移量注入配置后 setOption，图表内容整体平移（画布本身不动） */
            const draw = () => {
                const w = viewport.clientWidth || 700, h = viewport.clientHeight || 420;
                const key = w + 'x' + h;
                if (key !== baseSize) {
                    baseSize = key;
                    stage.style.width = w + 'px';
                    stage.style.height = h + 'px';
                    baseOption = normalizeEChartsOption(cloneChartOption(rawOption), w, h);
                    try { chart.resize(); } catch (e) { /* 图表已释放 */ }
                }
                const option = shiftChartOption(cloneChartOption(baseOption), offset, w, h);
                // 拖动按帧重绘，更新过程不做补间动画，图表内容才能实时跟手
                option.animationDurationUpdate = 0;
                chart.setOption(option);
            };
            const ctrl = attachContentOffset(viewport, offset, draw);
            activeChartRuntimes.push(observeChartViewport(viewport, ctrl, chart));
            ctrl.refresh();
        } catch (e) {
            console.warn('ECharts 渲染失败', e);
            const errDiv = document.createElement('div');
            errDiv.className = 'mermaid-error';
            errDiv.textContent = 'ECharts 渲染失败：' + (e.message || String(e));
            const pre = block.parentElement;
            if (pre && pre.tagName === 'PRE') pre.replaceWith(errDiv);
            else block.replaceWith(errDiv);
        }
    });
}

// ---------- 图表位置归一化 / 图表内容偏移 ----------

/**
 * 圆心/位置值钳制：百分比限制在 [30%,70%]，像素值限制在 [0.3,0.7]×画布维度，
 * 非法值回退默认，确保饼图/仪表盘等永远落在画布可见区内
 */
function clampCenterDim(v, dim, def) {
    const m = String(v).match(/^(-?[\d.]+)%$/);
    if (m) {
        let n = parseFloat(m[1]);
        if (isNaN(n)) return def;
        n = Math.max(30, Math.min(70, n));
        return n + '%';
    }
    const n = parseFloat(v);
    if (isNaN(n)) return def;
    return Math.max(dim * 0.3, Math.min(dim * 0.7, n));
}

/**
 * 智能体输出配置的位置归一化：修正会导致图表跑到显示区外的常见问题
 * - 坐标轴缺 grid.containLabel → 补上，轴标签/刻度自动收缩进画布内，防止贴边被裁切
 * - 饼图/环形图缺 center → 补默认居中；center（百分比/像素）越界 → 钳制回画布中央区
 * - 仪表盘 center 同理；雷达图 radar.center 同理
 * - 饼图系列补 labelLayout.hideOverlap，防止标签互相重叠挤出画面
 * @param {Object} config - ECharts option
 * @param {number} lw - 画布宽（px 值钳制基准，等于容器可见宽）
 * @param {number} lh - 画布高（等于容器可见高）
 */
function normalizeEChartsOption(config, lw, lh) {
    lw = lw || 960; lh = lh || 600;
    try {
        // 坐标轴容器：containLabel 把轴标签收进画布内（不被容器边缘裁切）；
        // 未声明 grid 时给一份紧凑默认值，避免 ECharts 默认上下各 60px 留白吃掉绘图区
        if (!config.grid) {
            config.grid = { left: 12, right: 16, top: 44, bottom: 20, containLabel: true };
        } else if (!Array.isArray(config.grid) && config.grid.containLabel === undefined) {
            config.grid.containLabel = true;
        }
        const seriesList = Array.isArray(config.series) ? config.series : (config.series ? [config.series] : []);
        for (const s of seriesList) {
            if (!s || typeof s !== 'object') continue;
            if (s.type === 'pie' || s.type === 'gauge') {
                const def = s.type === 'pie' ? ['50%', '54%'] : ['50%', '58%'];
                const c = Array.isArray(s.center) ? s.center : def;
                s.center = [clampCenterDim(c[0], lw, def[0]), clampCenterDim(c[1], lh, def[1])];
                if (s.type === 'pie' && s.labelLayout === undefined) s.labelLayout = { hideOverlap: true };
            }
        }
        if (config.radar) {
            const c = Array.isArray(config.radar.center) ? config.radar.center : ['50%', '50%'];
            config.radar.center = [clampCenterDim(c[0], lw, '50%'), clampCenterDim(c[1], lh, '50%')];
        }
    } catch (e) { /* 归一化失败不影响原配置渲染 */ }
    return config;
}

/** 深拷贝 option：归一化与偏移都会改写配置，须在副本上进行，保证基准配置可反复复用 */
function cloneChartOption(option) {
    return JSON.parse(JSON.stringify(option || {}));
}

/** 锚点值换算为像素：数字原样返回，百分比按画布对应维度换算，居中类关键字无法换算（返回 null 保持原样） */
function anchorToPx(v, dim) {
    if (typeof v === 'number') return isNaN(v) ? null : v;
    if (typeof v === 'string') {
        const m = v.match(/^(-?[\d.]+)%$/);
        if (m) {
            const n = parseFloat(m[1]);
            if (!isNaN(n)) return n / 100 * dim;
        }
    }
    return null;
}

/** 单侧锚点位移：换算成像素后叠加偏移量；无法换算的锚点保持原样（不影响居中布局） */
function shiftAnchor(node, key, delta, dim) {
    const px = anchorToPx(node[key], dim);
    if (px !== null) node[key] = px + delta;
}

/** 盒子定位锚点整体位移：left/right/top/bottom 同步平移（未声明的锚点保持未声明） */
function shiftBoxAnchor(node, ox, oy, lw, lh) {
    if (!node || typeof node !== 'object') return;
    shiftAnchor(node, 'left', ox, lw);
    shiftAnchor(node, 'right', -ox, lw);
    shiftAnchor(node, 'top', oy, lh);
    shiftAnchor(node, 'bottom', -oy, lh);
}

/** 中心点锚点位移：center / layoutCenter（饼图、仪表盘、雷达、极坐标、地理布局） */
function shiftCenterAnchor(node, key, ox, oy, lw, lh) {
    if (!node || !Array.isArray(node[key])) return;
    const cx = anchorToPx(node[key][0], lw), cy = anchorToPx(node[key][1], lh);
    node[key] = [cx === null ? node[key][0] : cx + ox, cy === null ? node[key][1] : cy + oy];
}

/** 组件既可能写成单个对象也可能写成数组，统一按对象逐个处理 */
function forEachOptionNode(node, fn) {
    if (Array.isArray(node)) node.forEach(n => fn(n));
    else if (node && typeof node === 'object') fn(node);
}

/**
 * 图表内容整体偏移：把 option 中决定「内容绝对位置」的锚点统一平移 (ox, oy) 像素后重新绘制
 * 平移的是图表内容本身，画布与取景窗的尺寸、位置保持不变；原先绘制在画布显示区之外的部分
 * 会随重绘进入可见区
 * - 覆盖：grid / title / legend / toolbox / visualMap / dataZoom 的盒子锚点，
 *   polar / radar 的中心，geo 的 layoutCenter，series 的 center 与 left/top/right/bottom，
 *   graphic 顶层元素的位置
 * - 百分比锚点先按画布尺寸换算成像素再偏移，保证各锚点同步位移
 * @param {Object} option - 已归一化的配置副本
 * @param {{x:number,y:number}} offset - 内容偏移量（px）
 * @param {number} lw - 画布宽
 * @param {number} lh - 画布高
 */
function shiftChartOption(option, offset, lw, lh) {
    const ox = offset.x, oy = offset.y;
    if (!ox && !oy) return option;
    ['grid', 'title', 'legend', 'toolbox', 'visualMap', 'dataZoom'].forEach(key => {
        forEachOptionNode(option[key], node => shiftBoxAnchor(node, ox, oy, lw, lh));
    });
    forEachOptionNode(option.polar, node => shiftCenterAnchor(node, 'center', ox, oy, lw, lh));
    forEachOptionNode(option.radar, node => shiftCenterAnchor(node, 'center', ox, oy, lw, lh));
    // geo 的 center 是经纬度，不能动；画布内的位置由 layoutCenter 决定
    forEachOptionNode(option.geo, node => shiftCenterAnchor(node, 'layoutCenter', ox, oy, lw, lh));
    forEachOptionNode(option.series, node => {
        shiftCenterAnchor(node, 'center', ox, oy, lw, lh);
        shiftBoxAnchor(node, ox, oy, lw, lh);
    });
    forEachOptionNode(option.graphic, node => {
        shiftBoxAnchor(node, ox, oy, lw, lh);
        // graphic 元素也可用 x/y 定位，与 left/top 等价须同步位移；组内元素坐标相对父级，不重复位移
        const x = anchorToPx(node.x, lw), y = anchorToPx(node.y, lh);
        if (x !== null) node.x = x + ox;
        if (y !== null) node.y = y + oy;
    });
    return option;
}

// ---------- 尺寸同步（容器尺寸变化时重算绘制尺寸） ----------

/** 读取 SVG 自然取景范围：优先 viewBox（含原点），其次 width/height 属性，最后退回元素测量值 */
function svgNaturalSize(svgEl) {
    if (!svgEl) return { x: 0, y: 0, w: 600, h: 400 };
    const vb = svgEl.viewBox && svgEl.viewBox.baseVal;
    if (vb && vb.width > 0 && vb.height > 0) return { x: vb.x, y: vb.y, w: vb.width, h: vb.height };
    const rect = svgEl.getBoundingClientRect();
    const w = parseFloat(svgEl.getAttribute('width')) || rect.width || 600;
    const h = parseFloat(svgEl.getAttribute('height')) || rect.height || 400;
    return { x: 0, y: 0, w: w, h: h };
}

/**
 * ECharts 视口尺寸同步：容器尺寸变化（窗口缩放、面板宽度调整导致宽高比重算）时重算画布尺寸并重绘
 * 画布尺寸 = 容器可见尺寸，图表永远完整铺满取景窗、比例恒定
 * @param {HTMLElement} viewport - 视口容器
 * @param {{refresh:Function}} ctrl - 内容偏移控制器（重算偏移范围并重绘）
 * @param {Object} chart - ECharts 实例（登记进运行时，重渲染时统一 dispose）
 * @returns {{ chart:Object, observer:ResizeObserver }} 运行时句柄（重渲染时断开）
 */
function observeChartViewport(viewport, ctrl, chart) {
    const size = { w: 0, h: 0 };
    const sync = () => {
        const w = viewport.clientWidth, h = viewport.clientHeight;
        if (!w || !h || (w === size.w && h === size.h)) return;
        size.w = w; size.h = h;
        ctrl.refresh();
    };
    const observer = new ResizeObserver(sync);
    observer.observe(viewport);
    return { chart: chart, observer: observer };
}

/**
 * Mermaid 视口尺寸同步：容器宽度变化时重算铺满视图
 * 以宽度为签名，自身高度被重绘改写不会触发重算（避免观察器自激）
 * @param {HTMLElement} viewport - 视口容器
 * @param {{refresh:Function}} ctrl - 内容偏移控制器（重算偏移范围并重绘）
 * @returns {{ observer:ResizeObserver }} 运行时句柄（重渲染时断开）
 */
function observeMermaidViewport(viewport, ctrl) {
    let sig = 0;
    const sync = () => {
        const vw = viewport.clientWidth;
        if (!vw || vw === sig) return;
        sig = vw;
        ctrl.refresh();
    };
    const observer = new ResizeObserver(sync);
    observer.observe(viewport);
    return { observer: observer };
}

/**
 * 图表内容偏移交互：拖动只对「图表内容」整体施加偏移量并重绘，画布与取景窗保持不动
 * - 不缩放：不监听滚轮，滚轮事件交还预览区正常滚动
 * - 不平移画布：不对画布/内容层做 transform，画布的尺寸与位置始终不变
 * - 拖动：累积偏移量（px）后按帧交给渲染器重绘，原先绘制在画布显示区之外、未能显示的部分
 *   随偏移重绘进可见区
 * - 偏移范围钳制在半个视口内，避免把图表拖出视野；双击复位内容偏移
 * @param {HTMLElement} viewport - 视口容器（overflow hidden）
 * @param {{x:number,y:number}} offset - 内容偏移量（重绘时读取）
 * @param {Function} redraw - 重绘回调：按当前 offset 重新绘制图表内容
 * @returns {{ refresh: Function }} refresh()：尺寸变化后重算偏移范围并重绘
 */
function attachContentOffset(viewport, offset, redraw) {
    let dragging = false, startX = 0, startY = 0, baseX = 0, baseY = 0, raf = 0;
    /** 偏移量钳制：不超出半个视口，任何情况下都能把图表拖回取景窗 */
    const clamp = () => {
        const mx = viewport.clientWidth * CONTENT_OFFSET_LIMIT;
        const my = viewport.clientHeight * CONTENT_OFFSET_LIMIT;
        offset.x = Math.max(-mx, Math.min(mx, offset.x));
        offset.y = Math.max(-my, Math.min(my, offset.y));
    };
    /** 按帧重绘：拖动过程中持续重绘，图表内容实时跟随鼠标 */
    const schedule = () => {
        if (raf) return;
        raf = requestAnimationFrame(() => { raf = 0; redraw(); });
    };
    // 拖动施加偏移：pointer capture 保证拖出取景窗仍能继续，且不向 window 泄漏监听
    viewport.addEventListener('pointerdown', (e) => {
        if (e.button !== 0) return;
        dragging = true;
        startX = e.clientX; startY = e.clientY;
        baseX = offset.x; baseY = offset.y;
        viewport.setPointerCapture(e.pointerId);
        viewport.classList.add('grabbing');
        e.preventDefault();
    });
    viewport.addEventListener('pointermove', (e) => {
        if (!dragging) return;
        offset.x = baseX + (e.clientX - startX);
        offset.y = baseY + (e.clientY - startY);
        clamp();
        schedule();
    });
    const endDrag = () => {
        if (!dragging) return;
        dragging = false;
        viewport.classList.remove('grabbing');
        if (raf) { cancelAnimationFrame(raf); raf = 0; }
        redraw();
    };
    viewport.addEventListener('pointerup', endDrag);
    viewport.addEventListener('pointercancel', endDrag);
    viewport.addEventListener('lostpointercapture', endDrag);
    // 双击复位内容偏移（画布与取景窗本就没有移动，复位的是图表内容偏移量）
    viewport.addEventListener('dblclick', () => {
        offset.x = 0; offset.y = 0;
        redraw();
    });
    // 操作提示（悬停浮现）
    const hint = document.createElement('span');
    hint.className = 'chart-viewport-hint';
    hint.textContent = '拖动查看画布显示区外的内容 · 双击复位';
    viewport.appendChild(hint);
    return { refresh: () => { clamp(); redraw(); } };
}

/** Mermaid 渲染：```mermaid 代码块 → SVG（与月华消息终端同配置与容错展示） */
async function renderMermaid(container) {
    if (!window.mermaid || !mermaidInitialized) return;
    const blocks = Array.from(container.querySelectorAll('pre code.language-mermaid'));
    for (const block of blocks) {
        const code = (block.textContent || '').trim();
        if (!code) continue;
        try {
            await window.mermaid.parse(code);
            const id = `mermaid-${Date.now()}-${Math.floor(Math.random() * 10000)}`;
            const { svg } = await window.mermaid.render(id, code);
            const wrapper = document.createElement('div');
            wrapper.className = 'mermaid-container';
            const viewport = document.createElement('div');
            viewport.className = 'chart-viewport';
            const stage = document.createElement('div');
            stage.className = 'chart-viewport-inner';
            stage.innerHTML = svg;
            viewport.appendChild(stage);
            wrapper.appendChild(viewport);
            const pre = block.parentElement;
            if (pre && pre.tagName === 'PRE') {
                pre.replaceWith(wrapper);
            } else {
                block.replaceWith(wrapper);
            }
            // SVG 按 viewBox 自然尺寸排版：缩放通过设置 svg 元素尺寸实现（不对内容层做 transform），
            // 图表内容整体等比缩放到容器宽度，容器高度取缩放后的实际高度
            const svgEl = stage.querySelector('svg');
            const nat = svgNaturalSize(svgEl);
            if (svgEl) svgEl.style.maxWidth = 'none';
            /** 图表内容偏移量（px）：拖动只改它，svg 元素与取景窗不动 */
            const offset = { x: 0, y: 0 };
            /** 重绘：按当前容器宽度重算缩放，并把内容偏移量写进 viewBox（图形本体不移动） */
            const draw = () => {
                const vw = viewport.clientWidth || nat.w;
                const scale = Math.min(vw / nat.w, MERMAID_MAX_UPSCALE);
                const ew = Math.max(1, Math.round(nat.w * scale));
                const eh = Math.max(1, Math.round(nat.h * scale));
                stage.style.width = ew + 'px';
                stage.style.height = eh + 'px';
                viewport.style.height = eh + 'px';
                if (!svgEl) return;
                svgEl.setAttribute('width', ew);
                svgEl.setAttribute('height', eh);
                // 内容偏移：取景窗（viewBox）在自然坐标系里整体平移，原本落在取景窗外的部分随之显示；
                // 原点沿用 SVG 原始 viewBox，避免丢掉 Mermaid 预留的边距
                svgEl.setAttribute('viewBox', [(nat.x - offset.x / scale), (nat.y - offset.y / scale), nat.w, nat.h].join(' '));
            };
            const ctrl = attachContentOffset(viewport, offset, draw);
            activeChartRuntimes.push(observeMermaidViewport(viewport, ctrl));
            ctrl.refresh();
        } catch (e) {
            console.error('Mermaid 渲染失败', e);
            const errDiv = document.createElement('div');
            errDiv.className = 'mermaid-error';
            errDiv.textContent = `Mermaid 渲染失败：${e.message || String(e)}`;
            const pre = block.parentElement;
            if (pre && pre.tagName === 'PRE') {
                pre.replaceWith(errDiv);
            } else {
                block.replaceWith(errDiv);
            }
        }
    }
}

/** 释放当前所有图表运行时：断开尺寸观察器并 dispose ECharts 实例（实时编辑器重渲染前调用，防泄漏） */
function disposeChartInstances() {
    while (activeChartRuntimes.length) {
        const rt = activeChartRuntimes.pop();
        try { rt.observer.disconnect(); } catch (e) { /* 观察器可能已失效 */ }
        if (rt.chart) {
            try { rt.chart.dispose(); } catch (e) { /* 实例可能已被容器移除释放 */ }
        }
    }
}

// ---------- 预览编排 ----------

/**
 * 将 Markdown 源码完整渲染到预览容器（高亮 → ECharts → Mermaid）
 * @param {HTMLElement} container - 预览容器（内含 .markdown-content）
 * @param {string} raw - Markdown 源码
 */
async function renderChartPreview(container, raw) {
    const contentDiv = container.querySelector('.markdown-content');
    if (!contentDiv) return;
    disposeChartInstances();
    if (!String(raw || '').trim()) {
        contentDiv.innerHTML = '<div class="result-empty"><i class="fas fa-chart-line"></i> 编辑左侧源码或等待月华指令，图表将实时渲染在此处。</div>';
        return;
    }
    contentDiv.innerHTML = '<i class="fas fa-spinner fa-pulse"></i> 渲染中...';
    const html = await renderMarkdown(raw);
    contentDiv.innerHTML = html;
    contentDiv.querySelectorAll('table').forEach(t => t.classList.add('markdown-table'));
    highlightCode(contentDiv);
    renderECharts(contentDiv);
    await renderMermaid(contentDiv);
}

// ---------- 源码工具 ----------

/**
 * 统计源码中的图表块：返回 { echarts, mermaid, total }（供预览区角标展示）
 * @param {string} source - Markdown 源码
 */
function countChartBlocks(source) {
    const counts = { echarts: 0, mermaid: 0, total: 0 };
    const re = /```(echarts|mermaid)\b/gi;
    let m;
    while ((m = re.exec(String(source || ''))) !== null) {
        counts[m[1].toLowerCase()]++;
        counts.total++;
    }
    return counts;
}

/** HTML 转义（页面逻辑与渲染器共用） */
function escapeHTML(text) {
    const div = document.createElement('div');
    div.textContent = String(text);
    return div.innerHTML;
}
