// ==== engine.js — 月华 Live2D 引擎层 · 引擎入口 ====
//
// 职责：
//   - 与基岩引擎（lunar.rendering_engine）同构的统一通信：
//     接收走 /ws（宿主以 ?bus=0 退出原始总线订阅，仅消费结构化 engine 指令），
//     收发都走同一条 /ws 连接；频道名通过 URL ?channel= 指定。
//   - 初始化顺序：waitForPIXI → waitForLive2DPlugin → PIXI.Application →
//     读取 setting.json → 加载模型 → 自适应取景 → 广播 engine_ready
//   - 入站 engine 指令：panel_ready 握手 / action 动作映射 / mode_changed 记录 /
//     query_molang 回包；movement/rotation 等 3D 语义指令安全忽略
//   - 取景：按模型实际尺寸与容器比例自适应缩放，完整可见时居中、
//     容器偏窄时按「桌面宠物」取景（头顶贴近上沿、下半身自然裁切）
//   - 交互：滚轮 / 双指缩放（以指针为中心）、鼠标 / 触屏拖动、
//     双击或中键复位、resize 自动重排
//   - 生命周期：重复加载/失败时安全销毁模型，避免 WebGL 上下文泄漏
//
// 注意：本文件是 ES Module，但三个依赖库均为 UMD 全局脚本，
// 必须使用 window.PIXI（禁止 import pixi，否则会得到另一个实例）。

// ==== URL 参数 ====
const urlParams = new URLSearchParams(window.location.search);

// 频道名（宿主既有值：桌面 message-terminal-renderer / 移动 lunar-astral-renderer）
const CHANNEL_NAME = urlParams.get('channel') || 'live2d-engine';
const SOURCE_ENGINE = 'engine';

// 模型基址（?model= 可覆盖，默认指向公共 Live2D 模型目录，与基岩引擎 /file/read/models/bedrock/ 对称）
const MODEL_BASE_URL = urlParams.get('model') || '/file/read/models/live2d/';

// ?scale= 存在时锁定为手动缩放，否则完全按容器自适应
const MANUAL_SCALE = urlParams.has('scale') ? Number(urlParams.get('scale')) : null;

// ==== 统一通信客户端 ====
const channel = new WsBridge(CHANNEL_NAME);
/** 前端动作匹配：后端只推送消息，动作由本引擎按 50% 概率自行匹配 */
const actionMatcher = window.LunarActionMatcher
    ? new window.LunarActionMatcher({ endpoint: '/agent/embeddings' })
    : null;

// ==== 状态 ====
let app = null;
let model = null;
let currentEmotion = 'idle';
let emotionTimer = null;
let loading = false;
let setting = {
    name: 'live2d',
    url: '',
    scale: 0.15,
    x: 0.5,
    y: 0.5,
    mapping: {},
    autoInteract: true,
};

// 视图变换：base 为自适应基准缩放，zoom 为用户缩放倍率，pan 为平移（容器像素）
const view = {
    base: 0,
    zoom: 1,
    panX: 0,
    panY: 0,
};

const MIN_ZOOM = 0.4;
const MAX_ZOOM = 4;

/** setting.json 缺失时的兜底动作映射（键 → 候选动作组，取首个模型确实存在的组） */
const DEFAULT_MAPPING = {
    idle: ['Idle', 'Tap', 'Tap@Body'],
    thinking: ['Idle', 'Tap'],
    speaking: ['Talk@Body', 'Tap@Body', 'Idle'],
    happy: ['Tap@Body', 'Tap', 'Idle'],
    sad: ['Idle', 'Tap'],
    angry: ['Flick@Body', 'Flick', 'Idle'],
};

// ============================================================
//  启动
// ============================================================
async function init() {
    try {
        await waitForPIXI();
        await waitForLive2DPlugin();
        initApplication();
        bindInteractions();
        observeContainer();
        await loadSetting();
        await loadModel();
        resetView();
        setEmotion('idle');
        broadcast('engine_ready', { mode: 'embedded' });
        // 模型加载完成即上报可用动作，供面板与总线参与者读取
        broadcastAnimationList();
    } catch (err) {
        console.error('[Live2D] 初始化失败:', err);
        showError(err);
        broadcast('engine_error', { message: String((err && err.message) || err) });
    } finally {
        hideLoadingMask();
    }
}

/** 等待 PIXI 全局对象就绪（经典脚本已同步执行，这里仅作兜底轮询） */
function waitForPIXI() {
    return new Promise((resolve) => {
        (function check() {
            if (window.PIXI) resolve();
            else setTimeout(check, 100);
        })();
    });
}

/** 等待 pixi-live2d-display 插件挂载到 PIXI.live2d */
function waitForLive2DPlugin() {
    return new Promise((resolve) => {
        (function check() {
            if (window.PIXI && window.PIXI.live2d) resolve();
            else setTimeout(check, 100);
        })();
    });
}

/** 初始化 PIXI 应用（透明背景，画布铺满容器） */
function initApplication() {
    if (app) return;
    const container = document.getElementById('live2d-container');
    const canvas = document.getElementById('live2d-canvas');
    app = new window.PIXI.Application({
        view: canvas,
        transparent: true,
        antialias: true,
        width: container?.clientWidth || window.innerWidth,
        height: container?.clientHeight || window.innerHeight,
        autoStart: true,
    });
}

/** 读取 setting.json（容忍注释与单引号，与基岩侧读取习惯一致） */
async function loadSetting() {
    try {
        const resp = await fetch(MODEL_BASE_URL + 'setting.json');
        if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
        const raw = await resp.text();
        const jsonText = raw
            .replace(/\/\/.*$/gm, '')
            .replace(/\/\*[\s\S]*?\*\//g, '')
            .replace(/'/g, '"');
        Object.assign(setting, JSON.parse(jsonText));
    } catch (e) {
        console.warn('[Live2D] setting.json 读取失败，使用内置默认:', e);
    }
    // URL 参数覆盖布局（便于不同容器尺寸实测微调）
    if (urlParams.get('name')) setting.name = urlParams.get('name');
    if (urlParams.get('x')) setting.x = Number(urlParams.get('x'));
    if (urlParams.get('y')) setting.y = Number(urlParams.get('y'));
}

/** 解析模型文件 URL：优先 setting.url，缺省回退到公共模型默认文件 */
function resolveModelUrl() {
    if (setting.url) return setting.url;
    return MODEL_BASE_URL + 'hiyori_pro/hiyori_pro_t11.model3.json';
}

/** 加载模型（串行化，避免并发 from；先销毁旧模型防泄漏） */
async function loadModel() {
    if (loading) return;
    loading = true;
    try {
        destroyModel();
        const url = resolveModelUrl();
        console.log('[Live2D] 加载模型:', url);
        const m = await window.PIXI.live2d.Live2DModel.from(url, { autoInteract: !!setting.autoInteract });
        if (!m) throw new Error('Live2DModel.from 返回空结果');
        model = m;
        app.stage.addChild(model);
        model.anchor.set(0.5, 0.5);
        view.base = 0;
        applyLayout();
    } finally {
        loading = false;
    }
}

/** 安全销毁当前模型（切换/失败/重载时调用） */
function destroyModel() {
    if (!model) return;
    try { app?.stage.removeChild(model); } catch (e) { /* 已移除 */ }
    try { model.destroy(); } catch (e) { /* 忽略销毁异常 */ }
    model = null;
}

// ============================================================
//  取景：自适应缩放 + 缩放 / 平移 / 复位
// ============================================================

/** 容器的可用像素尺寸（隐藏或 0 尺寸时回退到渲染器尺寸） */
function containerSize() {
    const container = document.getElementById('live2d-container');
    const w = container?.clientWidth || app?.renderer?.width || window.innerWidth;
    const h = container?.clientHeight || app?.renderer?.height || window.innerHeight;
    return { w: Math.max(1, w), h: Math.max(1, h) };
}

/** 模型在缩放 1 时的像素尺寸（pixi-live2d-display 的 internalModel 布局尺寸） */
function modelSize() {
    const inner = model?.internalModel;
    return {
        nw: Number(inner?.width) || 0,
        nh: Number(inner?.height) || 0,
    };
}

/**
 * 自适应基准缩放。
 * 优先完整可见（contain）；若容器偏宽导致模型只占一条竖带，
 * 则按宽度放大到接近铺满，同时把纵向裁切限制在 2.2 倍以内，
 * 让小窗口里也能看到「头 + 上半身」，而不是一个缩得很小的全身。
 */
function computeBaseScale(w, h, nw, nh) {
    if (MANUAL_SCALE && Number.isFinite(MANUAL_SCALE) && MANUAL_SCALE > 0) {
        return MANUAL_SCALE;
    }
    if (!nw || !nh) return 0.12;
    const fitW = (w * 0.92) / nw;
    const fitH = (h * 0.92) / nh;
    let base = Math.min(fitW, fitH);
    if (base * nw < w * 0.55) {
        base = Math.min((w * 0.88) / nw, (h * 2.2) / nh, base * 2.6);
    }
    return base;
}

/** 模型中心在容器中的纵向基准位置：完整可见居中，超高时头顶贴近上沿 */
function baseCenterY(scale, h, nh) {
    const mh = nh * scale;
    return mh >= h ? h * 0.04 + mh / 2 : h / 2;
}

/** 平移边界：至少保留 40px 模型在视野内，避免完全拖出屏幕 */
function clampPan(w, h, mw, mh) {
    const keep = 40;
    const limitX = w / 2 + mw / 2 - keep;
    const limitY = h / 2 + mh / 2 - keep;
    view.panX = Math.min(limitX, Math.max(-limitX, view.panX));
    view.panY = Math.min(limitY, Math.max(-limitY, view.panY));
}

/** 把视图状态同步到模型变换 */
function applyLayout() {
    if (!model || !app) return;
    const { w, h } = containerSize();
    const { nw, nh } = modelSize();
    if (!nw || !nh) return;
    if (!view.base) view.base = computeBaseScale(w, h, nw, nh);
    const scale = view.base * view.zoom;
    const mw = nw * scale;
    const mh = nh * scale;
    clampPan(w, h, mw, mh);
    model.scale.set(scale);
    model.anchor.set(0.5, 0.5);
    model.x = w / 2 + view.panX;
    model.y = baseCenterY(scale, h, nh) + view.panY;
}

/** 上一次的容器尺寸，用于区分「真实尺寸变化」与 ResizeObserver 的首帧回调 */
let lastSize = { w: 0, h: 0 };

/** 容器尺寸变化：重设应用尺寸并按新容器重算基准缩放（保留用户缩放与平移） */
function reloadContainer() {
    if (!app) return;
    const container = document.getElementById('live2d-container');
    if (!container) return;
    const w = container.clientWidth;
    const h = container.clientHeight;
    if (w < 1 || h < 1) return;
    const changed = lastSize.w > 0 && lastSize.h > 0 && (lastSize.w !== w || lastSize.h !== h);
    lastSize = { w, h };
    try { app.renderer.resize(w, h); } catch (e) { /* 忽略 */ }
    const { nw, nh } = modelSize();
    if (nw && nh) view.base = computeBaseScale(w, h, nw, nh);
    applyLayout();
    if (changed) flashHint(2000);
}

/** 复位：回到自适应取景（双击 / 中键 / 宿主调用） */
function resetView() {
    const { w, h } = containerSize();
    const { nw, nh } = modelSize();
    view.zoom = 1;
    view.panX = 0;
    view.panY = 0;
    if (nw && nh) view.base = computeBaseScale(w, h, nw, nh);
    applyLayout();
}

/** 以容器内某点为锚点缩放（factor > 1 放大，< 1 缩小） */
function zoomAt(factor, px, py) {
    if (!model) return;
    const { w, h } = containerSize();
    const { nh } = modelSize();
    if (!nh) return;
    const oldScale = view.base * view.zoom;
    const nextZoom = Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, view.zoom * factor));
    if (nextZoom === view.zoom) return;
    const k = (view.base * nextZoom) / oldScale;
    // 缩放前模型中心在容器中的位置
    const cx = w / 2 + view.panX;
    const cy = baseCenterY(oldScale, h, nh) + view.panY;
    // 让锚点在缩放前后保持不动
    const ncx = px + (cx - px) * k;
    const ncy = py + (cy - py) * k;
    view.zoom = nextZoom;
    const newScale = view.base * nextZoom;
    view.panX = ncx - w / 2;
    view.panY = ncy - baseCenterY(newScale, h, nh);
    applyLayout();
}

/** 按容器像素平移 */
function panBy(dx, dy) {
    if (!dx && !dy) return;
    view.panX += dx;
    view.panY += dy;
    applyLayout();
}

// ============================================================
//  交互：滚轮 / 双指缩放、拖动、双击与中键复位
// ============================================================
const pointers = new Map();
let dragOrigin = null;
let pinchState = null;

function canvasPoint(event) {
    const canvas = document.getElementById('live2d-canvas');
    const rect = canvas.getBoundingClientRect();
    return { x: event.clientX - rect.left, y: event.clientY - rect.top };
}

function onWheel(event) {
    event.preventDefault();
    const point = canvasPoint(event);
    const unit = event.deltaMode === 1 ? 0.05 : event.deltaMode === 2 ? 1 : 0.0012;
    const factor = Math.exp(-event.deltaY * unit);
    zoomAt(factor, point.x, point.y);
}

function onPointerDown(event) {
    const canvas = event.currentTarget;
    if (event.pointerType === 'mouse' && event.button === 1) {
        // 中键：复位取景，同时屏蔽浏览器自动滚动
        event.preventDefault();
        resetView();
        return;
    }
    if (event.pointerType === 'mouse' && event.button !== 0) return;
    event.preventDefault();
    try { canvas.setPointerCapture(event.pointerId); } catch (e) { /* 不支持时忽略 */ }
    pointers.set(event.pointerId, { x: event.clientX, y: event.clientY });
    if (pointers.size === 1) {
        dragOrigin = { x: event.clientX, y: event.clientY };
        canvas.classList.add('dragging');
    } else if (pointers.size === 2) {
        const [a, b] = [...pointers.values()];
        pinchState = { dist: distance(a, b), mid: midpoint(a, b) };
        dragOrigin = null;
        canvas.classList.add('dragging');
    }
}

function onPointerMove(event) {
    if (!pointers.has(event.pointerId)) return;
    pointers.set(event.pointerId, { x: event.clientX, y: event.clientY });
    if (pointers.size >= 2 && pinchState) {
        const [a, b] = [...pointers.values()];
        const dist = distance(a, b);
        const mid = midpoint(a, b);
        if (pinchState.dist > 0 && dist > 0) {
            const point = canvasPoint({ clientX: mid.x, clientY: mid.y });
            zoomAt(dist / pinchState.dist, point.x, point.y);
        }
        panBy(mid.x - pinchState.mid.x, mid.y - pinchState.mid.y);
        pinchState = { dist, mid };
        return;
    }
    if (!dragOrigin) return;
    panBy(event.clientX - dragOrigin.x, event.clientY - dragOrigin.y);
    dragOrigin = { x: event.clientX, y: event.clientY };
}

function onPointerEnd(event) {
    pointers.delete(event.pointerId);
    const canvas = event.currentTarget;
    if (pointers.size === 0) {
        dragOrigin = null;
        pinchState = null;
        canvas.classList.remove('dragging');
        return;
    }
    const [rest] = [...pointers.values()];
    dragOrigin = { x: rest.x, y: rest.y };
    pinchState = null;
}

function distance(a, b) {
    return Math.hypot(a.x - b.x, a.y - b.y);
}

function midpoint(a, b) {
    return { x: (a.x + b.x) / 2, y: (a.y + b.y) / 2 };
}

function bindInteractions() {
    const canvas = document.getElementById('live2d-canvas');
    if (!canvas || canvas.dataset.bound === '1') return;
    canvas.dataset.bound = '1';
    canvas.addEventListener('wheel', onWheel, { passive: false });
    canvas.addEventListener('pointerdown', onPointerDown);
    canvas.addEventListener('pointermove', onPointerMove);
    canvas.addEventListener('pointerup', onPointerEnd);
    canvas.addEventListener('pointercancel', onPointerEnd);
    canvas.addEventListener('dblclick', (event) => {
        event.preventDefault();
        resetView();
    });
    // 双击缩放与中键自动滚动都交给引擎处理
    canvas.addEventListener('mousedown', (event) => {
        if (event.button === 1) event.preventDefault();
    });
}

/** 监听容器尺寸变化（iframe 内只依赖自身尺寸，用 ResizeObserver 更准） */
function observeContainer() {
    const container = document.getElementById('live2d-container');
    if (window.ResizeObserver && container) {
        new ResizeObserver(() => {
            window.requestAnimationFrame(() => reloadContainer());
        }).observe(container);
    }
    window.addEventListener('resize', () => reloadContainer());
}

// 操作提示：仅在容器尺寸变化时出现 2 秒
let hintTimer = null;

function flashHint(duration = 2000) {
    const hint = document.getElementById('view-hint');
    if (!hint) return;
    hint.classList.add('visible');
    clearTimeout(hintTimer);
    hintTimer = setTimeout(() => hint.classList.remove('visible'), duration);
}

// ============================================================
//  动作 / 情绪映射
// ============================================================

/** 当前模型的可用动作组定义表（{ 组名: 动作数组 }） */
function availableMotions() {
    return model?.internalModel?.motionManager?.definitions || {};
}

/**
 * 上报可用动作（形状与基岩版引擎的 animation_list 一致）。
 * 后端不再消费该帧；保留给面板 / 工作室等总线参与者按需读取，只列出有动作帧的组。
 */
function broadcastAnimationList() {
    const motions = availableMotions();
    const actionDefs = Object.keys(motions)
        .filter(name => Array.isArray(motions[name]) && motions[name].length > 0)
        .map(name => ({ name, mouseTracking: false }));
    broadcast('animation_list', {
        groups: [],
        allAnimations: [],
        bones: [],
        actionDefinitions: actionDefs,
    });
}

/**
 * 播放指定情绪对应的动作组。
 * 查 setting.mapping（或内置兜底）得到候选组，取首个模型确实存在的组播放；
 * 全部缺失则尽力直呼首个候选组。
 * @param {string} name 情绪名（大小写不敏感，如 idle/happy/hug）
 * @param {number} index 组内动作下标（默认 0）
 */
function setEmotion(name, index = 0) {
    if (!model) return false;
    const key = String(name || '').toLowerCase();
    currentEmotion = key || 'idle';
    const mapping = Object.keys(setting.mapping || {}).length ? setting.mapping : DEFAULT_MAPPING;
    let groups = mapping[key];
    if (!groups) {
        // 未在映射表中：若自身就是模型的动作组名，直接播放
        groups = availableMotions()[name] ? [name] : null;
    }
    if (!groups || !groups.length) groups = mapping.idle || ['Idle'];
    const defs = availableMotions();
    for (const g of groups) {
        if (Array.isArray(defs[g]) && defs[g].length) {
            try { model.motion(g, index); return true; } catch (e) { /* 尝试下一个候选 */ }
        }
    }
    try { model.motion(groups[0], index); } catch (e) { /* 无可用动作 */ }
    return false;
}

/** 播放情绪动作，duration 毫秒后自动回落到 idle（duration<=0 表示不回落） */
function setEmotionWithTimeout(name, duration = 9000) {
    if (emotionTimer) { clearTimeout(emotionTimer); emotionTimer = null; }
    setEmotion(name);
    if (duration > 0 && currentEmotion !== 'idle') {
        emotionTimer = setTimeout(() => {
            emotionTimer = null;
            setEmotion('idle');
        }, duration);
    }
}

/** 模块就绪判定（供宿主探测） */
function isReady() { return !!model; }

/** 取回底层模型实例 */
function getModel() { return model; }

/** 取回当前视图状态（调试用） */
function getView() {
    return {
        base: view.base,
        zoom: view.zoom,
        panX: view.panX,
        panY: view.panY,
    };
}

 // ============================================================
 //  统一总线：入站 / 出站
 // ============================================================

channel.onmessage = (event) => {
    const raw = event.data;
    if (!raw || raw.source === SOURCE_ENGINE) return; // 忽略自己发的
    // 智能体引擎命令包装：{type:"engine", data:{type,payload,source}} → 解包
    let msg = raw;
    if (raw.type === 'engine' && raw.data && raw.data.type) {
        msg = { type: raw.data.type, source: raw.data.source, payload: raw.data.payload };
    } else if (raw.type === 'context') {
        reactToPushedMessage(raw.data); // 展示类消息：引擎自行匹配动作，不执行指令
        return;
    }
    handleChannelMessage(msg);
};

/**
 * 后端推送的对话消息：按 50% 概率用嵌入模型在本地动作列表里挑选最匹配的动作。
 * 后端不再下发 action 指令，匹配与播放完全由引擎侧完成。
 */
function reactToPushedMessage(data) {
    if (!actionMatcher || !data || data.type !== 'text') return;
    const text = String(data.content || '').trim();
    if (!text) return;
    const motions = availableMotions();
    const names = Object.keys(motions)
        .filter(name => Array.isArray(motions[name]) && motions[name].length > 0);
    if (names.length === 0) return;
    actionMatcher.consider(text, names, name => setEmotionWithTimeout(name, 9000));
}

/** 引擎指令分发（仅处理 Live2D 有意义的类型，其余安全忽略） */
function handleChannelMessage(msg) {
    const { type, payload } = msg || {};
    switch (type) {
        case 'panel_ready':
            // 面板就绪握手：晚订阅的面板/智能体可重新请求状态与动作列表
            broadcast('engine_ready', { mode: 'embedded' });
            broadcastAnimationList();
            break;

        case 'action': {
            // 动作指令 → 情绪/动作映射（动作名兼容 action / emotion / state 三种字段）
            const name = payload?.action || payload?.emotion || payload?.state;
            if (name) setEmotionWithTimeout(name, payload?.duration || 9000);
            break;
        }

        case 'mode_changed':
            // Live2D 无编辑器模式，仅记录不处理
            break;

        case 'query_molang':
            // 回空值，避免调用方挂等（形状与基岩引擎一致）
            broadcast('molang_value', { expression: null, value: null });
            break;

        default:
            // movement/rotation/quick_track/highlight_bone/physics_*/primitive_*/anim_group_*
            // 等 3D 语义指令：Live2D 无对应表现，安全忽略
            break;
    }
}

/** 总线出站（形状与基岩引擎完全一致） */
function broadcast(type, payload) {
    channel.postMessage({
        type,
        source: SOURCE_ENGINE,
        payload,
        timestamp: Date.now(),
    });
}

// ============================================================
//  UI 辅助
// ============================================================
function hideLoadingMask() {
    const mask = document.getElementById('loading-mask');
    if (!mask) return;
    mask.classList.add('hidden');
    setTimeout(() => { mask.style.display = 'none'; }, 500);
}

function showError(err) {
    const mask = document.getElementById('loading-mask');
    if (!mask) return;
    mask.classList.remove('hidden');
    mask.style.display = 'flex';
    const msg = (err && err.message) ? err.message : String(err);
    mask.innerHTML = `<span>Live2D 加载失败：${msg}</span>`;
}

// ==== 对外暴露（调试 / 宿主可选调用） ====
window.Live2DEngine = {
    init,
    loadModel,
    setEmotion,
    setEmotionWithTimeout,
    reloadContainer,
    resetView,
    zoomBy: (factor) => {
        const { w, h } = containerSize();
        zoomAt(factor, w / 2, h / 2);
    },
    getView,
    isReady,
    getModel,
};

// ==== 启动 ====
init();
