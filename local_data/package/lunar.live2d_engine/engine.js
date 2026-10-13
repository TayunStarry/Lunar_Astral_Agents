// ==== engine.js — 月华 Live2D 引擎层 · 引擎入口 ====
//
// 职责：
//   - 与基岩引擎（lunar.rendering_engine）同构的统一通信：
//     接收走 /ws（宿主以 ?bus=0 退出原始总线订阅，仅消费结构化 engine 指令），
//     发送走 POST /write/engine；频道名通过 URL ?channel= 指定。
//   - 初始化顺序：waitForPIXI → waitForLive2DPlugin → PIXI.Application →
//     读取 setting.json → 加载模型 → 布局 → 广播 engine_ready
//   - 入站 engine 指令：panel_ready 握手 / action 动作映射 / mode_changed 记录 /
//     query_molang 回包；movement/rotation 等 3D 语义指令安全忽略
//   - 尺寸自适应：容器变化时重算应用尺寸、模型缩放与位置
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

// ==== 统一通信客户端 ====
const channel = new WsBridge(CHANNEL_NAME);

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
        await loadSetting();
        await loadModel();
        window.addEventListener('resize', () => reloadContainer());
        setEmotion('idle');
        broadcast('engine_ready', { mode: 'embedded' });
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
    if (urlParams.get('scale')) setting.scale = Number(urlParams.get('scale'));
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

/** 布局：缩放 + 锚点居中 + 按容器比例定位 */
function applyLayout() {
    if (!model || !app) return;
    const container = document.getElementById('live2d-container');
    const w = container?.clientWidth || app.renderer.width;
    const h = container?.clientHeight || app.renderer.height;
    const baseScale = Number(setting.scale) || 0.15;
    // 宿主容器高度差异大：桌面侧栏小方框（<500）额外缩小，移动全高容器用基准值
    const scale = h < 500 ? baseScale * 0.65 : baseScale;
    model.scale.set(scale);
    model.anchor.set(0.5, 0.5);
    model.x = w * (Number(setting.x) || 0.5);
    model.y = h * (Number(setting.y) || 0.5);
}

/** 容器尺寸变化：重设应用尺寸并重算布局 */
function reloadContainer() {
    if (!app) return;
    const container = document.getElementById('live2d-container');
    if (!container) return;
    try { app.renderer.resize(container.clientWidth, container.clientHeight); } catch (e) { /* 忽略 */ }
    applyLayout();
}

// ============================================================
//  动作 / 情绪映射
// ============================================================

/** 当前模型的可用动作组定义表（{ 组名: 动作数组 }） */
function availableMotions() {
    return model?.internalModel?.motionManager?.definitions || {};
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
        return; // 展示类消息，引擎不执行
    }
    handleChannelMessage(msg);
};

/** 引擎指令分发（仅处理 Live2D 有意义的类型，其余安全忽略） */
function handleChannelMessage(msg) {
    const { type, payload } = msg || {};
    switch (type) {
        case 'panel_ready':
            // 面板就绪握手：晚订阅的面板可重新请求状态
            broadcast('engine_ready', { mode: 'embedded' });
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
    isReady,
    getModel,
};

// ==== 启动 ====
init();
